package kucoin

import (
	"context"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"
	"golang.org/x/time/rate"

	"github.com/c9s/bbgo/pkg/exchange/kucoin/kucoinapi"
	"github.com/c9s/bbgo/pkg/types"
)

var ErrMissingSequence = errors.New("sequence is missing")

// OKB is the platform currency of OKEx, pre-allocate static string here
const KCS = "KCS"

var log = logrus.WithFields(logrus.Fields{
	"exchange": "kucoin",
})

type Exchange struct {
	key, secret, passphrase string
	client                  *kucoinapi.RestClient
}

func New(key, secret, passphrase string) *Exchange {
	client := kucoinapi.NewClient()

	// for public access mode
	if len(key) > 0 && len(secret) > 0 && len(passphrase) > 0 {
		client.Auth(key, secret, passphrase)
	}

	return &Exchange{
		key:        key,
		secret:     secret,
		passphrase: passphrase,
		client:     client,
	}
}

func (e *Exchange) Name() types.ExchangeName {
	return types.ExchangeKucoin
}

func (e *Exchange) PlatformFeeCurrency() string {
	return KCS
}

func (e *Exchange) QueryAccount(ctx context.Context) (*types.Account, error) {
	accounts, err := e.client.AccountService.ListAccounts()
	if err != nil {
		return nil, err
	}

	// for now, we only return the trading account
	a := types.NewAccount()
	balances := toGlobalBalanceMap(accounts)
	a.UpdateBalances(balances)
	return a, nil
}

func (e *Exchange) QueryAccountBalances(ctx context.Context) (types.BalanceMap, error) {
	accounts, err := e.client.AccountService.ListAccounts()
	if err != nil {
		return nil, err
	}

	return toGlobalBalanceMap(accounts), nil
}

func (e *Exchange) QueryMarkets(ctx context.Context) (types.MarketMap, error) {
	markets, err := e.client.MarketDataService.ListSymbols()
	if err != nil {
		return nil, err
	}

	marketMap := types.MarketMap{}
	for _, s := range markets {
		market := toGlobalMarket(s)
		marketMap.Add(market)
	}

	return marketMap, nil
}

func (e *Exchange) QueryTicker(ctx context.Context, symbol string) (*types.Ticker, error) {
	s, err := e.client.MarketDataService.GetTicker24HStat(symbol)
	if err != nil {
		return nil, err
	}

	ticker := toGlobalTicker(*s)
	return &ticker, nil
}

func (e *Exchange) QueryTickers(ctx context.Context, symbols ...string) (map[string]types.Ticker, error) {
	tickers := map[string]types.Ticker{}
	if len(symbols) > 0 {
		for _, s := range symbols {
			t, err := e.QueryTicker(ctx, s)
			if err != nil {
				return nil, err
			}

			tickers[s] = *t
		}

		return tickers, nil
	}

	allTickers, err := e.client.MarketDataService.ListTickers()
	if err != nil {
		return nil, err
	}

	for _, s := range allTickers.Ticker {
		tickers[s.Symbol] = toGlobalTicker(s)
	}

	return tickers, nil
}

// From the doc
// Type of candlestick patterns: 1min, 3min, 5min, 15min, 30min, 1hour, 2hour, 4hour, 6hour, 8hour, 12hour, 1day, 1week
var supportedIntervals = map[types.Interval]int{
	types.Interval1m:  60,
	types.Interval5m:  60 * 5,
	types.Interval15m:  60 * 15,
	types.Interval30m: 60 * 30,
	types.Interval1h:  60 * 60,
	types.Interval2h:  60 * 60 * 2,
	types.Interval4h:  60 * 60 * 4,
	types.Interval6h:  60 * 60 * 6,
	// types.Interval8h: 60 * 60 * 8,
	types.Interval12h: 60 * 60 * 12,
}

func (e *Exchange) SupportedInterval() map[types.Interval]int {
	return supportedIntervals
}

func (e *Exchange) IsSupportedInterval(interval types.Interval) bool {
	_, ok := supportedIntervals[interval]
	return ok
}

var marketDataLimiter = rate.NewLimiter(rate.Every(200*time.Millisecond), 1)

func (e *Exchange) QueryKLines(ctx context.Context, symbol string, interval types.Interval, options types.KLineQueryOptions) ([]types.KLine, error) {
	_ = marketDataLimiter.Wait(ctx)

	req := e.client.MarketDataService.NewGetKLinesRequest()
	req.Symbol(toLocalSymbol(symbol))
	req.Interval(toLocalInterval(interval))
	if options.StartTime != nil {
		req.StartAt(*options.StartTime)
		// For each query, the system would return at most **1500** pieces of data. To obtain more data, please page the data by time.
		req.EndAt(options.StartTime.Add(1500 * interval.Duration()))
	} else if options.EndTime != nil {
		req.EndAt(*options.EndTime)
	}

	ks, err := req.Do(ctx)
	if err != nil {
		return nil, err
	}

	var klines []types.KLine
	for _, k := range ks {
		gi := toGlobalInterval(k.Interval)
		klines = append(klines, types.KLine{
			Exchange:    types.ExchangeKucoin,
			Symbol:      toGlobalSymbol(k.Symbol),
			StartTime:   types.Time(k.StartTime),
			EndTime:     types.Time(k.StartTime.Add(gi.Duration() - time.Millisecond)),
			Interval:    gi,
			Open:        k.Open.Float64(),
			Close:       k.Close.Float64(),
			High:        k.High.Float64(),
			Low:         k.Low.Float64(),
			Volume:      k.Volume.Float64(),
			QuoteVolume: k.QuoteVolume.Float64(),
			Closed:      true,
		})
	}

	return klines, nil
}

func (e *Exchange) SubmitOrders(ctx context.Context, orders ...types.SubmitOrder) (createdOrders types.OrderSlice, err error) {
	for _, order := range orders {
		req := e.client.TradeService.NewPlaceOrderRequest()
		req.Symbol(toLocalSymbol(order.Symbol))
		req.Side(toLocalSide(order.Side))

		if order.ClientOrderID != "" {
			req.ClientOrderID(order.ClientOrderID)
		}

		if len(order.QuantityString) > 0 {
			req.Size(order.QuantityString)
		} else if order.Market.Symbol != "" {
			req.Size(order.Market.FormatQuantity(order.Quantity))
		} else {
			req.Size(strconv.FormatFloat(order.Quantity, 'f', 8, 64))
		}

		// set price field for limit orders
		switch order.Type {
		case types.OrderTypeStopLimit, types.OrderTypeLimit:
			if len(order.PriceString) > 0 {
				req.Price(order.PriceString)
			} else if order.Market.Symbol != "" {
				req.Price(order.Market.FormatPrice(order.Price))
			}
		}

		switch order.TimeInForce {
		case "FOK":
			req.TimeInForce(kucoinapi.TimeInForceFOK)
		case "IOC":
			req.TimeInForce(kucoinapi.TimeInForceIOC)
		default:
			// default to GTC
			req.TimeInForce(kucoinapi.TimeInForceGTC)
		}

		orderResponse, err := req.Do(ctx)
		if err != nil {
			return createdOrders, err
		}

		createdOrders = append(createdOrders, types.Order{
			SubmitOrder:      order,
			Exchange:         types.ExchangeKucoin,
			OrderID:          hashStringID(orderResponse.OrderID),
			Status:           types.OrderStatusNew,
			ExecutedQuantity: 0,
			IsWorking:        true,
			CreationTime:     types.Time(time.Now()),
			UpdateTime:       types.Time(time.Now()),
		})
	}

	return createdOrders, err
}

// QueryOpenOrders
/*
Documentation from the Kucoin API page

Any order on the exchange order book is in active status.
Orders removed from the order book will be marked with done status.
After an order becomes done, there may be a few milliseconds latency before it’s fully settled.

You can check the orders in any status.
If the status parameter is not specified, orders of done status will be returned by default.

When you query orders in active status, there is no time limit.
However, when you query orders in done status, the start and end time range cannot exceed 7* 24 hours.
An error will occur if the specified time window exceeds the range.

If you specify the end time only, the system will automatically calculate the start time as end time minus 7*24 hours, and vice versa.

The history for cancelled orders is only kept for one month.
You will not be able to query for cancelled orders that have happened more than a month ago.
*/
func (e *Exchange) QueryOpenOrders(ctx context.Context, symbol string) (orders []types.Order, err error) {
	req := e.client.TradeService.NewListOrdersRequest()
	req.Symbol(toLocalSymbol(symbol))
	req.Status("active")
	orderList, err := req.Do(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: support pagination (right now we can only get 50 items from the first page)
	for _, o := range orderList.Items {
		order := toGlobalOrder(o)
		orders = append(orders, order)
	}

	return orders, err
}

func (e *Exchange) QueryClosedOrders(ctx context.Context, symbol string, since, until time.Time, lastOrderID uint64) (orders []types.Order, err error) {
	req := e.client.TradeService.NewListOrdersRequest()
	req.Symbol(toLocalSymbol(symbol))
	req.Status("done")
	req.EndAt(until)
	req.StartAt(since)

	orderList, err := req.Do(ctx)
	if err != nil {
		return orders, err
	}

	// TODO: support pagination (right now we can only get 50 items from the first page)
	for _, o := range orderList.Items {
		order := toGlobalOrder(o)
		orders = append(orders, order)
	}

	return orders, err
}

func (e *Exchange) QueryTrades(ctx context.Context, symbol string, options *types.TradeQueryOptions) (trades []types.Trade, err error) {
	req := e.client.TradeService.NewGetFillsRequest()
	req.Symbol(toLocalSymbol(symbol))
	if options.StartTime != nil {
		req.StartAt(*options.StartTime)
	} else if options.EndTime != nil {
		req.EndAt(*options.EndTime)
	}

	response, err := req.Do(ctx)
	if err != nil {
		return trades, err
	}
	for _, fill := range response.Items {
		trade := toGlobalTrade(fill)
		trades = append(trades, trade)
	}

	return trades, nil
}

func (e *Exchange) CancelOrders(ctx context.Context, orders ...types.Order) (errs error) {
	for _, o := range orders {
		req := e.client.TradeService.NewCancelOrderRequest()

		if o.UUID != "" {
			req.OrderID(o.UUID)
		} else if o.ClientOrderID != "" {
			req.ClientOrderID(o.ClientOrderID)
		} else {
			errs = multierr.Append(errs, errors.New("can not cancel order, either order uuid nor client order id is empty"))
			continue
		}

		response, err := req.Do(ctx)
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}

		log.Infof("cancelled orders: %v", response.CancelledOrderIDs)
	}

	return errs
}

func (e *Exchange) NewStream() types.Stream {
	return NewStream(e.client, e)
}

func (e *Exchange) QueryDepth(ctx context.Context, symbol string) (types.SliceOrderBook, int64, error) {
	orderBook, err := e.client.MarketDataService.GetOrderBook(toLocalSymbol(symbol), 100)
	if err != nil {
		return types.SliceOrderBook{}, 0, err
	}

	if len(orderBook.Sequence) == 0 {
		return types.SliceOrderBook{}, 0, ErrMissingSequence
	}

	sequence, err := strconv.ParseInt(orderBook.Sequence, 10, 64)
	if err != nil {
		return types.SliceOrderBook{}, 0, err
	}

	return types.SliceOrderBook{
		Symbol: toGlobalSymbol(symbol),
		Bids:   orderBook.Bids,
		Asks:   orderBook.Asks,
	}, sequence, nil
}
