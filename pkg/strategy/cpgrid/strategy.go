package cpgrid

import (
	"context"
	"fmt"
	"github.com/c9s/bbgo/pkg/exchange/max"
	"github.com/c9s/bbgo/pkg/indicator"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sort"
	"sync"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/service"
	"github.com/c9s/bbgo/pkg/types"
)

const ID = "cpgrid"

var log = logrus.WithField("strategy", ID)
var orderLock sync.Mutex

func init() {
	// Register the pointer of the strategy struct,
	// so that bbgo knows what struct to be used to unmarshal the configs (YAML or JSON)
	// Note: built-in strategies need to imported manually in the bbgo cmd package.
	bbgo.RegisterStrategy(ID, &Strategy{})
}

//manage all asset we used
type StrategyAccount struct {
	types.AssetMap
}

type AssetMap map[string]*types.Asset

func (m AssetMap) String() (o string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a := m[k]
		o += fmt.Sprintf(" %s: %f ,",
			a.Currency,
			a.Total.Float64(),
		)
	}
	return o
}

// State is the grid snapshot
type State struct {
	Position           *types.Position `json:"position,omitempty"`
	Cells              []GridCell      `json:"cells"`
	Profits            *AssetMap
	TradeCount         int
	Fees               *AssetMap
	ReinvestingProfits *AssetMap
	LockProfits        *AssetMap
	ProfitStats        bbgo.ProfitStats `json:"profitStats,omitempty"`
	UsingLockProfits   bool
	ShouldReInv        bool
}

type Strategy struct {
	// The notification system will be injected into the strategy automatically.
	// This field will be injected automatically since it's a single exchange strategy.
	*bbgo.Notifiability `json:"-" yaml:"-"`
	*bbgo.Graceful      `json:"-" yaml:"-"`
	*bbgo.Persistence

	// OrderExecutor is an interface for submitting Order.
	// This field will be injected automatically since it's a single exchange strategy.
	bbgo.OrderExecutor `json:"-" yaml:"-"`

	// Market stores the configuration of the market, for example, VolumePrecision, PricePrecision, MinLotSize... etc
	// This field will be injected automatically since we defined the Symbol field.
	types.Market `json:"-" yaml:"-"`

	TradeService *service.TradeService `json:"-" yaml:"-"`

	// These fields will be filled from the config file (it translates YAML to JSON)
	Symbol string `json:"symbol" yaml:"symbol"`

	// ProfitSpread is the fixed profit spread you want to submit the sell Order
	//ProfitSpread fixedpoint.Value `json:"profitSpread" yaml:"profitSpread"`

	//will ignore ProfitSpread if you set ProfitSpreadRate
	ProfitSpreadRate fixedpoint.Value `json:"profitSpreadRate" yaml:"profitSpreadRate"`

	// GridNum is the grid number, how many orders you want to post on the orderbook.
	GridNum int `json:"gridNumber" yaml:"gridNumber"`

	UpperPrice fixedpoint.Value `json:"upperPrice" yaml:"upperPrice"`

	LowerPrice fixedpoint.Value `json:"lowerPrice" yaml:"lowerPrice"`

	// Quantity is the quantity you want to submit for each Order.
	Quantity fixedpoint.Value `json:"quantity,omitempty"`

	// QuantityScale helps user to define the quantity by price scale or volume scale
	QuantityScale *bbgo.PriceVolumeScale `json:"quantityScale,omitempty"`

	// FixedAmount is used for fixed amount (dynamic quantity) if you don't want to use fixed quantity.
	FixedAmount fixedpoint.Value `json:"amount,omitempty" yaml:"amount"`

	// Side is the initial maker orders side. defaults to "both"
	Side types.SideType `json:"side" yaml:"side"`

	// CatchUp let the maker grid catch up with the price change.
	CatchUp bool `json:"catchUp" yaml:"catchUp"`

	// Long means you want to hold more base asset than the quote asset.
	//Long bool `json:"long,omitempty" yaml:"long,omitempty"`

	KeepBaseCurrency bool `json:"keepBaseCurrency" yaml:"keepBaseCurrency"`

	ReinvestRate fixedpoint.Value `json:"reinvestRate,omitempty" yaml:"reinvestRate"`

	ReinvBolInterval types.IntervalWindow `json:"ReinvBolInterval"`
	LockReinvPrice   fixedpoint.Value     `json:"LockReinvPrice"`

	ReinvEmaIndicator *indicator.BOLL

	state *State

	// orderStore is used to store all the created orders, so that we can filter the trades.
	orderStore *bbgo.OrderStore

	// activeOrders is the locally maintained active Order book of the maker orders.
	activeOrders *bbgo.LocalActiveOrderBook

	tradeCollector *bbgo.TradeCollector

	// groupID is the group ID used for the strategy instance for canceling orders
	groupID uint32
}

func (s *Strategy) ID() string {
	return ID
}

func (s *Strategy) Validate() error {
	if s.UpperPrice == 0 {
		return errors.New("upperPrice can not be zero, you forgot to set?")
	}
	if s.LowerPrice == 0 {
		return errors.New("lowerPrice can not be zero, you forgot to set?")
	}
	if s.UpperPrice <= s.LowerPrice {
		return fmt.Errorf("upperPrice (%f) should not be less than or equal to lowerPrice (%f)", s.UpperPrice.Float64(), s.LowerPrice.Float64())
	}

	//s.ProfitSpread <= 0 &&
	if s.ProfitSpreadRate <= 0 {
		// If profitSpread is empty or its value is negative
		return fmt.Errorf("one of profit or profitspreadrate spread should bigger than 0")
	}

	if s.Quantity == 0 && s.QuantityScale == nil {
		return fmt.Errorf("quantity or scaleQuantity can not be zero")
	}

	return nil
}

func (s *Strategy) _calcProfitSpread(price fixedpoint.Value, sideType types.SideType) fixedpoint.Value {

	if sideType == types.SideTypeSell {
		return price - price.Div(fixedpoint.NewFromInt(1)+s.ProfitSpreadRate)
	} else if sideType == types.SideTypeBuy {
		return price.Mul(fixedpoint.NewFromInt(1)+s.ProfitSpreadRate) - price
	}

	panic("not handled side")
}

// InstanceID returns the instance identifier from the current grid configuration parameters
func (s *Strategy) InstanceID() string {
	return fmt.Sprintf("%s-%s-%d-%d-%d", ID, s.Symbol, s.GridNum, s.UpperPrice, s.LowerPrice)
}

func (s *Strategy) InitGrids() error {

	priceRange := s.UpperPrice - s.LowerPrice
	numGrids := fixedpoint.NewFromInt(s.GridNum)
	gridSpread := priceRange.Div(numGrids)

	startPrice := fixedpoint.Min(s.UpperPrice,
		s.LowerPrice)

	if startPrice < s.LowerPrice {
		return fmt.Errorf("start price %f exceeded the lower price boundary %f",
			startPrice.Float64(),
			s.UpperPrice.Float64())
	}

	s.state.Cells = nil
	for price := startPrice; s.UpperPrice > price; price += gridSpread {
		spread := s._calcProfitSpread(price, types.SideTypeBuy)
		var cell = GridCell{
			Strategy:      s,
			Symbol:        s.Symbol,
			Market:        s.Market,
			BuyPrice:      price,
			Quantity:      s.Quantity,
			SellPrice:     price + spread,
			ProfitSpread:  spread,
			Orders:        map[string]*ICellOrder{},
			OneTimeOrders: map[string]*ICellOrder{},
			Profits: &AssetMap{
				s.Market.QuoteCurrency: &types.Asset{Currency: s.Market.QuoteCurrency, Total: fixedpoint.NewFromInt(0)},
				s.Market.BaseCurrency:  &types.Asset{Currency: s.Market.BaseCurrency, Total: fixedpoint.NewFromInt(0)},
			},
			Cost: &AssetMap{
				s.Market.QuoteCurrency: &types.Asset{Currency: s.Market.QuoteCurrency, Total: price.Mul(s.Quantity)},
				s.Market.BaseCurrency:  &types.Asset{Currency: s.Market.BaseCurrency, Total: s.Quantity},
			},
		}

		s.state.Cells = append(s.state.Cells, cell)
	}

	return nil
}

func (s *Strategy) generateInitOrders(currentPrice fixedpoint.Value) ([]ICellOrder, error) {
	var orders []ICellOrder
	for ind := range s.state.Cells {
		cell := &s.state.Cells[ind]
		orders = append(orders, cell.generateInitOrders(currentPrice)...)
	}
	return orders, nil
}

func (s *Strategy) handleCounterOrdersGenerate(ctx context.Context) {
	for _, cell := range s.state.Cells {
		var nextOrders = cell.generateOrders()

		for _, o := range nextOrders {
			createOrders, err := s.OrderExecutor.SubmitOrders(ctx, o.GetOrder())
			if err != nil {
				continue
			}
			o.UpdateClientOrderId(createOrders[0].ClientOrderID)
			o.GetCell().commitOpenOrder(&o)
			o.GetCell().commitOrderClosed(o.GetPrevOrder())
			s.activeOrders.Add(createOrders...)
		}
	}
}

func (s *Strategy) placeGridOrders(orderExecutor bbgo.OrderExecutor, session *bbgo.ExchangeSession) error {
	orderLock.Lock()
	defer orderLock.Unlock()

	log.Infof("placing grid orders on side %s...", s.Side)

	lastPrice, ok := session.LastPrice(s.Symbol)
	if !ok {

		return fmt.Errorf("cant get last price")
	}

	orderForms, err := s.generateInitOrders(fixedpoint.NewFromFloat(lastPrice))
	if err != nil {
		return err
	}

	s.handleCounterOrdersGenerate(context.Background())

	//if len(orderForms) == 0 {
	//	return errors.New("none of Order is generated")
	//}

	log.Infof("submitting %d orders...", len(orderForms))

	for ind := range orderForms {
		form := &orderForms[ind]

		order := (*form).GetOrder()
		createdOrders, err := orderExecutor.SubmitOrders(context.Background(), order)
		if err != nil {
			log.WithError(err)
			continue
		}
		(*form).UpdateClientOrderId(createdOrders[0].ClientOrderID)
		(*form).GetCell().commitOpenOrder(form)
		s.activeOrders.Add(createdOrders...)
	}

	return nil
}

var zeroiw = types.IntervalWindow{}

func (s *Strategy) Subscribe(session *bbgo.ExchangeSession) {
	session.Subscribe(types.KLineChannel, s.Symbol, types.SubscribeOptions{Interval: "1m"})

	if s.ReinvBolInterval != zeroiw {
		session.Subscribe(types.KLineChannel, s.Symbol,
			types.SubscribeOptions{Interval: string(s.ReinvBolInterval.Interval)})
	}
}

func (s *Strategy) LoadState() error {
	instanceID := s.InstanceID()

	var state State
	if s.Persistence != nil {
		if err := s.Persistence.Load(&state, ID, instanceID); err != nil {
			if err != service.ErrPersistenceNotExists {
				return errors.Wrapf(err, "status load error")
			}

			s.state = &State{
				Position: types.NewPositionFromMarket(s.Market),
			}
		} else {
			s.state = &state
		}
	} else {
		s.state = &State{}
	}

	s.state.Profits = &AssetMap{
		s.Market.QuoteCurrency: &types.Asset{Currency: s.Market.QuoteCurrency, Total: fixedpoint.NewFromInt(0)},
		s.Market.BaseCurrency:  &types.Asset{Currency: s.Market.BaseCurrency, Total: fixedpoint.NewFromInt(0)},
	}

	s.state.ReinvestingProfits = &AssetMap{
		s.Market.QuoteCurrency: &types.Asset{Currency: s.Market.QuoteCurrency, Total: fixedpoint.NewFromInt(0)},
		s.Market.BaseCurrency:  &types.Asset{Currency: s.Market.BaseCurrency, Total: fixedpoint.NewFromInt(0)},
	}

	s.state.Fees = &AssetMap{
		s.Market.QuoteCurrency: &types.Asset{Currency: s.Market.QuoteCurrency, Total: fixedpoint.NewFromInt(0)},
		s.Market.BaseCurrency:  &types.Asset{Currency: s.Market.BaseCurrency, Total: fixedpoint.NewFromInt(0)},
	}

	s.state.TradeCount = 0

	s.state.LockProfits = &AssetMap{
		s.Market.QuoteCurrency: &types.Asset{Currency: s.Market.QuoteCurrency, Total: fixedpoint.NewFromInt(0)},
		s.Market.BaseCurrency:  &types.Asset{Currency: s.Market.BaseCurrency, Total: fixedpoint.NewFromInt(0)},
	}

	// init profit stats
	s.state.ProfitStats.Init(s.Market)

	return nil
}

func (s *Strategy) SaveState() error {
	if s.Persistence != nil {
		log.Infof("backing up grid status...")

		instanceID := s.InstanceID()
		//submitOrders := s.activeOrders.Backup()
		//s.status.Orders = submitOrders

		if err := s.Persistence.Save(s.state, ID, instanceID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Strategy) handleFilledOrder(o types.Order) {
	ctx := context.Background()
	s._handleFilledSubmitOrder(o, ctx)
	return

}

func (s *Strategy) _handleFilledSubmitOrder(o types.Order, ctx context.Context) {
	orderLock.Lock()
	defer orderLock.Unlock()
	for _, cell := range s.state.Cells {
		//if the Order didn't report to the Cell , it will skip then.
		orders := cell.findOrders(o)
		if len(orders) == 0 {
			continue
		}

		cell.commitOrderFilled(o)

		var nextOrders = cell.generateOrders()

		for ind := range nextOrders {
			o2 := nextOrders[ind]
			createOrders, err := s.OrderExecutor.SubmitOrders(ctx, o2.GetOrder())
			if err != nil {
				continue
			}
			o2.UpdateClientOrderId(createOrders[0].ClientOrderID)
			o2.GetCell().commitOpenOrder(&o2)
			o2.GetCell().commitOrderClosed(o2.GetPrevOrder())
			s.activeOrders.Add(createOrders...)
		}
	}
}

func (s *Strategy) Run(ctx context.Context, orderExecutor bbgo.OrderExecutor, session *bbgo.ExchangeSession) error {
	//TODO: handle fee calculate

	// do some basic validation
	if s.GridNum == 0 {
		s.GridNum = 10
	}

	if s.Side == "" {
		s.Side = types.SideTypeBoth
	}

	instanceID := s.InstanceID()
	s.groupID = max.GenerateGroupID(instanceID)
	log.Infof("using group id %d from fnv(%s)", s.groupID, instanceID)

	if err := s.LoadState(); err != nil {
		return err
	}

	s.Notify("grid %s position", s.Symbol, s.state.Position)

	s.orderStore = bbgo.NewOrderStore(s.Symbol)
	s.orderStore.BindStream(session.UserDataStream)

	// we don't persist orders so that we can not clear the previous orders for now. just need time to support this.
	s.activeOrders = bbgo.NewLocalActiveOrderBook()
	s.activeOrders.OnFilled(s.handleFilledOrder)
	s.activeOrders.BindStream(session.UserDataStream)

	s.tradeCollector = bbgo.NewTradeCollector(s.Symbol, s.state.Position, s.orderStore)

	s.tradeCollector.OnTrade(func(trade types.Trade) {
		(*s.state.Fees)[trade.FeeCurrency].Total += fixedpoint.NewFromFloat(trade.Fee)
		s.state.TradeCount++
		s.Notifiability.Notify(trade)
		s.state.ProfitStats.AddTrade(trade)
	})

	if s.TradeService != nil {
		s.tradeCollector.OnTrade(func(trade types.Trade) {
			if err := s.TradeService.Mark(ctx, trade.ID, ID); err != nil {
				log.WithError(err).Error("trade mark error")
			}
		})
	}

	s.tradeCollector.OnPositionUpdate(func(position *types.Position) {
		s.Notifiability.Notify(position)
	})
	s.tradeCollector.BindStream(session.UserDataStream)
	s.InitGrids()

	standardIndicatorSet, ok := session.StandardIndicatorSet(s.Symbol)
	if !ok {
		return fmt.Errorf("standardIndicatorSet is nil, symbol %s", s.Symbol)
	}

	if s.ReinvBolInterval != zeroiw {
		s.ReinvEmaIndicator = standardIndicatorSet.BOLL(s.ReinvBolInterval, 1.0)
	}

	s.Graceful.OnShutdown(func(ctx context.Context, wg *sync.WaitGroup) {
		defer wg.Done()

		if err := s.SaveState(); err != nil {
			log.WithError(err).Errorf("can not save status: %+v", s.state)
		} else {
			s.Notify("%s: %s grid is saved", ID, s.Symbol)
		}

		// now we can cancel the open orders
		log.Infof("canceling active orders...")
		if err := session.Exchange.CancelOrders(ctx, s.activeOrders.Orders()...); err != nil {
			log.WithError(err).Errorf("cancel Order error")
		}
	})

	session.UserDataStream.OnStart(func() {
		// update grid
		s.placeGridOrders(orderExecutor, session)
	})

	if s.CatchUp {
		session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
			if kline.Interval == types.Interval1m {
				// update grid
				s.placeGridOrders(orderExecutor, session)

				//		session.Exchange.QueryOpenOrders()
				// re-check orders
			}
		})
	}

	return nil
}
