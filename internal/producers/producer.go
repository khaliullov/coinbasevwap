package producers

//go:generate mockery --name Producer --case underscore --output ../../pkg/mocks/producers --outpkg producers
//go:generate mockery --name VWAP --case underscore --output ../../pkg/mocks/producers --outpkg producers

// Producer – interface for all producers
type Producer interface {
	VWAP() VWAP
}

type producer struct {
	vwap VWAP
}

// NewProducer – constructor for producers
func NewProducer() Producer {
	return &producer{
		vwap: newVWAP(),
	}
}

func (m *producer) VWAP() VWAP {
	return m.vwap
}
