package matchingengine

import (
	"container/heap"
	"cryptosim/models"
)

type OrderBook struct {
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
		bestRestingOrder := orderBook.peek(order.Side)

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
			trade.BuyerMMID = order.MMID
			trade.SellerMMID = bestRestingOrder.MMID
		} else {
			trade.BuyerOrderID = bestRestingOrder.ID
			trade.SellerOrderID = order.ID
			trade.BuyerMMID = bestRestingOrder.MMID
			trade.SellerMMID = order.MMID
		}

		order.Fill(filledQuantity)
		bestRestingOrder.Fill(filledQuantity)

		if bestRestingOrder.IsFilled() {
			orderBook.removeFromHeap(order.Side)
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
		restingOrder := orderBook.peek(order.Side)
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
			orderBook.removeFromHeap(order.Side)
			delete(orderBook.orders, restingOrder.ID)
		}

		trades = append(trades, trade)
	}

	return trades

}

func (orderBook *OrderBook) addToBook(order *models.Order) {
	orderBook.orders[order.ID] = order

	if order.Side == models.Ask {
		heap.Push(orderBook.asks, order)
	} else {
		heap.Push(orderBook.bids, order)
	}
}

func (orderBook *OrderBook) removeFromHeap(side models.Side) {
	if side == models.Ask {
		heap.Pop(orderBook.asks)
	} else {
		heap.Pop(orderBook.bids)
	}
}

func (orderBook *OrderBook) peek(side models.Side) *models.Order {
	if side == models.Ask {
		if orderBook.asks.Len() == 0 {
			return nil
		}
		return (*orderBook.asks)[0]
	} else {
		if orderBook.bids.Len() == 0 {
			return nil
		}
		return (*orderBook.bids)[0]
	}
}
