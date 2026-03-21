package main

import (
	"container/heap"
	"cryptosim/models"
	"sync"
)

type OrderBook struct {
	symbol string
	bids   *models.MaxHeap
	asks   *models.MinHeap
	orders map[string]*models.Order
	mu     sync.RWMutex
}

func NewOrderBook(symbol string) *OrderBook {
	bids := &models.MaxHeap{}
	asks := &models.MinHeap{}
	heap.Init(bids)
	heap.Init(asks)

	return &OrderBook{
		symbol: symbol,
		bids:   bids,
		asks:   asks,
		orders: make(map[string]*models.Order),
	}
}

func (ob *OrderBook) SubmitOrder(order *models.Order) []*models.Trade {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	trades := []*models.Trade{}

	if order.OrderType == models.Market {
		trades = ob.matchMarketOrder(order)
	} else {
		trades = ob.matchLimitOrder(order)
	}

	if !order.IsFilled() && order.OrderType == models.Limit {
		ob.addToBook(order)
	}

	return trades
}

func (ob *OrderBook) matchMarketOrder(order *models.Order) []*models.Trade {
	trades := []*models.Trade{}
	var oppositeHeap heap.Interface

	if order.Side == models.Bid {
		oppositeHeap = ob.asks
	} else {
		oppositeHeap = ob.bids
	}

	for oppositeHeap.Len() > 0 && !order.IsFilled() {
		restingOrder := ob.peekOpposite(order.Side)
		if restingOrder == nil {
			break
		}

		fillQty := min(order.RemainingQty(), restingOrder.RemainingQty())
		trade := models.NewTrade(order.Symbol, restingOrder.Price, fillQty, order, restingOrder)

		if order.Side == models.Bid {
			trade.BuyerOrderID = order.ID
			trade.SellerOrderID = restingOrder.ID
			trade.BuyerMMID = order.MMID
			trade.SellerMMID = restingOrder.MMID
		} else {
			trade.BuyerOrderID = restingOrder.ID
			trade.SellerOrderID = order.ID
			trade.BuyerMMID = restingOrder.MMID
			trade.SellerMMID = order.MMID
		}

		order.Fill(fillQty)
		restingOrder.Fill(fillQty)

		if restingOrder.IsFilled() {
			ob.removeFromHeap(order.Side)
			delete(ob.orders, restingOrder.ID)
		}

		trades = append(trades, trade)
	}

	if !order.IsFilled() {
		order.Status = models.Cancelled
	}

	return trades
}

func (ob *OrderBook) matchLimitOrder(order *models.Order) []*models.Trade {
	trades := []*models.Trade{}
	var oppositeHeap heap.Interface

	if order.Side == models.Bid {
		oppositeHeap = ob.asks
	} else {
		oppositeHeap = ob.bids
	}

	for oppositeHeap.Len() > 0 && !order.IsFilled() {
		restingOrder := ob.peekOpposite(order.Side)
		if restingOrder == nil {
			break
		}

		canMatch := false
		if order.Side == models.Bid {
			canMatch = order.Price >= restingOrder.Price
		} else {
			canMatch = order.Price <= restingOrder.Price
		}

		if !canMatch {
			break
		}

		fillQty := min(order.RemainingQty(), restingOrder.RemainingQty())
		trade := models.NewTrade(order.Symbol, restingOrder.Price, fillQty, order, restingOrder)

		if order.Side == models.Bid {
			trade.BuyerOrderID = order.ID
			trade.SellerOrderID = restingOrder.ID
			trade.BuyerMMID = order.MMID
			trade.SellerMMID = restingOrder.MMID
		} else {
			trade.BuyerOrderID = restingOrder.ID
			trade.SellerOrderID = order.ID
			trade.BuyerMMID = restingOrder.MMID
			trade.SellerMMID = order.MMID
		}

		order.Fill(fillQty)
		restingOrder.Fill(fillQty)

		if restingOrder.IsFilled() {
			ob.removeFromHeap(order.Side)
			delete(ob.orders, restingOrder.ID)
		}

		trades = append(trades, trade)
	}

	return trades
}

func (ob *OrderBook) addToBook(order *models.Order) {
	ob.orders[order.ID] = order
	if order.Side == models.Bid {
		heap.Push(ob.bids, order)
	} else {
		heap.Push(ob.asks, order)
	}
}

func (ob *OrderBook) peekOpposite(side models.Side) *models.Order {
	if side == models.Bid {
		if ob.asks.Len() == 0 {
			return nil
		}
		return (*ob.asks)[0]
	} else {
		if ob.bids.Len() == 0 {
			return nil
		}
		return (*ob.bids)[0]
	}
}

func (ob *OrderBook) removeFromHeap(side models.Side) {
	if side == models.Bid {
		heap.Pop(ob.asks)
	} else {
		heap.Pop(ob.bids)
	}
}

func (ob *OrderBook) CancelOrder(orderID string) bool {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	order, exists := ob.orders[orderID]
	if !exists {
		return false
	}

	order.Status = models.Cancelled
	delete(ob.orders, orderID)

	ob.removeOrderFromHeap(order)
	return true
}

func (ob *OrderBook) removeOrderFromHeap(order *models.Order) {
	var h heap.Interface
	if order.Side == models.Bid {
		h = ob.bids
	} else {
		h = ob.asks
	}

	for i := 0; i < h.Len(); i++ {
		var o *models.Order
		if order.Side == models.Bid {
			o = (*ob.bids)[i]
		} else {
			o = (*ob.asks)[i]
		}

		if o.ID == order.ID {
			heap.Remove(h, i)
			break
		}
	}
}

func (ob *OrderBook) GetSnapshot(depth int) ([][2]float64, [][2]float64) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bids := [][2]float64{}
	asks := [][2]float64{}

	for i := 0; i < ob.bids.Len() && i < depth; i++ {
		order := (*ob.bids)[i]
		bids = append(bids, [2]float64{order.Price, order.RemainingQty()})
	}

	for i := 0; i < ob.asks.Len() && i < depth; i++ {
		order := (*ob.asks)[i]
		asks = append(asks, [2]float64{order.Price, order.RemainingQty()})
	}

	return bids, asks
}

func (ob *OrderBook) GetOrder(orderID string) (*models.Order, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	order, exists := ob.orders[orderID]
	return order, exists
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
