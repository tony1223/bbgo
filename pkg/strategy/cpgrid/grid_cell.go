package cpgrid

import (
	"fmt"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/google/uuid"
)

type GridCell struct {
	Strategy      *Strategy
	Symbol        string
	Market        types.Market `json:"-" db:"-"`
	BuyPrice      fixedpoint.Value
	SellPrice     fixedpoint.Value
	ProfitSpread  fixedpoint.Value
	Orders        map[string]*ICellOrder
	OneTimeOrders map[string]*ICellOrder
	Profits       *AssetMap
	Cost          *AssetMap
	Quantity      fixedpoint.Value
	GroupID       uint32
}

func (c *GridCell) generateInitOrders(price fixedpoint.Value) []ICellOrder {
	//TODO: what if orders expired ?
	//clean archive orders
	for s, order := range c.Orders {
		if (*order).OrderState() == CellOrderStateClosed {
			delete(c.Orders, s)
		}
	}

	if len(c.Orders) > 0 { //dont generate init order when order exist
		return nil
	}
	return c._generateInitOrders(price)
}

func (c *GridCell) generateOrders() []ICellOrder {
	//TODO: what if orders expired ?
	//clean archive orders
	for s, order := range c.Orders {
		if (*order).OrderState() == CellOrderStateClosed {
			delete(c.Orders, s)
		}
	}

	if len(c.Orders) == 0 { //dont generate when order not exist
		return nil
	}

	var generatedOrders []ICellOrder
	for _, o := range c.Orders {
		order := *o
		if order.OrderState() == CellOrderStateFill {
			generatedOrders = append(generatedOrders, order.GenerateCounterOrder()...)
		}
	}

	return generatedOrders

}

func (c *GridCell) _generateInitOrders(currentPrice fixedpoint.Value) []ICellOrder {
	var orders []ICellOrder
	if currentPrice < c.SellPrice {
		//sell
		orders = append(orders, &CellOrder{
			Order: types.SubmitOrder{
				ClientOrderID: fmt.Sprintf("%s-%s", c.Symbol, uuid.New().String()),
				Symbol:        c.Symbol,
				Side:          types.SideTypeSell,
				Type:          types.OrderTypeLimit,
				Market:        c.Market,
				Quantity:      c.Quantity.Float64(),
				Price:         c.SellPrice.Float64(),
				TimeInForce:   "GTC",
				GroupID:       c.GroupID,
			},
			Cell:  c,
			State: CellOrderStateDraft,
			Cost: map[string]types.Asset{
				c.Strategy.BaseCurrency: types.Asset{
					Currency: c.Strategy.BaseCurrency,
					Total:    c.BuyPrice * c.Quantity,
				},
				c.Strategy.QuoteCurrency: types.Asset{
					Currency: c.Strategy.QuoteCurrency,
					Total:    c.Quantity,
				},
			},
		})
	} else {
		orders = append(orders, &CellOrder{
			Order: types.SubmitOrder{
				ClientOrderID: fmt.Sprintf("%s-%s", c.Symbol, uuid.New().String()),
				Symbol:        c.Symbol,
				Side:          types.SideTypeBuy,
				Type:          types.OrderTypeLimit,
				Market:        c.Market,
				Quantity:      c.Quantity.Float64(),
				Price:         c.BuyPrice.Float64(),
				TimeInForce:   "GTC",
				GroupID:       c.GroupID,
			},
			Cell:  c,
			State: CellOrderStateDraft,
			Cost: map[string]types.Asset{
				c.Strategy.BaseCurrency: types.Asset{
					Currency: c.Strategy.BaseCurrency,
					Total:    c.BuyPrice * c.Quantity,
				},
				c.Strategy.QuoteCurrency: types.Asset{
					Currency: c.Strategy.QuoteCurrency,
					Total:    c.Quantity,
				},
			},
		})
		//buy
	}
	return orders
}

func (c *GridCell) commitOpenOrder(form *ICellOrder) {
	c.Orders[(*form).GetOrder().ClientOrderID] = form
	(*form).UpdateOrderState(CellOrderStateOpen)

	state := c.Strategy.state
	reInvProfit := *state.ReinvestingProfits
	o := (*form).GetOrder()

	if o.Side == types.SideTypeSell {
		reInvProfit[c.Market.BaseCurrency].Total += fixedpoint.NewFromFloat(o.Quantity) - (*form).GetCost()[c.Market.QuoteCurrency].Total
	} else if o.Side == types.SideTypeBuy {
		reInvProfit[c.Market.QuoteCurrency].Total += (fixedpoint.NewFromFloat(o.Quantity) - (*form).GetCost()[c.Market.QuoteCurrency].Total).Mul(c.BuyPrice)
	}
}

func (c *GridCell) commitOrderFilled(order types.Order) {
	//nothing related to the grid , pass
	if cellOrder, ok := c.Orders[order.ClientOrderID]; ok {
		(*cellOrder).UpdateOrderState(CellOrderStateFill)
		c.UpdateProfit(cellOrder, order.SubmitOrder)
	}

	if cellOrder, ok := c.OneTimeOrders[order.ClientOrderID]; ok {
		(*cellOrder).UpdateOrderState(CellOrderStateFill)
		c.UpdateProfit(cellOrder, order.SubmitOrder)
	}

	return
}

func (c *GridCell) UpdateProfit(pCellOrder *ICellOrder, submitOrder types.SubmitOrder) {
	cellOrder := *pCellOrder
	if cellOrder.IsExcludeProfit() {
		log.Infof("Ignore profit with %s  ", submitOrder.String())
		return
	}

	//XRP / USD , XRP is base , USD is quote.

	gridBaseAsset := (*c.Profits)[c.Strategy.BaseCurrency]
	gridQuoteAsset := (*c.Profits)[c.Strategy.QuoteCurrency]

	baseAsset := (*c.Strategy.state.Profits)[c.Strategy.BaseCurrency]
	quoteAsset := (*c.Strategy.state.Profits)[c.Strategy.QuoteCurrency]

	baseLockAsset := (*c.Strategy.state.LockProfits)[c.Strategy.BaseCurrency]
	quoteLockAsset := (*c.Strategy.state.LockProfits)[c.Strategy.QuoteCurrency]

	reInvProfitBaseAsset := (*c.Strategy.state.ReinvestingProfits)[c.Strategy.BaseCurrency]
	reInvProfitQuoteAsset := (*c.Strategy.state.ReinvestingProfits)[c.Strategy.QuoteCurrency]

	var baseDiff fixedpoint.Value
	var quoteDiff fixedpoint.Value

	if submitOrder.Side == types.SideTypeSell {
		var reinvQuotes = fixedpoint.NewFromFloat(submitOrder.Quantity) - cellOrder.GetCost()[c.Strategy.QuoteCurrency].Total
		quoteDiff = fixedpoint.NewFromFloat(submitOrder.Quantity).Mul(c.ProfitSpread) + reinvQuotes.Mul(fixedpoint.NewFromFloat(submitOrder.Price))
		baseDiff = fixedpoint.NewFromInt(-1).Mul(reinvQuotes)
		reInvProfitBaseAsset.Total += baseDiff

	} else {
		diffQuantity := fixedpoint.NewFromFloat(submitOrder.Quantity - cellOrder.GetCost()[c.Strategy.QuoteCurrency].Total.Float64())
		//decrease the base asset for buying quote
		quoteDiff = fixedpoint.NewFromInt(-1).Mul(diffQuantity.Mul(fixedpoint.NewFromFloat(submitOrder.Price)))
		//increasing the quote
		baseDiff = diffQuantity

		reInvProfitQuoteAsset.Total += quoteDiff

	}

	gridBaseAsset.Total += baseDiff
	gridQuoteAsset.Total += quoteDiff

	var reInvBase = baseDiff
	var reInvQuote = quoteDiff

	if c.Strategy.KeepBaseCurrency {
		if baseDiff > 0 {
			reInvBase = baseDiff.Mul(c.Strategy.ReinvestRate)
		} else { // we should put it back when negative. (it means we flush back the reinvesting cost)
			reInvBase = baseDiff
		}
	} else {
		if quoteDiff > 0 {
			reInvQuote = quoteDiff.Mul(c.Strategy.ReinvestRate)
		} else { // we should put it back when negative. (it means we flush back the reinvesting cost)
			reInvQuote = quoteDiff
		}
	}

	baseLockAsset.Total += baseDiff - reInvBase
	quoteLockAsset.Total += quoteDiff - reInvQuote

	baseAsset.Total += reInvBase
	quoteAsset.Total += reInvQuote

	//TODO: update log
	msg := fmt.Sprintf("updating #profit with %s , reInvRate: %f , cell profit %s , grid trade count : %d,  grid locked profit : %s , grid active profit : %s , grid fee: %s  ",
		submitOrder.String(),
		c.Strategy.ReinvestRate.Float64(),
		c.Profits.String(),
		c.Strategy.state.TradeCount,
		c.Strategy.state.LockProfits.String(),
		c.Strategy.state.Profits.String(),
		c.Strategy.state.Fees.String())
	c.Strategy.Notify(msg)
	log.Infof(msg)

}

func (c *GridCell) findOrders(order types.Order) []*ICellOrder {
	if cellOrder, ok := c.Orders[order.ClientOrderID]; ok {
		return []*ICellOrder{cellOrder}
	}

	if cellOrder, ok := c.OneTimeOrders[order.ClientOrderID]; ok {
		return []*ICellOrder{cellOrder}
	}
	return nil
}

func (c *GridCell) commitOrderClosed(order *CellOrder) {
	if order != nil {
		(*order).UpdateOrderState(CellOrderStateClosed)
	}
}
