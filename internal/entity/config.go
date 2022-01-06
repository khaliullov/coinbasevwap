package entity

// Config – configuration of service
type Config struct {
	URL        string   `json:"url"`
	Capacity   int      `json:"capacity"`
	Channels   []string `json:"channels"`
	ProductIDs []string `json:"product_ids"`
}
