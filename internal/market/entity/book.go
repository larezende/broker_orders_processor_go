package entity

import (
	"container/heap"
	"sync"
)

type Book struct {
	Order         []*Order
	Transactions  []*Transaction
	OrdersChan    chan *Order
	OrdersChanOut chan *Order
	Wg            *sync.WaitGroup
}

func NewBook(orderChan chan *Order, orderChanOut chan *Order, wg *sync.WaitGroup) *Book {
	return &Book{
		Order:         []*Order{},
		Transactions:  []*Transaction{},
		OrdersChan:    orderChan,
		OrdersChanOut: orderChanOut,
		Wg:            wg,
	}
}

func (b *Book) Trade() {
	buyOrders := make(map[string]*OrderQueue)
	sellOrders := make(map[string]*OrderQueue)

	for order := range b.OrdersChan {
		assetId := order.Asset.ID

		if buyOrders[assetId] == nil {
			buyOrders[assetId] = NewOrderQueue()
			heap.Init(buyOrders[assetId])
		}
		if sellOrders[assetId] == nil {
			sellOrders[assetId] = NewOrderQueue()
			heap.Init(sellOrders[assetId])
		}

		if order.OrderType == "BUY" {
			buyOrders[assetId].Push(order)
			if sellOrders[assetId].Len() > 0 && sellOrders[assetId].Orders[0].Price <= order.Price {
				sellOrder := sellOrders[assetId].Pop().(*Order)
				if sellOrder.PendingShares > 0 {
					transaction := NewTransaction(sellOrder, order, order.Shares, sellOrder.Price)
					b.addTransaction(transaction, b.Wg)
					order.Transactions = append(order.Transactions, transaction)
					sellOrder.Transactions = append(sellOrder.Transactions, transaction)
					b.OrdersChanOut <- sellOrder
					b.OrdersChanOut <- order
					if sellOrder.PendingShares > 0 {
						sellOrders[assetId].Push(sellOrder)
					}
				}
			}
		} else if order.OrderType == "SELL" {
			sellOrders[assetId].Push(order)
			if buyOrders[assetId].Len() > 0 && buyOrders[assetId].Orders[0].Price >= order.Price {
				buyOrder := buyOrders[assetId].Pop().(*Order)
				if buyOrder.PendingShares > 0 {
					transaction := NewTransaction(order, buyOrder, order.Shares, buyOrder.Price)
					b.addTransaction(transaction, b.Wg)
					order.Transactions = append(order.Transactions, transaction)
					buyOrder.Transactions = append(buyOrder.Transactions, transaction)
					b.OrdersChanOut <- buyOrder
					b.OrdersChanOut <- order
					if buyOrder.PendingShares > 0 {
						buyOrders[assetId].Push(buyOrder)
					}
				}
			}
		}
	}
}

func (b *Book) addTransaction(transaction *Transaction, wg *sync.WaitGroup) {
	defer wg.Done()

	sellingShares := transaction.SellingOrder.PendingShares
	buyingShares := transaction.BuyingOrder.PendingShares

	minShares := sellingShares
	if buyingShares < minShares {
		minShares = buyingShares
	}

	transaction.SellingOrder.Investor.UpdateAssetPosition(transaction.SellingOrder.Asset.ID, -minShares)
	transaction.SellingOrder.PendingShares -= minShares

	transaction.BuyingOrder.Investor.UpdateAssetPosition(transaction.BuyingOrder.Asset.ID, minShares)
	transaction.BuyingOrder.PendingShares -= minShares

	transaction.CalculateTotal(transaction.Shares, transaction.BuyingOrder.Price)

	transaction.CloseBuyingOrder()
	transaction.CloseSellingOrder()

	b.Transactions = append(b.Transactions, transaction)

}
