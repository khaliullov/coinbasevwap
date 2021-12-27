package consumers

type Consumer interface {
	Consume(message interface{}) error
}
