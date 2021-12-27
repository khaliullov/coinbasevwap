package producers

type Producer interface {
	VWAP() VWAP
}

type producer struct {
	vwap VWAP
}

func NewProducer() Producer {
	return &producer{
		vwap: newVWAP(),
	}
}

func (m *producer) VWAP() VWAP {
	return m.vwap
}
