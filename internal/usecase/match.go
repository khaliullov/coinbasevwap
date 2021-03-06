package usecase

import (
	"errors"
	"strconv"

	"github.com/khaliullov/coinbasevwap/internal/entity"
	"github.com/khaliullov/coinbasevwap/internal/producers"
	"github.com/khaliullov/coinbasevwap/internal/repository"
)

// MatchUseCase – use case for processing *entity.Match entities
type MatchUseCase interface {
	UpdateVWAP(match *entity.Match) error
}

type matchUseCase struct {
	repo     repository.Repository
	producer producers.Producer
	config   *entity.Config
}

// ErrNegativeOrZeroValue – wrong *entity.Match instance with Size or Price fields having zero or negative values
var ErrNegativeOrZeroValue = errors.New("negative or zero value of volume or price")

func newMatchUseCase(
	repo repository.Repository,
	producer producers.Producer,
	config *entity.Config,
) MatchUseCase {
	return &matchUseCase{
		repo:     repo,
		producer: producer,
		config:   config,
	}
}

func (m *matchUseCase) UpdateVWAP(match *entity.Match) error {
	volume, err := strconv.ParseFloat(match.Size, 32)
	if err != nil {
		return err
	}
	price, err := strconv.ParseFloat(match.Price, 32)
	if err != nil {
		return err
	}
	deal := entity.Deal{
		Volume: volume,
		Price:  price,
	}

	if deal.Volume <= 0 || deal.Price <= 0 {
		return ErrNegativeOrZeroValue
	}

	err = m.repo.Match().Append(match.ProductID, &deal)
	if err != nil {
		return err
	}

	capacity, err := m.repo.Match().Len(match.ProductID)
	if err != nil {
		return err
	}

	if capacity > m.config.Capacity { // cyclic buffer
		if _, err := m.repo.Match().PopFirst(match.ProductID); err != nil {
			return err
		}
	}

	vwap, err := m.repo.Match().GetVWAP(match.ProductID)
	if err != nil {
		return err
	}

	return m.producer.VWAP().Send(vwap)
}
