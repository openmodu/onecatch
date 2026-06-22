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

func (b *OrderBinding) ListOrders(status string) ([]oneshot.Order, error) {
	return b.client.ListOrders(context.Background(), status)
}

func (b *OrderBinding) GetOrder(orderID string) (oneshot.Order, error) {
	return b.client.GetOrder(context.Background(), orderID)
}

// GetOrderRun returns the live agent run log for an order so the desktop can
// stream the agent's progress into the conversation.
func (b *OrderBinding) GetOrderRun(orderID string) (oneshot.RunLog, error) {
	return b.client.GetOrderRun(context.Background(), orderID)
}

func (b *OrderBinding) CancelOrder(orderID string) (oneshot.Order, error) {
	return b.client.CancelOrder(context.Background(), orderID)
}
