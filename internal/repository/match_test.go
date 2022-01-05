package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/khaliullov/coinbasevwap/internal/entity"
)

type MatchRepositorySuite struct {
	suite.Suite
	repo Repository
}

func TestMatchRepositorySuite(t *testing.T) {
	suite.Run(t, new(MatchRepositorySuite))
}

func (suite *MatchRepositorySuite) SetupTest() {
	config := &entity.Config{
		URL:        "https://test.org",
		Capacity:   200,
		Channels:   []string{"best_channel"},
		ProductIDs: []string{"BTC-USD", "ETH-USD", "ETH-BTC"},
	}
	suite.repo = NewRepository(config)
}

func (suite *MatchRepositorySuite) TearDownTest() {

}

// Test_MatchRepository_Append_WrongProductID – try to add Deal from the unsubscribed channel
func (suite *MatchRepositorySuite) Test_MatchRepository_Append_WrongProductID() {
	err := suite.repo.Match().Append("Test", &entity.Deal{
		Volume: 10,
		Price:  5,
	})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), ErrTradingPairNotFound, err)
}

// Test_MatchRepository_Append_Ok – adding Deal from the subscribed channel
func (suite *MatchRepositorySuite) Test_MatchRepository_Append_Ok() {
	err := suite.repo.Match().Append("BTC-USD", &entity.Deal{
		Volume: 1000,
		Price:  0.01,
	})
	assert.NoError(suite.T(), err)
}

// Test_MatchRepository_Len_WrongProductID – try to gen circular buffer size of the non-existing trading pair
func (suite *MatchRepositorySuite) Test_MatchRepository_Len_WrongProductID() {
	err := suite.repo.Match().Append("BTC-USD", &entity.Deal{
		Volume: 1000,
		Price:  0.01,
	})
	assert.NoError(suite.T(), err)

	_, err = suite.repo.Match().Len("Test")
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), ErrTradingPairNotFound, err)
}

// Test_MatchRepository_Len_OK – get length of some existing circular buffers
func (suite *MatchRepositorySuite) Test_MatchRepository_Len_OK() {
	err := suite.repo.Match().Append("BTC-USD", &entity.Deal{
		Volume: 1000,
		Price:  0.01,
	})
	assert.NoError(suite.T(), err)
	err = suite.repo.Match().Append("BTC-USD", &entity.Deal{
		Volume: 500,
		Price:  0.02,
	})
	assert.NoError(suite.T(), err)

	length, err := suite.repo.Match().Len("BTC-USD")
	assert.NoError(suite.T(), err)
	assert.EqualValues(suite.T(), 2, length)

	length, err = suite.repo.Match().Len("ETH-USD")
	assert.NoError(suite.T(), err)
	assert.EqualValues(suite.T(), 0, length)
}

// Test_MatchRepository_PopFirst_WrongProductID – try to remove the oldest trading deal for unsubscribed channel
func (suite *MatchRepositorySuite) Test_MatchRepository_PopFirst_WrongProductID() {
	_, err := suite.repo.Match().PopFirst("Test")
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), ErrTradingPairNotFound, err)
}

// Test_MatchRepository_PopFirst_EmptyBuffer – try to remove the oldest trading deal from empty cyclic buffer
func (suite *MatchRepositorySuite) Test_MatchRepository_PopFirst_EmptyBuffer() {
	deal, err := suite.repo.Match().PopFirst("BTC-USD")
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), deal)
}

// Test_MatchRepository_PopFirst_Ok – successfully remothe the oldest trading deal from cyclic buffer
func (suite *MatchRepositorySuite) Test_MatchRepository_PopFirst_Ok() {
	err := suite.repo.Match().Append("BTC-USD", &entity.Deal{
		Volume: 1000,
		Price:  0.01,
	})
	assert.NoError(suite.T(), err)
	err = suite.repo.Match().Append("BTC-USD", &entity.Deal{
		Volume: 500,
		Price:  0.02,
	})
	assert.NoError(suite.T(), err)

	deal, err := suite.repo.Match().PopFirst("BTC-USD")
	assert.NoError(suite.T(), err)
	assert.EqualValues(suite.T(), 1000, deal.Volume)
	assert.EqualValues(suite.T(), 0.01, deal.Price)

	deal, err = suite.repo.Match().PopFirst("BTC-USD")
	assert.NoError(suite.T(), err)
	assert.EqualValues(suite.T(), 500, deal.Volume)
	assert.EqualValues(suite.T(), 0.02, deal.Price)

	deal, err = suite.repo.Match().PopFirst("BTC-USD")
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), deal)
}

// Test_MatchRepository_GetVWAP_WrongProductID – try to get VWAP value for unsubscribed channel
func (suite *MatchRepositorySuite) Test_MatchRepository_GetVWAP_WrongProductID() {
	_, err := suite.repo.Match().GetVWAP("Test")
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), ErrTradingPairNotFound, err)
}

// Test_MatchRepository_GetVWAP_DivisionByZero – try to get VWAP with division by zero error
func (suite *MatchRepositorySuite) Test_MatchRepository_GetVWAP_DivisionByZero() {
	err := suite.repo.Match().Append("BTC-USD", &entity.Deal{
		Volume: 0,
		Price:  0.01,
	})
	assert.NoError(suite.T(), err)

	_, err = suite.repo.Match().GetVWAP("BTC-USD")
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), ErrDivisionByZero, err)
}

// Test_MatchRepository_GetVWAP_Ok – successfully get VWAP
func (suite *MatchRepositorySuite) Test_MatchRepository_GetVWAP_Ok() {
	err := suite.repo.Match().Append("BTC-USD", &entity.Deal{
		Volume: 1000,
		Price:  0.01,
	})
	assert.NoError(suite.T(), err)
	err = suite.repo.Match().Append("BTC-USD", &entity.Deal{
		Volume: 500,
		Price:  0.02,
	})
	assert.NoError(suite.T(), err)

	vwap, err := suite.repo.Match().GetVWAP("BTC-USD")
	assert.NoError(suite.T(), err)
	expectedVWAP := &entity.VWAP{
		ProductID: "BTC-USD",
		VWAP:      (1000*0.01 + 500*0.02) / (1000 + 500),
	}
	assert.EqualValues(suite.T(), expectedVWAP, vwap)
}
