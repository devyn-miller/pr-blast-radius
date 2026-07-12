package routes

import "services"

func HandleCreateOrder(orderID string, amount float64) error {
	svc := services.NewOrderService(nil)
	svc.Create(orderID, amount)
	return nil
}