package cpgrid

import (
	"context"
	"fmt"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/rand"
	"testing"
)

func TestInitGrids(t *testing.T) {

	s := Strategy{
		UpperPrice:       fixedpoint.NewFromFloat(0.85),
		LowerPrice:       fixedpoint.NewFromFloat(0.7),
		ProfitSpreadRate: fixedpoint.NewFromFloat(0.003),
		GridNum:          10,
	}
	if err := s.LoadState(); err != nil {
		assert.NoError(t, err)
	}

	s.InitGrids()
	for _, cell := range s.state.Cells {
		log.Infof("%f %f ", cell.BuyPrice.Float64(), cell.SellPrice.Float64())
	}

	assert.Equal(t, 0.700000, s.state.Cells[0].BuyPrice.Float64())
	assert.Equal(t, 0.715000, s.state.Cells[1].BuyPrice.Float64())
	assert.Equal(t, 0.730000, s.state.Cells[2].BuyPrice.Float64())
	assert.Equal(t, 0.745000, s.state.Cells[3].BuyPrice.Float64())
	assert.Equal(t, 0.760000, s.state.Cells[4].BuyPrice.Float64())
	assert.Equal(t, 0.775000, s.state.Cells[5].BuyPrice.Float64())
	assert.Equal(t, 0.790000, s.state.Cells[6].BuyPrice.Float64())
	assert.Equal(t, 0.805000, s.state.Cells[7].BuyPrice.Float64())
	assert.Equal(t, 0.820000, s.state.Cells[8].BuyPrice.Float64())
	assert.Equal(t, 0.835000, s.state.Cells[9].BuyPrice.Float64())

	assert.Equal(t, 0.702100, s.state.Cells[0].SellPrice.Float64())
	assert.Equal(t, 0.717145, s.state.Cells[1].SellPrice.Float64())
	assert.Equal(t, 0.732190, s.state.Cells[2].SellPrice.Float64())
	assert.Equal(t, 0.747235, s.state.Cells[3].SellPrice.Float64())
	assert.Equal(t, 0.762280, s.state.Cells[4].SellPrice.Float64())
	assert.Equal(t, 0.777325, s.state.Cells[5].SellPrice.Float64())
	assert.Equal(t, 0.792370, s.state.Cells[6].SellPrice.Float64())
	assert.Equal(t, 0.807415, s.state.Cells[7].SellPrice.Float64())
	assert.Equal(t, 0.822460, s.state.Cells[8].SellPrice.Float64())
	assert.Equal(t, 0.837505, s.state.Cells[9].SellPrice.Float64())

	assert.Equal(t, 10, len(s.state.Cells))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx
}

func TestGenerateOrders(t *testing.T) {

	s := Strategy{
		Symbol:   "XRPUSDT",
		Quantity: fixedpoint.NewFromFloat(10),
		Market: types.Market{
			Symbol:        "XRPUSDT",
			BaseCurrency:  "USDT",
			QuoteCurrency: "XRP",
			StepSize:      1.0,
		},
		ReinvestRate:     fixedpoint.NewFromInt(0),
		UpperPrice:       fixedpoint.NewFromFloat(0.85),
		LowerPrice:       fixedpoint.NewFromFloat(0.7),
		ProfitSpreadRate: fixedpoint.NewFromFloat(0.003),
		GridNum:          10,
	}
	if err := s.LoadState(); err != nil {
		assert.NoError(t, err)
	}

	s.InitGrids()

	orderForms, err := s.generateInitOrders(fixedpoint.NewFromFloat(0.77))
	assert.NoError(t, err)
	assert.Equal(t, 10, len(orderForms))

	assert.Equal(t, "XRPUSDT", orderForms[0].GetOrder().Symbol)
	assert.Equal(t, types.SideTypeBuy, orderForms[0].GetOrder().Side)

	assert.Equal(t, types.OrderTypeLimit, orderForms[0].GetOrder().Type)
	assert.Equal(t, "XRPUSDT", orderForms[0].GetOrder().Symbol)
	assert.Equal(t, float64(10), orderForms[0].GetOrder().Quantity)
	assert.Equal(t, 0.7, orderForms[0].GetOrder().Price)

	mockOpen(&orderForms)

	orderForms, err = s.generateInitOrders(fixedpoint.NewFromFloat(0.77))
	assert.NoError(t, err)
	assert.Equal(t, 0, len(orderForms))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx
}

func TestGenerateOrderAndCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := Strategy{
		Symbol:   "XRPUSDT",
		Quantity: fixedpoint.NewFromFloat(10),
		Market: types.Market{
			Symbol:        "XRPUSDT",
			BaseCurrency:  "XRP",
			QuoteCurrency: "USDT",
			StepSize:      1.0,
		},
		ReinvestRate:     fixedpoint.NewFromInt(0),
		UpperPrice:       fixedpoint.NewFromFloat(0.85),
		LowerPrice:       fixedpoint.NewFromFloat(0.7),
		ProfitSpreadRate: fixedpoint.NewFromFloat(0.003),
		GridNum:          10,
	}
	if err := s.LoadState(); err != nil {
		assert.NoError(t, err)
	}

	s.InitGrids()

	orderForms, err := s.generateInitOrders(fixedpoint.NewFromFloat(0.77))
	assert.NoError(t, err)
	//mock open
	mockOpen(&orderForms)

	order := orderForms[0]
	cell := order.GetCell()
	assert.Equal(t, CellOrderStateOpen, order.OrderState())

	mockGridFill(order.GetCell(), order)
	assert.Equal(t, CellOrderStateFill, order.OrderState())
	//it should be zero profit for first trade.
	profits := *cell.Profits
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.QuoteCurrency].Total)

	orderPointer := &order
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.021), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.021), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.042), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	defer cancel()
	_ = ctx
}

func TestGenerateOrderAndCommit2(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := Strategy{
		Symbol:   "XRPUSDT",
		Quantity: fixedpoint.NewFromFloat(10),
		Market: types.Market{
			Symbol:        "XRPUSDT",
			BaseCurrency:  "USDT",
			QuoteCurrency: "XRP",
			StepSize:      1.0,
		},
		ReinvestRate:     fixedpoint.NewFromInt(0),
		UpperPrice:       fixedpoint.NewFromFloat(0.85),
		LowerPrice:       fixedpoint.NewFromFloat(0.7),
		ProfitSpreadRate: fixedpoint.NewFromFloat(0.003),
		GridNum:          10,
	}
	if err := s.LoadState(); err != nil {
		assert.NoError(t, err)
	}

	s.InitGrids()

	orderForms, err := s.generateInitOrders(fixedpoint.NewFromFloat(0.5))
	assert.NoError(t, err)
	//mock open
	mockOpen(&orderForms)

	order := orderForms[0]
	cell := order.GetCell()
	assert.Equal(t, CellOrderStateOpen, order.OrderState())

	mockGridFill(order.GetCell(), order)
	assert.Equal(t, CellOrderStateFill, order.OrderState())
	//it should be zero profit for first trade.

	profits := *cell.Profits
	assert.Equal(t, fixedpoint.NewFromFloat(0.021), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	orderPointer := &order
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.021), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.042), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.042), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.063), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	defer cancel()
	_ = ctx
}

func TestGenerateOrderAndCommitWithReInv(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := Strategy{
		Symbol:   "XRPUSDT",
		Quantity: fixedpoint.NewFromFloat(100),
		Market: types.Market{
			Symbol:        "XRPUSDT",
			BaseCurrency:  "XRP",
			QuoteCurrency: "USDT",
			StepSize:      1.0,
		},
		ReinvestRate:     fixedpoint.NewFromInt(1),
		UpperPrice:       fixedpoint.NewFromFloat(0.85),
		LowerPrice:       fixedpoint.NewFromFloat(0.7),
		ProfitSpreadRate: fixedpoint.NewFromFloat(0.003),
		GridNum:          10,
	}
	if err := s.LoadState(); err != nil {
		assert.NoError(t, err)
	}

	s.InitGrids()

	orderForms, err := s.generateInitOrders(fixedpoint.NewFromFloat(0.5))
	assert.NoError(t, err)
	//mock open
	mockOpen(&orderForms)

	order := orderForms[0]
	cell := order.GetCell()
	assert.Equal(t, CellOrderStateOpen, order.OrderState())

	mockGridFill(order.GetCell(), order)
	assert.Equal(t, CellOrderStateFill, order.OrderState())
	//it should be zero profit for first trade.
	profits := *cell.Profits
	assert.Equal(t, fixedpoint.NewFromFloat(0.21), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	orderPointer := &order

	//buy
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.21), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.42), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//buy
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.42), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.63), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//buy
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.63), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.84), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//buy
	//over 0.70 , buy one more xrp back at @0.70
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.14), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(1), profits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.14+0.2121+0.7021), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	defer cancel()
	_ = ctx
}

func TestGenerateOrderAndCommitWithReInvOverCellOrders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := Strategy{
		Symbol:   "XRPUSDT",
		Quantity: fixedpoint.NewFromFloat(100),
		Market: types.Market{
			Symbol:        "XRPUSDT",
			BaseCurrency:  "XRP",
			QuoteCurrency: "USDT",
			StepSize:      1.0,
		},
		ReinvestRate:     fixedpoint.NewFromInt(1),
		UpperPrice:       fixedpoint.NewFromFloat(0.85),
		LowerPrice:       fixedpoint.NewFromFloat(0.7),
		ProfitSpreadRate: fixedpoint.NewFromFloat(0.003),
		GridNum:          10,
	}
	if err := s.LoadState(); err != nil {
		assert.NoError(t, err)
	}

	s.InitGrids()

	orderForms, err := s.generateInitOrders(fixedpoint.NewFromFloat(0.5))
	assert.NoError(t, err)
	//mock open
	mockOpen(&orderForms)

	order := orderForms[0]
	order1 := orderForms[1]
	cell := order.GetCell()
	profits := *cell.Profits

	//sell
	mockGridFill(order.GetCell(), order)
	assert.Equal(t, CellOrderStateFill, order.OrderState())

	//buy
	orderPointer := mockGenerateAndFillNextOrder(t, cell, &order)
	assert.Equal(t, fixedpoint.NewFromFloat(0.21), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.42), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//buy
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.42), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.63), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//buy
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.63), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//sell
	mockGridFill(order1.GetCell(), order1)
	assert.Equal(t, CellOrderStateFill, order1.OrderState())
	order1Profits := *order1.GetCell().Profits
	assert.Equal(t, fixedpoint.NewFromFloat(0.2145), order1Profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), order1Profits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.84), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	//buy
	//over 0.70 , buy one more xrp back at @0.70
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.14), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(1), profits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.14+0.2121+0.7021), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	defer cancel()
	_ = ctx
}

func TestGenerateOrderAndCommitWithReInvOverCellOrdersHalfPrice(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := Strategy{
		Symbol:   "XRPUSDT",
		Quantity: fixedpoint.NewFromFloat(100),
		Market: types.Market{
			Symbol:        "XRPUSDT",
			BaseCurrency:  "XRP",
			QuoteCurrency: "USDT",
			StepSize:      1.0,
		},
		ReinvestRate:     fixedpoint.NewFromFloat(0.5),
		UpperPrice:       fixedpoint.NewFromFloat(0.85),
		LowerPrice:       fixedpoint.NewFromFloat(0.7),
		ProfitSpreadRate: fixedpoint.NewFromFloat(0.003),
		GridNum:          10,
	}
	if err := s.LoadState(); err != nil {
		assert.NoError(t, err)
	}

	s.InitGrids()

	orderForms, err := s.generateInitOrders(fixedpoint.NewFromFloat(0.5))
	assert.NoError(t, err)
	//mock open
	mockOpen(&orderForms)

	order := orderForms[0]
	order1 := orderForms[1]
	cell := order.GetCell()
	profits := *cell.Profits
	gridProfits := *cell.Strategy.state.Profits

	//sell
	mockGridFill(order.GetCell(), order)
	assert.Equal(t, CellOrderStateFill, order.OrderState())

	//buy
	orderPointer := mockGenerateAndFillNextOrder(t, cell, &order)
	assert.Equal(t, fixedpoint.NewFromFloat(0.21), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.105), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.42), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.21), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	//buy
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.42), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.21), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.63), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.315), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	//buy
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.63), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.315), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	//sell
	mockGridFill(order1.GetCell(), order1)
	assert.Equal(t, CellOrderStateFill, order1.OrderState())
	order1Profits := *order1.GetCell().Profits
	assert.Equal(t, fixedpoint.NewFromFloat(0.2145), order1Profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), order1Profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.315+0.2145*0.5), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.84), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.42+0.2145*0.5), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	//buy
	//over 0.70 , buy one more xrp back at @0.70
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.84), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.42+0.2145*0.5), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	//sell
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.84+0.21), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.525+0.2145*0.5), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	defer cancel()
	_ = ctx
}

func TestGenerateOrderAndCommitFromHighToLow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := Strategy{
		Symbol:   "XRPUSDT",
		Quantity: fixedpoint.NewFromFloat(100),
		Market: types.Market{
			Symbol:        "XRPUSDT",
			BaseCurrency:  "XRP",
			QuoteCurrency: "USDT",
			StepSize:      1.0,
		},
		ReinvestRate:     fixedpoint.NewFromFloat(1),
		UpperPrice:       fixedpoint.NewFromFloat(0.85),
		LowerPrice:       fixedpoint.NewFromFloat(0.7),
		ProfitSpreadRate: fixedpoint.NewFromFloat(0.003),
		GridNum:          10,
	}
	if err := s.LoadState(); err != nil {
		assert.NoError(t, err)
	}

	s.InitGrids()

	orderForms, err := s.generateInitOrders(fixedpoint.NewFromFloat(1.2))
	assert.NoError(t, err)
	//mock open
	mockOpen(&orderForms)

	order := orderForms[0]
	cell := order.GetCell()
	profits := *cell.Profits
	gridProfits := *cell.Strategy.state.Profits

	//buy
	mockGridFill(order.GetCell(), order)
	assert.Equal(t, CellOrderStateFill, order.OrderState())

	//sell
	orderPointer := mockGenerateAndFillNextOrder(t, cell, &order)
	assert.Equal(t, fixedpoint.NewFromFloat(0.21), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.21), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	//buy
	orderPointer = mockGenerateAndFillNextOrder(t, cell, orderPointer)
	assert.Equal(t, fixedpoint.NewFromFloat(0.21), profits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), profits[s.Market.BaseCurrency].Total)

	assert.Equal(t, fixedpoint.NewFromFloat(0.21), gridProfits[s.Market.QuoteCurrency].Total)
	assert.Equal(t, fixedpoint.NewFromInt(0), gridProfits[s.Market.BaseCurrency].Total)

	defer cancel()
	_ = ctx
}

func mockGenerateAndFillNextOrder(t *testing.T, cell *GridCell, order *ICellOrder) *ICellOrder {
	var nextOrders = cell.generateOrders()
	assert.Equal(t, 1, len(nextOrders))
	mockOpen(&nextOrders)
	assert.Equal(t, CellOrderStateClosed, (*order).OrderState())
	assert.Equal(t, 2, len(cell.Orders))

	mockGridFill((*order).GetCell(), nextOrders[0])
	assert.Equal(t, CellOrderStateFill, nextOrders[0].OrderState())
	return &nextOrders[0]
}

func mockFillOrder(order ICellOrder) types.Order {
	return types.Order{
		SubmitOrder:      order.GetOrder(),
		Status:           types.OrderStatusFilled,
		ExecutedQuantity: order.GetOrder().Quantity,
		IsWorking:        true,
	}
}

func mockGridFill(grid *GridCell, orderForm ICellOrder) {

	grid.commitOrderFilled(mockFillOrder(orderForm))
}

func mockOpen(orderForms *[]ICellOrder) {
	for ind := range *orderForms {
		var o = (*orderForms)[ind]
		o.UpdateClientOrderId(fmt.Sprintf("%d", rand.Int()))
		o.GetCell().commitOpenOrder(&o)
		o.GetCell().commitOrderClosed(o.GetPrevOrder())
	}
}
