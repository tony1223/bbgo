package cpgrid

import (
	"fmt"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/google/uuid"
	"math"
)

type ICellOrder interface {
	GetOrder() types.SubmitOrder
	GetPrevOrder() *CellOrder
	UpdateClientOrderId(clientOrderId string)
	GetCell() *GridCell
	GenerateCounterOrder() []ICellOrder
	OrderState() CellOrderState
	UpdateOrderState(state CellOrderState)
	GetCost() types.AssetMap
	IsExcludeProfit() bool
	HasCounterOrder() bool
}
type CellOrderState string

const (
	CellOrderStateDraft         = CellOrderState("DRAFT")
	CellOrderStateChecking      = CellOrderState("CHECKING")
	CellOrderStateOpen          = CellOrderState("OPEN")
	CellOrderStatePartiallyFill = CellOrderState("PARTIALLY_FILL")
	CellOrderStateFill          = CellOrderState("FILL")
	CellOrderStateClosed        = CellOrderState("CLOSED")
	CellOrderStateCancel        = CellOrderState("CANCEL")
)

type CellOrder struct {
	Order            types.SubmitOrder
	ExecutedQuantity fixedpoint.Value
	PrevOrder        *CellOrder
	Cell             *GridCell
	State            CellOrderState
	Cost             types.AssetMap
	ExcludeProfit    bool
	hasCounterOrder  bool
}

func (c *CellOrder) HasCounterOrder() bool {
	return c.hasCounterOrder
}

func (c *CellOrder) IsExcludeProfit() bool {
	return c.ExcludeProfit
}

func (c *CellOrder) GetCost() types.AssetMap {
	return c.Cost
}

func (c *CellOrder) GetPrevOrder() *CellOrder {
	return c.PrevOrder
}

func (c *CellOrder) OrderState() CellOrderState {
	return c.State
}

func (c *CellOrder) UpdateOrderState(state CellOrderState) {
	c.State = state
}

func (c *CellOrder) GenerateCounterOrder() []ICellOrder {

	if !c.hasCounterOrder {
		return []ICellOrder{}
	}

	state := c.Cell.Strategy.state
	profit := *state.Profits
	reInvProfit := *state.ReinvestingProfits

	market := c.Cell.Market
	baseCurrency := market.BaseCurrency
	quoteCurrency := market.QuoteCurrency

	//XRP / USD , XRP is base , USD is quote.
	var workBaseProfit = profit[baseCurrency].Total - reInvProfit[baseCurrency].Total
	var workQuoteProfit = profit[quoteCurrency].Total - reInvProfit[quoteCurrency].Total

	var cell = c.Cell
	var orders []ICellOrder

	indicator := c.Cell.Strategy.ReinvEmaIndicator

	var merge bool = true
	if c.Order.Side == types.SideTypeBuy {
		var quantity = workBaseProfit.Float64() - math.Mod(workBaseProfit.Float64(), market.StepSize)

		var orderQuantity float64 = 0
		if merge {
			orderQuantity = cell.Quantity.Float64() + quantity
		} else {
			orderQuantity = cell.Quantity.Float64()
		}
		//sell
		orders = append(orders, &CellOrder{
			Order: types.SubmitOrder{
				ClientOrderID: fmt.Sprintf("%s", uuid.New().String()),
				Symbol:        cell.Symbol,
				Side:          types.SideTypeSell,
				Type:          types.OrderTypeLimit,
				Market:        cell.Market,
				Quantity:      orderQuantity,
				Price:         cell.SellPrice.Float64(),
				TimeInForce:   "GTC",
				GroupID:       cell.GroupID,
			},
			PrevOrder:       c,
			Cell:            cell,
			State:           CellOrderStateDraft,
			Cost:            c.GetCost(),
			hasCounterOrder: true,
		})

		if quantity > 0 && !merge {
			orders = append(orders, &CellOrder{
				Order: types.SubmitOrder{
					ClientOrderID: fmt.Sprintf("%s", uuid.New().String()),
					Symbol:        cell.Symbol,
					Side:          types.SideTypeSell,
					Type:          types.OrderTypeLimit,
					Market:        cell.Market,
					Quantity:      quantity,
					Price:         cell.SellPrice.Float64(),
					TimeInForce:   "GTC",
					GroupID:       cell.GroupID,
				},
				PrevOrder: nil,
				Cell:      cell,
				State:     CellOrderStateDraft,
				Cost: map[string]types.Asset{
					c.Cell.Strategy.BaseCurrency: types.Asset{
						Currency: c.Cell.Strategy.BaseCurrency,
						Total:    fixedpoint.NewFromInt(0),
					},
					c.Cell.Strategy.QuoteCurrency: types.Asset{
						Currency: c.Cell.Strategy.BaseCurrency,
						Total:    fixedpoint.NewFromInt(0),
					},
				},
				hasCounterOrder: false,
			})
		}

	} else {
		var quoteQuantity = workQuoteProfit.Div(cell.BuyPrice)
		var quantity = quoteQuantity.Float64() - math.Mod(quoteQuantity.Float64(), market.StepSize)
		// in case we dont buy in relative high price.
		if indicator != nil && indicator.LastUpBand() != 0 && indicator.LastUpBand() < cell.SellPrice.Float64() {
			quantity = 0
		}
		var orderQuantity float64 = 0
		if merge {
			orderQuantity = cell.Quantity.Float64() + quantity
		} else {
			orderQuantity = cell.Quantity.Float64()
		}
		orders = append(orders, &CellOrder{
			Order: types.SubmitOrder{
				ClientOrderID: fmt.Sprintf("%s", uuid.New().String()),
				Symbol:        cell.Symbol,
				Side:          types.SideTypeBuy,
				Type:          types.OrderTypeLimit,
				Market:        cell.Market,
				Quantity:      orderQuantity,
				Price:         cell.BuyPrice.Float64(),
				TimeInForce:   "GTC",
				GroupID:       cell.GroupID,
			},
			PrevOrder:       c,
			Cell:            cell,
			State:           CellOrderStateDraft,
			Cost:            c.GetCost(),
			hasCounterOrder: true,
		})

		if quantity > 0 && !merge {
			orders = append(orders, &CellOrder{
				Order: types.SubmitOrder{
					ClientOrderID: fmt.Sprintf("%s", uuid.New().String()),
					Symbol:        cell.Symbol,
					Side:          types.SideTypeBuy,
					Type:          types.OrderTypeLimit,
					Market:        cell.Market,
					Quantity:      quantity,
					Price:         cell.BuyPrice.Float64() - cell.ProfitSpread.Float64(),
					TimeInForce:   "GTC",
					GroupID:       cell.GroupID,
				},
				PrevOrder: nil,
				Cell:      cell,
				State:     CellOrderStateDraft,
				Cost: map[string]types.Asset{
					c.Cell.Strategy.BaseCurrency: types.Asset{
						Currency: c.Cell.Strategy.BaseCurrency,
						Total:    fixedpoint.NewFromInt(0),
					},
					c.Cell.Strategy.QuoteCurrency: types.Asset{
						Currency: c.Cell.Strategy.QuoteCurrency,
						Total:    fixedpoint.NewFromInt(0),
					},
				},
				hasCounterOrder: false,
			})
		}

		//buy
	}

	return orders

}

func (c *CellOrder) GetCell() *GridCell {
	return c.Cell
}

func (c *CellOrder) UpdateClientOrderId(clientOrderId string) {
	c.Order.ClientOrderID = clientOrderId
}

func (c *CellOrder) GetOrder() types.SubmitOrder {
	return c.Order
}
