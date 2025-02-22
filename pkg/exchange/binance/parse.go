package binance

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/adshao/go-binance/v2/futures"

	"github.com/adshao/go-binance/v2"
	"github.com/valyala/fastjson"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/c9s/bbgo/pkg/util"
)

/*

executionReport

{
  "e": "executionReport",        // Event type
  "E": 1499405658658,            // Event time
  "s": "ETHBTC",                 // Symbol
  "c": "mUvoqJxFIILMdfAW5iGSOW", // Client order ID
  "S": "BUY",                    // Side
  "o": "LIMIT",                  // Order type
  "f": "GTC",                    // Time in force
  "q": "1.00000000",             // Order quantity
  "p": "0.10264410",             // Order price
  "P": "0.00000000",             // Stop price
  "F": "0.00000000",             // Iceberg quantity
  "g": -1,                       // OrderListId
  "C": null,                     // Original client order ID; This is the ID of the order being canceled
  "x": "NEW",                    // Current execution type
  "X": "NEW",                    // Current order status
  "r": "NONE",                   // Order reject reason; will be an error code.
  "i": 4293153,                  // Order ID
  "l": "0.00000000",             // Last executed quantity
  "z": "0.00000000",             // Cumulative filled quantity
  "L": "0.00000000",             // Last executed price
  "n": "0",                      // Commission amount
  "N": null,                     // Commission asset
  "T": 1499405658657,            // Transaction time
  "t": -1,                       // Trade ID
  "I": 8641984,                  // Ignore
  "w": true,                     // Is the order on the book?
  "m": false,                    // Is this trade the maker side?
  "M": false,                    // Ignore
  "O": 1499405658657,            // Order creation time
  "Z": "0.00000000",             // Cumulative quote asset transacted quantity
  "Y": "0.00000000",             // Last quote asset transacted quantity (i.e. lastPrice * lastQty)
  "Q": "0.00000000"              // Quote Order Qty
}
*/
type ExecutionReportEvent struct {
	EventBase

	Symbol string `json:"s"`
	Side   string `json:"S"`

	ClientOrderID         string `json:"c"`
	OriginalClientOrderID string `json:"C"`

	OrderType         string `json:"o"`
	OrderCreationTime int64  `json:"O"`

	TimeInForce     string `json:"f"`
	IcebergQuantity string `json:"F"`

	OrderQuantity      string `json:"q"`
	QuoteOrderQuantity string `json:"Q"`

	OrderPrice string `json:"p"`
	StopPrice  string `json:"P"`

	IsOnBook bool `json:"w"`

	IsMaker bool `json:"m"`
	Ignore  bool `json:"M"`

	CommissionAmount string `json:"n"`
	CommissionAsset  string `json:"N"`

	CurrentExecutionType string `json:"x"`
	CurrentOrderStatus   string `json:"X"`

	OrderID int64 `json:"i"`
	Ignored int64 `json:"I"`

	TradeID         int64 `json:"t"`
	TransactionTime int64 `json:"T"`

	LastExecutedQuantity string `json:"l"`
	LastExecutedPrice    string `json:"L"`

	CumulativeFilledQuantity               string `json:"z"`
	CumulativeQuoteAssetTransactedQuantity string `json:"Z"`

	LastQuoteAssetTransactedQuantity string `json:"Y"`
}

func (e *ExecutionReportEvent) Order() (*types.Order, error) {

	switch e.CurrentExecutionType {
	case "NEW", "CANCELED", "REJECTED", "EXPIRED":
	case "REPLACED":
	case "TRADE": // For Order FILLED status. And the order has been completed.
	default:
		return nil, errors.New("execution report type is not for order")
	}

	orderCreationTime := time.Unix(0, e.OrderCreationTime*int64(time.Millisecond))
	return &types.Order{
		Exchange: types.ExchangeBinance,
		SubmitOrder: types.SubmitOrder{
			Symbol:        e.Symbol,
			ClientOrderID: e.ClientOrderID,
			Side:          toGlobalSideType(binance.SideType(e.Side)),
			Type:          toGlobalOrderType(binance.OrderType(e.OrderType)),
			Quantity:      util.MustParseFloat(e.OrderQuantity),
			Price:         util.MustParseFloat(e.OrderPrice),
			TimeInForce:   e.TimeInForce,
		},
		OrderID:          uint64(e.OrderID),
		Status:           toGlobalOrderStatus(binance.OrderStatusType(e.CurrentOrderStatus)),
		ExecutedQuantity: util.MustParseFloat(e.CumulativeFilledQuantity),
		CreationTime:     types.Time(orderCreationTime),
	}, nil
}

func (e *ExecutionReportEvent) Trade() (*types.Trade, error) {
	if e.CurrentExecutionType != "TRADE" {
		return nil, errors.New("execution report is not a trade")
	}

	tt := time.Unix(0, e.TransactionTime*int64(time.Millisecond))
	return &types.Trade{
		ID:            uint64(e.TradeID),
		Exchange:      types.ExchangeBinance,
		Symbol:        e.Symbol,
		OrderID:       uint64(e.OrderID),
		Side:          toGlobalSideType(binance.SideType(e.Side)),
		Price:         util.MustParseFloat(e.LastExecutedPrice),
		Quantity:      util.MustParseFloat(e.LastExecutedQuantity),
		QuoteQuantity: util.MustParseFloat(e.LastQuoteAssetTransactedQuantity),
		IsBuyer:       e.Side == "BUY",
		IsMaker:       e.IsMaker,
		Time:          types.Time(tt),
		Fee:           util.MustParseFloat(e.CommissionAmount),
		FeeCurrency:   e.CommissionAsset,
	}, nil
}

/*
balanceUpdate

{
  "e": "balanceUpdate",         //KLineEvent Type
  "E": 1573200697110,           //KLineEvent Time
  "a": "BTC",                   //Asset
  "d": "100.00000000",          //Balance Delta
  "T": 1573200697068            //Clear Time
}
*/
type BalanceUpdateEvent struct {
	EventBase

	Asset     string `json:"a"`
	Delta     string `json:"d"`
	ClearTime int64  `json:"T"`
}

/*

outboundAccountInfo

{
  "e": "outboundAccountInfo",   // KLineEvent type
  "E": 1499405658849,           // KLineEvent time
  "m": 0,                       // Maker commission rate (bips)
  "t": 0,                       // Taker commission rate (bips)
  "b": 0,                       // Buyer commission rate (bips)
  "s": 0,                       // Seller commission rate (bips)
  "T": true,                    // Can trade?
  "W": true,                    // Can withdraw?
  "D": true,                    // Can deposit?
  "u": 1499405658848,           // Time of last account update
  "B": [                        // AccountBalances array
    {
      "a": "LTC",               // Asset
      "f": "17366.18538083",    // Free amount
      "l": "0.00000000"         // Locked amount
    },
    {
      "a": "BTC",
      "f": "10537.85314051",
      "l": "2.19464093"
    },
    {
      "a": "ETH",
      "f": "17902.35190619",
      "l": "0.00000000"
    },
    {
      "a": "BNC",
      "f": "1114503.29769312",
      "l": "0.00000000"
    },
    {
      "a": "NEO",
      "f": "0.00000000",
      "l": "0.00000000"
    }
  ],
  "P": [                       // Account Permissions
        "SPOT"
  ]
}

*/
type Balance struct {
	Asset  string           `json:"a"`
	Free   fixedpoint.Value `json:"f"`
	Locked fixedpoint.Value `json:"l"`
}

type OutboundAccountPositionEvent struct {
	EventBase

	LastAccountUpdateTime int       `json:"u"`
	Balances              []Balance `json:"B,omitempty"`
}

type OutboundAccountInfoEvent struct {
	EventBase

	MakerCommissionRate  int `json:"m"`
	TakerCommissionRate  int `json:"t"`
	BuyerCommissionRate  int `json:"b"`
	SellerCommissionRate int `json:"s"`

	CanTrade    bool `json:"T"`
	CanWithdraw bool `json:"W"`
	CanDeposit  bool `json:"D"`

	LastAccountUpdateTime int `json:"u"`

	Balances    []Balance `json:"B,omitempty"`
	Permissions []string  `json:"P,omitempty"`
}

type ResultEvent struct {
	Result interface{} `json:"result,omitempty"`
	ID     int         `json:"id"`
}

func ParseEvent(message string) (interface{}, error) {
	val, err := fastjson.Parse(message)

	if err != nil {
		return nil, err
	}

	//res, err := json.MarshalIndent(message, "", "  ")
	//if err != nil {
	//	log.Fatal(err)
	//}
	//str := strings.ReplaceAll(string(res), "\\", "")
	//fmt.Println(str)
	eventType := string(val.GetStringBytes("e"))
	if eventType == "" && IsBookTicker(val) {
		eventType = "bookticker"
	}

	switch eventType {
	case "kline":
		var event KLineEvent
		err := json.Unmarshal([]byte(message), &event)
		return &event, err
	case "bookticker":
		var event BookTickerEvent
		err := json.Unmarshal([]byte(message), &event)
		event.Event = eventType
		return &event, err

	case "outboundAccountPosition":
		var event OutboundAccountPositionEvent
		err := json.Unmarshal([]byte(message), &event)
		return &event, err

	case "outboundAccountInfo":
		var event OutboundAccountInfoEvent
		err := json.Unmarshal([]byte(message), &event)
		return &event, err

	case "balanceUpdate":
		var event BalanceUpdateEvent
		err := json.Unmarshal([]byte(message), &event)
		return &event, err

	case "executionReport":
		var event ExecutionReportEvent
		err := json.Unmarshal([]byte(message), &event)
		return &event, err

	case "depthUpdate":
		return parseDepthEvent(val)

	case "markPriceUpdate":
		var event MarkPriceUpdateEvent
		err := json.Unmarshal([]byte(message), &event)
		return &event, err

	// Binance futures data --------------
	case "continuousKline":
		var event ContinuousKLineEvent
		err := json.Unmarshal([]byte(message), &event)
		return &event, err

	case "ORDER_TRADE_UPDATE":
		var event OrderTradeUpdateEvent
		err := json.Unmarshal([]byte(message), &event)
		return &event, err

	// Event: Balance and Position Update
	case "ACCOUNT_UPDATE":
		var event AccountUpdateEvent
		err := json.Unmarshal([]byte(message), &event)
		return &event, err

	// Event: Order Update
	case "ACCOUNT_CONFIG_UPDATE":
		var event AccountConfigUpdateEvent
		err := json.Unmarshal([]byte(message), &event)
		return &event, err

	default:
		id := val.GetInt("id")
		if id > 0 {
			return &ResultEvent{ID: id}, nil
		}
	}

	return nil, fmt.Errorf("unsupported message: %s", message)
}

// IsBookTicker document ref :https://binance-docs.github.io/apidocs/spot/en/#individual-symbol-book-ticker-streams
//use key recognition because there's no identify in the content.
func IsBookTicker(val *fastjson.Value) bool {
	return !val.Exists("e") && val.Exists("u") &&
		val.Exists("s") && val.Exists("b") &&
		val.Exists("B") && val.Exists("a") && val.Exists("A")
}

type DepthEntry struct {
	PriceLevel fixedpoint.Value
	Quantity   fixedpoint.Value
}

type DepthEvent struct {
	EventBase

	Symbol        string `json:"s"`
	FirstUpdateID int64  `json:"U"`
	FinalUpdateID int64  `json:"u"`

	Bids types.PriceVolumeSlice `json:"b"`
	Asks types.PriceVolumeSlice `json:"a"`
}

func (e *DepthEvent) String() (o string) {
	o += fmt.Sprintf("Depth %s bid/ask = ", e.Symbol)

	if len(e.Bids) == 0 {
		o += "empty"
	} else {
		o += e.Bids[0].Price.String()
	}

	o += "/"

	if len(e.Asks) == 0 {
		o += "empty"
	} else {
		o += e.Asks[0].Price.String()
	}

	o += fmt.Sprintf(" %d ~ %d", e.FirstUpdateID, e.FinalUpdateID)
	return o
}

func (e *DepthEvent) OrderBook() (book types.SliceOrderBook, err error) {
	book.Symbol = e.Symbol

	// already in descending order
	book.Bids = e.Bids
	book.Asks = e.Asks
	return book, err
}

func parseDepthEntry(val *fastjson.Value) (*types.PriceVolume, error) {
	arr, err := val.Array()
	if err != nil {
		return nil, err
	}

	if len(arr) < 2 {
		return nil, errors.New("incorrect depth entry element length")
	}

	price, err := fixedpoint.NewFromString(string(arr[0].GetStringBytes()))
	if err != nil {
		return nil, err
	}

	quantity, err := fixedpoint.NewFromString(string(arr[1].GetStringBytes()))
	if err != nil {
		return nil, err
	}

	return &types.PriceVolume{
		Price:  price,
		Volume: quantity,
	}, nil
}

func parseDepthEvent(val *fastjson.Value) (*DepthEvent, error) {
	var err error
	var depth = &DepthEvent{
		EventBase: EventBase{
			Event: string(val.GetStringBytes("e")),
			Time:  val.GetInt64("E"),
		},
		Symbol:        string(val.GetStringBytes("s")),
		FirstUpdateID: val.GetInt64("U"),
		FinalUpdateID: val.GetInt64("u"),
	}

	for _, ev := range val.GetArray("b") {
		entry, err2 := parseDepthEntry(ev)
		if err2 != nil {
			err = err2
			continue
		}

		depth.Bids = append(depth.Bids, *entry)
	}

	for _, ev := range val.GetArray("a") {
		entry, err2 := parseDepthEntry(ev)
		if err2 != nil {
			err = err2
			continue
		}

		depth.Asks = append(depth.Asks, *entry)
	}

	return depth, err
}

type KLine struct {
	StartTime int64 `json:"t"`
	EndTime   int64 `json:"T"`

	Symbol   string `json:"s"`
	Interval string `json:"i"`

	Open  fixedpoint.Value `json:"o"`
	Close fixedpoint.Value `json:"c"`
	High  fixedpoint.Value `json:"h"`
	Low   fixedpoint.Value `json:"l"`

	Volume      fixedpoint.Value `json:"v"` // base asset volume (like 10 BTC)
	QuoteVolume fixedpoint.Value `json:"q"` // quote asset volume

	TakerBuyBaseAssetVolume  fixedpoint.Value `json:"V"` // taker buy base asset volume (like 10 BTC)
	TakerBuyQuoteAssetVolume fixedpoint.Value `json:"Q"` // taker buy quote asset volume (like 1000USDT)

	LastTradeID    int   `json:"L"`
	NumberOfTrades int64 `json:"n"`
	Closed         bool  `json:"x"`
}

/*

kline

{
  "e": "kline",     // KLineEvent type
  "E": 123456789,   // KLineEvent time
  "s": "BNBBTC",    // Symbol
  "k": {
    "t": 123400000, // Kline start time
    "T": 123460000, // Kline close time
    "s": "BNBBTC",  // Symbol
    "i": "1m",      // Interval
    "f": 100,       // First trade ID
    "L": 200,       // Last trade ID
    "o": "0.0010",  // Open price
    "c": "0.0020",  // Close price
    "h": "0.0025",  // High price
    "l": "0.0015",  // Low price
    "v": "1000",    // Base asset volume
    "n": 100,       // Number of trades
    "x": false,     // Is this kline closed?
    "q": "1.0000",  // Quote asset volume
    "V": "500",     // Taker buy base asset volume
    "Q": "0.500",   // Taker buy quote asset volume
    "B": "123456"   // Ignore
  }
}

*/

type KLineEvent struct {
	EventBase
	Symbol string `json:"s"`
	KLine  KLine  `json:"k,omitempty"`
}

func (k *KLine) KLine() types.KLine {
	return types.KLine{
		Exchange:                 types.ExchangeBinance,
		Symbol:                   k.Symbol,
		Interval:                 types.Interval(k.Interval),
		StartTime:                types.NewTimeFromUnix(0, k.StartTime*int64(time.Millisecond)),
		EndTime:                  types.NewTimeFromUnix(0, k.EndTime*int64(time.Millisecond)),
		Open:                     k.Open.Float64(),
		Close:                    k.Close.Float64(),
		High:                     k.High.Float64(),
		Low:                      k.Low.Float64(),
		Volume:                   k.Volume.Float64(),
		QuoteVolume:              k.QuoteVolume.Float64(),
		TakerBuyBaseAssetVolume:  k.TakerBuyBaseAssetVolume.Float64(),
		TakerBuyQuoteAssetVolume: k.TakerBuyQuoteAssetVolume.Float64(),
		LastTradeID:              uint64(k.LastTradeID),
		NumberOfTrades:           uint64(k.NumberOfTrades),
		Closed:                   k.Closed,
	}
}

type MarkPriceUpdateEvent struct {
	EventBase

	Symbol string `json:"s"`

	MarkPrice      fixedpoint.Value `json:"p"`
	IndexPrice     fixedpoint.Value `json:"i"`
	EstimatedPrice fixedpoint.Value `json:"P"`

	FundingRate     fixedpoint.Value `json:"r"`
	NextFundingTime int64            `json:"T"`
}

/*
{
  "e": "markPriceUpdate",     // Event type
  "E": 1562305380000,         // Event time
  "s": "BTCUSDT",             // Symbol
  "p": "11794.15000000",      // Mark price
  "i": "11784.62659091",      // Index price
  "P": "11784.25641265",      // Estimated Settle Price, only useful in the last hour before the settlement starts
  "r": "0.00038167",          // Funding rate
  "T": 1562306400000          // Next funding time
}
*/

type ContinuousKLineEvent struct {
	EventBase
	Symbol string `json:"ps"`
	ct     string `json:"ct"`
	KLine  KLine  `json:"k,omitempty"`
}

/*
{
  "e":"continuous_kline",   // Event type
  "E":1607443058651,        // Event time
  "ps":"BTCUSDT",           // Pair
  "ct":"PERPETUAL"          // Contract type
  "k":{
    "t":1607443020000,      // Kline start time
    "T":1607443079999,      // Kline close time
    "i":"1m",               // Interval
    "f":116467658886,       // First trade ID
    "L":116468012423,       // Last trade ID
    "o":"18787.00",         // Open price
    "c":"18804.04",         // Close price
    "h":"18804.04",         // High price
    "l":"18786.54",         // Low price
    "v":"197.664",          // volume
    "n": 543,               // Number of trades
    "x":false,              // Is this kline closed?
    "q":"3715253.19494",    // Quote asset volume
    "V":"184.769",          // Taker buy volume
    "Q":"3472925.84746",    //Taker buy quote asset volume
    "B":"0"                 // Ignore
  }
}
*/

// Similar to the ExecutionReportEvent's fields. But with totally different json key.
// e.g., Stop price. So that, we can not merge them.
type OrderTrade struct {
	Symbol           string `json:"s"`
	ClientOrderID    string `json:"c"`
	Side             string `json:"S"`
	OrderType        string `json:"o"`
	TimeInForce      string `json:"f"`
	OriginalQuantity string `json:"q"`
	OriginalPrice    string `json:"p"`

	AveragePrice         string `json:"ap"`
	StopPrice            string `json:"sp"`
	CurrentExecutionType string `json:"x"`
	CurrentOrderStatus   string `json:"X"`

	OrderId                        int64  `json:"i"`
	OrderLastFilledQuantity        string `json:"l"`
	OrderFilledAccumulatedQuantity string `json:"z"`
	LastFilledPrice                string `json:"L"`

	CommissionAmount string `json:"n"`
	CommissionAsset  string `json:"N"`

	OrderTradeTime int64 `json:"T"`
	TradeId        int64 `json:"t"`

	BidsNotional string `json:"b"`
	AskNotional  string `json:"a"`

	IsMaker      bool `json:"m"`
	IsReduceOnly bool ` json:"r"`

	StopPriceWorkingType string `json:"wt"`
	OriginalOrderType    string `json:"ot"`
	PositionSide         string `json:"ps"`
	RealizedProfit       string `json:"rp"`
}

type OrderTradeUpdateEvent struct {
	EventBase
	Transaction int64      `json:"T"`
	OrderTrade  OrderTrade `json:"o"`
}

// {

// 	"e":"ORDER_TRADE_UPDATE",     // Event Type
// 	"E":1568879465651,            // Event Time
// 	"T":1568879465650,            // Transaction Time
// 	"o":{
// 	  "s":"BTCUSDT",              // Symbol
// 	  "c":"TEST",                 // Client Order Id
// 		// special client order id:
// 		// starts with "autoclose-": liquidation order
// 		// "adl_autoclose": ADL auto close order
// 	  "S":"SELL",                 // Side
// 	  "o":"TRAILING_STOP_MARKET", // Order Type
// 	  "f":"GTC",                  // Time in Force
// 	  "q":"0.001",                // Original Quantity
// 	  "p":"0",                    // Original Price
// 	  "ap":"0",                   // Average Price
// 	  "sp":"7103.04",             // Stop Price. Please ignore with TRAILING_STOP_MARKET order
// 	  "x":"NEW",                  // Execution Type
// 	  "X":"NEW",                  // Order Status
// 	  "i":8886774,                // Order Id
// 	  "l":"0",                    // Order Last Filled Quantity
// 	  "z":"0",                    // Order Filled Accumulated Quantity
// 	  "L":"0",                    // Last Filled Price
// 	  "N":"USDT",             // Commission Asset, will not push if no commission
// 	  "n":"0",                // Commission, will not push if no commission
// 	  "T":1568879465651,          // Order Trade Time
// 	  "t":0,                      // Trade Id
// 	  "b":"0",                    // Bids Notional
// 	  "a":"9.91",                 // Ask Notional
// 	  "m":false,                  // Is this trade the maker side?
// 	  "R":false,                  // Is this reduce only
// 	  "wt":"CONTRACT_PRICE",      // Stop Price Working Type
// 	  "ot":"TRAILING_STOP_MARKET",    // Original Order Type
// 	  "ps":"LONG",                        // Position Side
// 	  "cp":false,                     // If Close-All, pushed with conditional order
// 	  "AP":"7476.89",             // Activation Price, only puhed with TRAILING_STOP_MARKET order
// 	  "cr":"5.0",                 // Callback Rate, only puhed with TRAILING_STOP_MARKET order
// 	  "rp":"0"                            // Realized Profit of the trade
// 	}

//   }

func (e *OrderTradeUpdateEvent) OrderFutures() (*types.Order, error) {

	switch e.OrderTrade.CurrentExecutionType {
	case "NEW", "CANCELED", "EXPIRED":
	case "CALCULATED - Liquidation Execution":
	case "TRADE": // For Order FILLED status. And the order has been completed.
	default:
		return nil, errors.New("execution report type is not for futures order")
	}

	orderCreationTime := time.Unix(0, e.OrderTrade.OrderTradeTime*int64(time.Millisecond))
	return &types.Order{
		Exchange: types.ExchangeBinance,
		SubmitOrder: types.SubmitOrder{
			Symbol:        e.OrderTrade.Symbol,
			ClientOrderID: e.OrderTrade.ClientOrderID,
			Side:          toGlobalFuturesSideType(futures.SideType(e.OrderTrade.Side)),
			Type:          toGlobalFuturesOrderType(futures.OrderType(e.OrderTrade.OrderType)),
			Quantity:      util.MustParseFloat(e.OrderTrade.OriginalQuantity),
			Price:         util.MustParseFloat(e.OrderTrade.OriginalPrice),
			TimeInForce:   e.OrderTrade.TimeInForce,
		},
		OrderID:          uint64(e.OrderTrade.OrderId),
		Status:           toGlobalFuturesOrderStatus(futures.OrderStatusType(e.OrderTrade.CurrentOrderStatus)),
		ExecutedQuantity: util.MustParseFloat(e.OrderTrade.OrderFilledAccumulatedQuantity),
		CreationTime:     types.Time(orderCreationTime),
	}, nil
}

type AccountUpdate struct {
	EventReasonType string                     `json:"m"`
	Balances        []*futures.Balance         `json:"B,omitempty"`
	Positions       []*futures.AccountPosition `json:"P,omitempty"`
}

type AccountUpdateEvent struct {
	EventBase
	Transaction int64 `json:"T"`

	AccountUpdate AccountUpdate `json:"a"`
}

type AccountConfig struct {
	Symbol   string           `json:"s"`
	Leverage fixedpoint.Value `json:"l"`
}

type AccountConfigUpdateEvent struct {
	EventBase
	Transaction int64 `json:"T"`

	AccountConfig AccountConfig `json:"ac"`
}

type EventBase struct {
	Event string `json:"e"` // event
	Time  int64  `json:"E"`
}

type BookTickerEvent struct {
	EventBase
	Symbol   string           `json:"s"`
	Buy      fixedpoint.Value `json:"b"`
	BuySize  fixedpoint.Value `json:"B"`
	Sell     fixedpoint.Value `json:"a"`
	SellSize fixedpoint.Value `json:"A"`
	//"u":400900217,     // order book updateId
	//"s":"BNBUSDT",     // symbol
	//"b":"25.35190000", // best bid price
	//"B":"31.21000000", // best bid qty
	//"a":"25.36520000", // best ask price
	//"A":"40.66000000"  // best ask qty
}

func (k *BookTickerEvent) BookTicker() types.BookTicker {
	return types.BookTicker{
		Symbol:   k.Symbol,
		Buy:      k.Buy,
		BuySize:  k.BuySize,
		Sell:     k.Sell,
		SellSize: k.SellSize,
	}
}
