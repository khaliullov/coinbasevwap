package consumers

//go:generate mockery --name Consumer --case underscore --output ../../pkg/mocks/consumers --outpkg consumers

// Consumer – base interface for all consumers
type Consumer interface {
	Consume(message interface{}) error
}
