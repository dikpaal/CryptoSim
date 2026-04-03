package engine

import (
	"container/heap"
	"cryptosim/internal/models"
	"sync"
)

type OrderBook struct {
	mu     sync.RWMutex
	symbol string
	bids   *models.MaxHeap
	asks   *models.MinHeap
	orders map[string]*models.Order
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

func (orderBook *OrderBook) SubmitOrder(order *models.Order) []*models.Trade {
	orderBook.mu.Lock()
	defer orderBook.mu.Unlock()

	trades := []*models.Trade{}

	if order.OrderType == models.Market {
		trades = orderBook.matchMarketOrder(order)
	} else {
		trades = orderBook.matchLimitOrder(order)
	}

	if !order.IsFilled() && order.OrderType == models.Limit {
		orderBook.addToBook(order)
	}

	return trades
}

func (orderBook *OrderBook) matchMarketOrder(order *models.Order) []*models.Trade {

	trades := []*models.Trade{}
	var restingOrders heap.Interface

	if order.Side == models.Ask {
		restingOrders = orderBook.bids
	} else {
		restingOrders = orderBook.asks
	}

	for restingOrders.Len() > 0 && !order.IsFilled() {
		bestRestingOrder := orderBook.peekOpposite(order.Side)

		if bestRestingOrder == nil {
			break
		}

		filledQuantity := min(order.RemainingQty(), bestRestingOrder.RemainingQty())
		trade := models.NewTrade(
			order.Symbol,
			bestRestingOrder.Price,
			filledQuantity,
			order,
			bestRestingOrder,
		)

		if order.Side == models.Bid {
			trade.BuyerOrderID = order.ID
			trade.SellerOrderID = bestRestingOrder.ID
			trade.BuyerID = order.Creator_ID
			trade.SellerID = bestRestingOrder.Creator_ID
		} else {
			trade.BuyerOrderID = bestRestingOrder.ID
			trade.SellerOrderID = order.ID
			trade.BuyerID = bestRestingOrder.Creator_ID
			trade.SellerID = order.Creator_ID
		}

		order.Fill(filledQuantity)
		bestRestingOrder.Fill(filledQuantity)

		if bestRestingOrder.IsFilled() {
			orderBook.removeFromHeap(bestRestingOrder.Side)
			delete(orderBook.orders, bestRestingOrder.ID)
		}

		trades = append(trades, trade)
	}

	if !order.IsFilled() {
		order.Status = models.Cancelled
	}

	return trades
}

func (orderBook *OrderBook) matchLimitOrder(order *models.Order) []*models.Trade {
	trades := []*models.Trade{}
	var restingOrders heap.Interface

	if order.Side == models.Ask {
		restingOrders = orderBook.bids
	} else {
		restingOrders = orderBook.asks
	}

	for restingOrders.Len() > 0 && !order.IsFilled() {
		restingOrder := orderBook.peekOpposite(order.Side)
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
			trade.BuyerID = order.Creator_ID
			trade.SellerID = restingOrder.Creator_ID
		} else {
			trade.BuyerOrderID = restingOrder.ID
			trade.SellerOrderID = order.ID
			trade.BuyerID = restingOrder.Creator_ID
			trade.SellerID = order.Creator_ID
		}

		order.Fill(fillQty)
		restingOrder.Fill(fillQty)

		if restingOrder.IsFilled() {
			orderBook.removeFromHeap(restingOrder.Side)
			delete(orderBook.orders, restingOrder.ID)
		}

		trades = append(trades, trade)
	}

	return trades

}

func (orderBook *OrderBook) addToBook(order *models.Order) {
	order.Status = models.Pending
	orderBook.orders[order.ID] = order

	if order.Side == models.Ask {
		heap.Push(orderBook.asks, order)
	} else {
		heap.Push(orderBook.bids, order)
	}
}

func (orderBook *OrderBook) CancelOrder(orderID string) bool {
	orderBook.mu.Lock()
	defer orderBook.mu.Unlock()

	order, exists := orderBook.orders[orderID]
	if !exists {
		return false
	}

	order.Status = models.Cancelled
	delete(orderBook.orders, order.ID)

	orderBook.removeOrderFromHeap(order)
	return true

}

func (orderBook *OrderBook) GetSnapshot(depth int) ([][2]float64, [][2]float64) {
	orderBook.mu.RLock()
	defer orderBook.mu.RUnlock()

	asks := [][2]float64{}
	bids := [][2]float64{}

	for i := 0; i < orderBook.asks.Len() && i < depth; i++ {
		order := (*orderBook.asks)[i]
		asks = append(asks, [2]float64{order.Price, order.RemainingQty()})
	}

	for i := 0; i < orderBook.bids.Len() && i < depth; i++ {
		order := (*orderBook.bids)[i]
		bids = append(bids, [2]float64{order.Price, order.RemainingQty()})
	}

	return asks, bids

}

func (orderBook *OrderBook) removeOrderFromHeap(order *models.Order) {
	var h heap.Interface
	if order.Side == models.Ask {
		h = orderBook.asks
	} else {
		h = orderBook.bids
	}

	for i := 0; i < h.Len(); i++ {
		var o *models.Order
		if order.Side == models.Ask {
			o = (*orderBook.asks)[i]
		} else {
			o = (*orderBook.bids)[i]
		}

		if o.ID == order.ID {
			heap.Remove(h, i)
			break
		}
	}
}

func (orderBook *OrderBook) removeFromHeap(side models.Side) {
	if side == models.Ask {
		heap.Pop(orderBook.asks)
	} else {
		heap.Pop(orderBook.bids)
	}
}

func (orderBook *OrderBook) peekOpposite(side models.Side) *models.Order {

	// peeks in the opposite heap

	if side == models.Ask {
		if orderBook.bids.Len() == 0 {
			return nil
		}
		return (*orderBook.bids)[0]
	} else {
		if orderBook.asks.Len() == 0 {
			return nil
		}
		return (*orderBook.asks)[0]
	}
}

func (ob *OrderBook) GetOrder(orderID string) (*models.Order, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	order, exists := ob.orders[orderID]
	return order, exists
}
