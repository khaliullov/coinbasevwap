package producers

import (
	"encoding/json"
	"fmt"

	"github.com/khaliullov/coinbasevwap/internal/entity"
)

type VWAP interface {
	Send(msg *entity.VWAP) error
}

type vwap struct {
}

func newVWAP() VWAP {
	return &vwap{}
}

func (m *vwap) Send(msg *entity.VWAP) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	fmt.Println(string(payload))

	return nil
}
