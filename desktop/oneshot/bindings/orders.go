package bindings

import (
	"context"

	oneshot "github.com/openmodu/oneshot/clients/oneshot"
)

type OrderBinding struct {
	client oneshot.Client
}

func NewOrderBinding(client oneshot.Client) *OrderBinding {
	return &OrderBinding{client: client}
}

func (b *OrderBinding) CreateOrder(input oneshot.CreateOrderRequest) (oneshot.Order, error) {
	return b.client.CreateOrder(context.Background(), input)
}

func (b *OrderBinding) ListOrders() ([]oneshot.Order, error) {
	return b.client.ListOrders(context.Background())
}

func (b *OrderBinding) GetOrder(orderID string) (oneshot.Order, error) {
	return b.client.GetOrder(context.Background(), orderID)
}

func (b *OrderBinding) CancelOrder(orderID string) (oneshot.Order, error) {
	return b.client.CancelOrder(context.Background(), orderID)
}
