package ftx

import (
	"context"
	"fmt"
	"time"

	"github.com/gorilla/websocket"

	"github.com/c9s/bbgo/pkg/service"
	"github.com/c9s/bbgo/pkg/types"
)

const endpoint = "wss://ftx.com/ws/"

type Stream struct {
	*types.StandardStream

	ws       *service.WebsocketClientBase
	exchange *Exchange

	key        string
	secret     string
	subAccount string

	// subscriptions are only accessed in single goroutine environment, so I don't use mutex to protect them
	subscriptions      []websocketRequest
	klineSubscriptions []klineSubscription
}

type klineSubscription struct {
	symbol   string
	interval types.Interval
}

func NewStream(key, secret string, subAccount string, e *Exchange) *Stream {
	s := &Stream{
		exchange:       e,
		key:            key,
		secret:         secret,
		subAccount:     subAccount,
		StandardStream: &types.StandardStream{},
		ws:             service.NewWebsocketClientBase(endpoint, 3*time.Second),
	}

	s.ws.OnMessage((&messageHandler{StandardStream: s.StandardStream}).handleMessage)
	s.ws.OnConnected(func(conn *websocket.Conn) {
		subs := []websocketRequest{newLoginRequest(s.key, s.secret, time.Now(), s.subAccount)}
		subs = append(subs, s.subscriptions...)
		for _, sub := range subs {
			if err := conn.WriteJSON(sub); err != nil {
				s.ws.EmitError(fmt.Errorf("failed to send subscription: %+v", sub))
			}
		}

		s.EmitConnect()
	})

	return s
}

func (s *Stream) Connect(ctx context.Context) error {
	// If it's not public only, let's do the authentication.
	if !s.PublicOnly {
		s.subscribePrivateEvents()
	}

	if err := s.ws.Connect(ctx); err != nil {
		return err
	}
	s.EmitStart()
	go s.pollKLines(ctx)

	go func() {
		// https://docs.ftx.com/?javascript#request-process
		tk := time.NewTicker(15 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				if err := ctx.Err(); err != nil {
					logger.WithError(err).Errorf("websocket ping goroutine is terminated")
				}
			case <-tk.C:
				if err := s.ws.Conn().WriteJSON(websocketRequest{
					Operation: ping,
				}); err != nil {
					logger.WithError(err).Warnf("failed to ping, try in next tick")
				}
			}
		}
	}()
	return nil
}

func (s *Stream) subscribePrivateEvents() {
	s.addSubscription(websocketRequest{
		Operation: subscribe,
		Channel:   privateOrdersChannel,
	})
	s.addSubscription(websocketRequest{
		Operation: subscribe,
		Channel:   privateTradesChannel,
	})
}

func (s *Stream) addSubscription(request websocketRequest) {
	s.subscriptions = append(s.subscriptions, request)
}

func (s *Stream) Subscribe(channel types.Channel, symbol string, option types.SubscribeOptions) {
	if channel == types.BookChannel {
		s.addSubscription(websocketRequest{
			Operation: subscribe,
			Channel:   orderBookChannel,
			Market:    toLocalSymbol(TrimUpperString(symbol)),
		})
	} else if channel == types.BookTickerChannel {
		s.addSubscription(websocketRequest{
			Operation: subscribe,
			Channel:   bookTickerChannel,
			Market:    toLocalSymbol(TrimUpperString(symbol)),
		})
	} else if channel == types.KLineChannel {
		// FTX does not support kline channel, do polling
		interval := types.Interval(option.Interval)
		ks := klineSubscription{symbol: symbol, interval: interval}
		s.klineSubscriptions = append(s.klineSubscriptions, ks)
	} else {
		panic("only support book/kline channel now")
	}
}

func (s *Stream) pollKLines(ctx context.Context) {

	lastClosed := time.Time{}
	// get current kline candle
	for _, sub := range s.klineSubscriptions {
		klines := getLast2KLine(s.exchange, ctx, sub.symbol, sub.interval)
		if len(klines) > 0 {
			// handle mutiple klines, get the latest one
			s.EmitKLine(klines[0])
			s.EmitKLineClosed(klines[0])
			s.EmitKLine(klines[1])
			lastClosed = klines[0].StartTime.Time()
		}
	}

	// the highest resolution of kline is 1min
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				logger.WithError(err).Errorf("pollKLines goroutine is terminated")
			}
			return
		case <-ticker.C:
			now := time.Now().Truncate(time.Minute)
			for _, sub := range s.klineSubscriptions {
				subTime := now.Truncate(sub.interval.Duration())
				if now != subTime {
					// not in the checking time slot, check next subscription
					continue
				}
				klines := getLast2KLine(s.exchange, ctx, sub.symbol, sub.interval)

				if len(klines) > 0 {
					// handle mutiple klines, get the latest one
					if lastClosed.Unix() < klines[0].StartTime.Unix() {
						s.EmitKLine(klines[0])
						s.EmitKLineClosed(klines[0])
						lastClosed = klines[0].StartTime.Time()
					}

					if len(klines) > 1 {
						s.EmitKLine(klines[1])
					}
				}
			}
		}
	}
}

func getLast2KLine(e *Exchange, ctx context.Context, symbol string, interval types.Interval) []types.KLine {
	// set since to more 30s ago to avoid getting no kline candle
	since := time.Now().Add(time.Duration(interval.Minutes()*-3) * time.Minute)
	klines, err := e.QueryKLines(ctx, symbol, interval, types.KLineQueryOptions{
		StartTime: &since,
		Limit:     2,
	})
	if err != nil {
		logger.WithError(err).Errorf("failed to get kline data")
		return klines
	}

	return klines
}

func getLastClosedKLine(e *Exchange, ctx context.Context, symbol string, interval types.Interval) []types.KLine {
	// set since to more 30s ago to avoid getting no kline candle
	klines := getLast2KLine(e, ctx, symbol, interval)
	if len(klines) == 0 {
		return []types.KLine{}
	}
	return []types.KLine{klines[0]}
}

func (s *Stream) Close() error {
	s.subscriptions = nil
	if s.ws != nil {
		return s.ws.Conn().Close()
	}
	return nil
}
