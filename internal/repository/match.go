package repository

import (
	"container/list"
	"errors"

	"github.com/khaliullov/coinbasevwap/internal/entity"
)

// MatchRepository – Match repository interface
type MatchRepository interface {
	Len(tradingPair string) (int, error)
	PopFirst(tradingPair string) (*entity.Deal, error)
	Append(tradingPair string, deal *entity.Deal) error
	GetVWAP(tradingPair string) (*entity.VWAP, error)
}

// MatchStorage – storage for storing history of transactions and temporary sums for calculating VWAP
type MatchStorage struct {
	VolumeSum      float32
	VolumePriceSum float32
	Deals          *list.List
}

type matchRepository struct {
	config  *entity.Config
	storage map[string]*MatchStorage
}

var (
	// ErrTradingPairNotFound – transaction from unsubsribed trading pair
	ErrTradingPairNotFound = errors.New("trading pair not found")
	// ErrDivisionByZero – VWAP calculation leads to division by zero
	ErrDivisionByZero = errors.New("division by zero")
)

func newMatchRepository(config *entity.Config) MatchRepository {
	storage := make(map[string]*MatchStorage)

	for _, tradingPair := range config.ProductIDs {
		storage[tradingPair] = &MatchStorage{
			Deals: list.New(),
		}
	}

	return &matchRepository{
		config:  config,
		storage: storage,
	}
}

func (m *matchRepository) getStorage(tradingPair string) (*MatchStorage, error) {
	storage, ok := m.storage[tradingPair]

	if !ok {
		return nil, ErrTradingPairNotFound
	}

	return storage, nil
}

func (m *matchRepository) Len(tradingPair string) (int, error) {
	storage, err := m.getStorage(tradingPair)
	if err != nil {
		return 0, err
	}

	return storage.Deals.Len(), nil
}

func (m *matchRepository) PopFirst(tradingPair string) (*entity.Deal, error) {
	storage, err := m.getStorage(tradingPair)
	if err != nil {
		return nil, err
	}

	element := storage.Deals.Front()
	if element == nil {
		return nil, nil
	}

	deal := storage.Deals.Remove(element).(*entity.Deal)
	storage.VolumeSum -= deal.Volume
	storage.VolumePriceSum -= deal.Volume * deal.Price

	return deal, nil
}

func (m *matchRepository) Append(tradingPair string, deal *entity.Deal) error {
	storage, err := m.getStorage(tradingPair)
	if err != nil {
		return err
	}

	storage.Deals.PushBack(deal)
	storage.VolumeSum += deal.Volume
	storage.VolumePriceSum += deal.Volume * deal.Price

	return nil
}

func (m *matchRepository) GetVWAP(tradingPair string) (*entity.VWAP, error) {
	storage, err := m.getStorage(tradingPair)
	if err != nil {
		return nil, err
	}

	if storage.VolumeSum == 0 {
		return nil, ErrDivisionByZero
	}

	vwap := entity.VWAP{
		ProductID: tradingPair,
		VWAP:      storage.VolumePriceSum / storage.VolumeSum,
	}
	return &vwap, nil
}
