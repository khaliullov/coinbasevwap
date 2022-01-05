package usecase

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/khaliullov/coinbasevwap/internal/entity"
	"github.com/khaliullov/coinbasevwap/internal/repository"
	"github.com/khaliullov/coinbasevwap/pkg/mocks/producers"
	mockRepository "github.com/khaliullov/coinbasevwap/pkg/mocks/repository"
)

type MatchUseCaseSuite struct {
	suite.Suite
	producer     *producers.Producer
	vwapProducer *producers.VWAP
	repo         repository.Repository
	useCase      UseCase
	config       *entity.Config
}

func TestMatchUseCaseSuite(t *testing.T) {
	suite.Run(t, new(MatchUseCaseSuite))
}

func (suite *MatchUseCaseSuite) SetupTest() {
	suite.config = &entity.Config{
		URL:        "https://test.org",
		Capacity:   200,
		Channels:   []string{"best_channel"},
		ProductIDs: []string{"BTC-USD", "ETH-USD", "ETH-BTC"},
	}
	suite.repo = repository.NewRepository(suite.config)
	suite.vwapProducer = new(producers.VWAP)
	suite.producer = new(producers.Producer)
	suite.producer.On("VWAP").Return(suite.vwapProducer)
	suite.useCase = NewUseCase(suite.repo, suite.producer, suite.config)
}

func (suite *MatchUseCaseSuite) TearDownTest() {
}

// Test_MatchUseCase_UpdateVWAP_BadVolume – Volume is not number
func (suite *MatchUseCaseSuite) Test_MatchUseCase_UpdateVWAP_BadVolume() {
	err := suite.useCase.Match().UpdateVWAP(&entity.Match{
		Size: "Test",
	})
	assert.Error(suite.T(), err)
	expectedError := (&strconv.NumError{
		Func: "ParseFloat",
		Num:  "Test",
		Err:  strconv.ErrSyntax,
	}).Error()
	assert.EqualValues(suite.T(), expectedError, err.Error())
}

// Test_MatchUseCase_UpdateVWAP_BadPrice – price is not number
func (suite *MatchUseCaseSuite) Test_MatchUseCase_UpdateVWAP_BadPrice() {
	err := suite.useCase.Match().UpdateVWAP(&entity.Match{
		Size:  "1000",
		Price: "Test",
	})
	assert.Error(suite.T(), err)
	expectedError := (&strconv.NumError{
		Func: "ParseFloat",
		Num:  "Test",
		Err:  strconv.ErrSyntax,
	}).Error()
	assert.EqualValues(suite.T(), expectedError, err.Error())
}

// Test_MatchUseCase_UpdateVWAP_ZeroVolume – volume is zero
func (suite *MatchUseCaseSuite) Test_MatchUseCase_UpdateVWAP_ZeroVolume() {
	err := suite.useCase.Match().UpdateVWAP(&entity.Match{
		Size:  "0",
		Price: "100.0",
	})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), ErrZeroValue, err)
}

// Test_MatchUseCase_UpdateVWAP_ZeroPrice – price is zero
func (suite *MatchUseCaseSuite) Test_MatchUseCase_UpdateVWAP_ZeroPrice() {
	err := suite.useCase.Match().UpdateVWAP(&entity.Match{
		Size:  "100",
		Price: "0.0",
	})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), ErrZeroValue, err)
}

// Test_MatchUseCase_UpdateVWAP_AppendError – error while appending new deal
func (suite *MatchUseCaseSuite) Test_MatchUseCase_UpdateVWAP_AppendError() {
	err := suite.useCase.Match().UpdateVWAP(&entity.Match{
		Size:      "1000",
		Price:     "0.01",
		ProductID: "Test",
	})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), repository.ErrTradingPairNotFound, err)
}

// Test_MatchUseCase_UpdateVWAP_ProduceError – error while producing calculated VWAP
func (suite *MatchUseCaseSuite) Test_MatchUseCase_UpdateVWAP_ProduceError() {
	_, expectedError := json.Marshal(make(chan int))
	suite.vwapProducer.On("Send", mock.Anything).Return(expectedError)
	suite.producer.On("VWAP").Return(suite.vwapProducer)

	err := suite.useCase.Match().UpdateVWAP(&entity.Match{
		Size:      "1000",
		Price:     "0.01",
		ProductID: "BTC-USD",
	})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), expectedError, err)
}

// Test_MatchUseCase_UpdateVWAP_LenError – impossible error, because it should appear with Append already
func (suite *MatchUseCaseSuite) Test_MatchUseCase_UpdateVWAP_LenError() {
	matchRepo := new(mockRepository.MatchRepository)
	matchRepo.On("Len", mock.Anything).Return(0, repository.ErrTradingPairNotFound)
	matchRepo.On("Append", mock.Anything, mock.Anything).Return(nil)
	repo := new(mockRepository.Repository)
	repo.On("Match").Return(matchRepo)
	useCase := NewUseCase(repo, nil, suite.config)
	err := useCase.Match().UpdateVWAP(&entity.Match{
		Size:      "1000",
		Price:     "0.01",
		ProductID: "BTC-USD",
	})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), repository.ErrTradingPairNotFound, err)
}

// Test_MatchUseCase_UpdateVWAP_PopFirstError – simulate PopFirst error
func (suite *MatchUseCaseSuite) Test_MatchUseCase_UpdateVWAP_PopFirstError() {
	matchRepo := new(mockRepository.MatchRepository)
	matchRepo.On("Len", mock.Anything).Return(suite.config.Capacity+1, nil)
	matchRepo.On("Append", mock.Anything, mock.Anything).Return(nil)
	matchRepo.On("PopFirst", mock.Anything).Return(nil, repository.ErrTradingPairNotFound)
	repo := new(mockRepository.Repository)
	repo.On("Match").Return(matchRepo)
	useCase := NewUseCase(repo, nil, suite.config)
	err := useCase.Match().UpdateVWAP(&entity.Match{
		Size:      "1000",
		Price:     "0.01",
		ProductID: "BTC-USD",
	})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), repository.ErrTradingPairNotFound, err)
}

// Test_MatchUseCase_UpdateVWAP_GetVWAPError – simulate GetVWAP error
func (suite *MatchUseCaseSuite) Test_MatchUseCase_UpdateVWAP_GetVWAPError() {
	matchRepo := new(mockRepository.MatchRepository)
	matchRepo.On("Len", mock.Anything).Return(0, nil)
	matchRepo.On("Append", mock.Anything, mock.Anything).Return(nil)
	matchRepo.On("GetVWAP", mock.Anything).Return(nil, repository.ErrTradingPairNotFound)
	repo := new(mockRepository.Repository)
	repo.On("Match").Return(matchRepo)
	useCase := NewUseCase(repo, nil, suite.config)
	err := useCase.Match().UpdateVWAP(&entity.Match{
		Size:      "1000",
		Price:     "0.01",
		ProductID: "BTC-USD",
	})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), repository.ErrTradingPairNotFound, err)
}
