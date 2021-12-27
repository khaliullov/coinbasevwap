package entity

type Channel struct {
	Name       string   `json:"name"`
	ProductIds []string `json:"product_ids" mapstructure:"product_ids"`
}

type Subscription struct {
	Channels []Channel `json:"channels"`
}

type SubscriptionRequest struct {
	Type       string   `json:"type"`
	Channels   []string `json:"channels"`
	ProductIds []string `json:"product_ids" mapstructure:"product_ids"`
}
