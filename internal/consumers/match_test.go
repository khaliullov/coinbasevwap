package consumers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/khaliullov/coinbasevwap/internal/entity"
	"github.com/khaliullov/coinbasevwap/internal/repository"
	"github.com/khaliullov/coinbasevwap/pkg/mocks/usecase"
)

type MatchConsumerSuite struct {
	suite.Suite
	config       *entity.Config
	matchUseCase *usecase.MatchUseCase
	useCase      *usecase.UseCase
	consumer     Consumer
}

func TestMatchConsumerSuite(t *testing.T) {
	suite.Run(t, new(MatchConsumerSuite))
}

func (suite *MatchConsumerSuite) SetupTest() {
	suite.config = &entity.Config{
		URL:        "https://test.org",
		Capacity:   200,
		Channels:   []string{"best_channel"},
		ProductIDs: []string{"BTC-USD", "ETH-USD", "ETH-BTC"},
	}
	suite.matchUseCase = &usecase.MatchUseCase{}
	suite.useCase = &usecase.UseCase{}
	suite.useCase.On("Match").Return(suite.matchUseCase)
	suite.consumer = NewMatchConsumer(suite.useCase, suite.config)
}

func (suite *MatchConsumerSuite) TearDownTest() {

}

func (suite *MatchConsumerSuite) Test_Consume_CastError() {
	err := suite.consumer.Consume(nil)
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrBadMatchMessage)
}

func (suite *MatchConsumerSuite) Test_Consume_UseCaseError() {
	suite.matchUseCase.On("UpdateVWAP", mock.Anything).Return(repository.ErrTradingPairNotFound)
	err := suite.consumer.Consume(&entity.Match{
		ProductID: "Test",
		Price:     "0.01",
		Size:      "100",
	})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, repository.ErrTradingPairNotFound)
}

func (suite *MatchConsumerSuite) Test_Consume_Ok() {
	suite.matchUseCase.On("UpdateVWAP", mock.Anything).Return(nil)
	err := suite.consumer.Consume(&entity.Match{
		ProductID: "BTC-USD",
		Price:     "0.01",
		Size:      "100",
	})
	assert.NoError(suite.T(), err)
}
