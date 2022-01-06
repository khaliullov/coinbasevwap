package consumers

// Consumer – base interface for all consumers
type Consumer interface {
	Consume(message interface{}) error
}
