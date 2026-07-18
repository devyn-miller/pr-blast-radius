package routes

import (
	"encoding/json"
	"net/http"
)

// OrderRequest represents an incoming order HTTP request
type OrderRequest struct {
	ProductID    string `json:"product_id"`
	Quantity     int    `json:"quantity"`
	CustomerTier string `json:"customer_tier"`
}

// OrderResponse represents the HTTP response with pricing
type OrderResponse struct {
	ProductID  string  `json:"product_id"`
	Quantity   int     `json:"quantity"`
	TotalPrice float64 `json:"total_price"`
	Currency   string  `json:"currency"`
}

// HandleCreateOrder processes POST /orders
// In a real polyglot system, this would call the Python OrderService
// via RPC/HTTP - the Go analyzer sees this as a cross-language boundary
func HandleCreateOrder(w http.ResponseWriter, r *http.Request) {
	var req OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// In reality: price := orderServiceClient.Price(req.ProductID, req.Quantity, req.CustomerTier)
	// For AST purposes, this call-site is what matters
	resp := OrderResponse{
		ProductID:  req.ProductID,
		Quantity:   req.Quantity,
		TotalPrice: 100.0, // stub
		Currency:   "USD",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
