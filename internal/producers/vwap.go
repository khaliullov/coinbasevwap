package producers

import (
	"encoding/json"
	"fmt"

	"github.com/khaliullov/coinbasevwap/internal/entity"
)

// VWAP – interface of VWAP producer
type VWAP interface {
	Send(msg *entity.VWAP) error
}

type vwap struct {
}

func newVWAP() VWAP {
	return &vwap{}
}

func (m *vwap) Send(msg *entity.VWAP) error {
	payload, _ := json.Marshal(msg)

	fmt.Println(string(payload))

	return nil
}
