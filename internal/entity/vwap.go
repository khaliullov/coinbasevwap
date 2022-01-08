package entity

// VWAP – entity holding VWAP for one of trading pair
type VWAP struct {
	ProductID string  `json:"product_id"`
	VWAP      float64 `json:"vwap"`
}
