package entity

type Match struct {
	TradeID      int64  `json:"trade_id" mapstructure:"trade_id"`
	MakerOrderID string `json:"maker_order_id" mapstructure:"maker_order_id"`
	TakerOrderID string `json:"taker_order_id" mapstructure:"taker_order_id"`
	Side         string `json:"side"`
	Size         string `json:"size"`
	Price        string `json:"price"`
	ProductID    string `json:"product_id" mapstructure:"product_id"`
	Sequence     int64  `json:"sequence"`
	Time         string `json:"time"`
}
