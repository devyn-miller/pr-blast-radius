package services

import (
	"fmt"

	"github.com/tinyshop/shared"
)

type Notifier interface {
	Notify(msg string) error
}

type OrderService struct {
	pricing *PricingEngine
}

func NewOrderService(pe *PricingEngine) *OrderService {
	return &OrderService{pricing: pe}
}

func (s *OrderService) Create(orderID string, amount float64) error {
	final := s.pricing.ApplyDiscount(amount)
	rounded := shared.RoundCurrency(final)
	fmt.Println(rounded)
	return s.notify(orderID)
}

func (s *OrderService) notify(orderID string) error {
	return nil
}

func HandleOrderCreated(orderID string) {
	svc := NewOrderService(nil)
	svc.Create(orderID, 100.0)
}