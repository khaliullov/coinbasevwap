package clients

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/khaliullov/coinbasevwap/internal/consumers"
	"github.com/khaliullov/coinbasevwap/internal/entity"
	mockConsumers "github.com/khaliullov/coinbasevwap/pkg/mocks/consumers"
)

type CoinbaseRateFeedSuite struct {
	suite.Suite
	wg     *sync.WaitGroup
	config *entity.Config
}

func TestCoinbaseRateFeedSuite(t *testing.T) {
	suite.Run(t, new(CoinbaseRateFeedSuite))
}

func (suite *CoinbaseRateFeedSuite) SetupTest() {
	suite.wg = new(sync.WaitGroup)
	suite.config = &entity.Config{
		URL:        DefaultCoinbaseRateFeedWebsocketURL,
		Capacity:   200,
		Channels:   []string{DefaultCoinbaseRateFeedChannel},
		ProductIDs: []string{"BTC-USD", "ETH-USD", "ETH-BTC"},
	}
}

func (suite *CoinbaseRateFeedSuite) TearDownTest() {

}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_ConstructorError() {
	_, err := NewCoinbaseRateFeed(suite.wg, &entity.Config{})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrBadConfiguration)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_ConstructorOk() {
	_, err := NewCoinbaseRateFeed(suite.wg, suite.config)
	assert.NoError(suite.T(), err)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_RegisterConsumerOk() {
	consumer := new(mockConsumers.Consumer)
	client := &coinbaseRateFeed{
		wg:          suite.wg,
		wsm:         NewWebSocket(DefaultCoinbaseRateFeedWebsocketURL, http.Header{}),
		config:      suite.config,
		state:       WS_CONNECTING,
		subscribers: make([]consumers.Consumer, 0),
	}
	client.RegisterMatchConsumer(consumer)
	assert.Len(suite.T(), client.subscribers, 1)
}

type Helloer struct {
	test   *testing.T
	buffer []string
}

func NewHelloer(t *testing.T, message string) *Helloer {
	return &Helloer{
		test:   t,
		buffer: []string{message},
	}
}

func (m *Helloer) ServeHTTP(ans http.ResponseWriter, req *http.Request) {
	ws, err := upgrader.Upgrade(ans, req, nil)
	if err != nil {
		m.test.Errorf("HTTP upgrade error: %v", err)
		return
	}
	defer ws.Close()
	for {
		_, p, err := ws.ReadMessage()
		if err != nil {
			if err != io.EOF {
				m.test.Logf("Helloer: ReadMessage error: %v", err)
			}
			return
		}
		m.test.Logf("Helloer: read: %v", string(p))
		if bytes.Contains(p, []byte(`"type":"subscribe"`)) {
			r := `{"type":"subscriptions","channels":[{"name":"matches","product_ids":["BTC-USD","ETH-USD","ETH-BTC"]}]}`
			m.buffer = append([]string{r + "\n"}, m.buffer...)
		}
		if bytes.Contains(p, []byte(`"type":"unsubscribe"`)) {
			m.buffer = append([]string{`{"type":"subscriptions","channels":[]}` + "\n"}, m.buffer...)
		}
		for len(m.buffer) > 0 {
			var msg string
			msg, m.buffer = m.buffer[0], m.buffer[1:]
			if err := ws.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				m.test.Logf("Helloer: WriteMessage error: %v", err)
				return
			}
			m.test.Logf("Helloer: send: %v", msg)
		}
	}
}

type Hook struct {
	Entries []log.Entry
	mu      sync.RWMutex
	client  CoinbaseRateFeedInterface
}

func (m *Hook) Fire(e *log.Entry) error {
	if m.client != nil {
		m.client.Stop()
		m.client = nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Entries = append(m.Entries, *e)
	return nil
}

func (m *Hook) Levels() []log.Level {
	return []log.Level{
		log.ErrorLevel,
	}
}

func (m *Hook) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Entries = make([]log.Entry, 0)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_Ok() {
	var buf bytes.Buffer
	log.SetOutput(&buf)

	testMessage := `{
		"type": "match",
 		"trade_id": 260823760,
		"maker_order_id": "2458f516-42e1-4f3d-950d-556a77fa7699",
		"taker_order_id": "6ecfedd9-d434-4096-b3ae-a4b2850bad10",
		"side": "sell",
		"size": "0.0234701",
		"price": "41801.67",
		"product_id": "BTC-USD",
		"sequence": 32856489309,
		"time":	"2022-01-07T19:54:43.295378Z"
	}
`
	srv := httptest.NewServer(NewHelloer(suite.T(), testMessage))
	defer srv.Close()

	URL := httpToWs(srv.URL)
	suite.config.URL = URL
	client, err := NewCoinbaseRateFeed(suite.wg, suite.config)
	assert.NoError(suite.T(), err)
	suite.T().Logf("Server URL: %v", URL)
	consumer := new(mockConsumers.Consumer)
	consumer.On("Consume", mock.Anything).Return(func(msg interface{}) error {
		message, _ := msg.(*entity.Match)
		assert.EqualValues(suite.T(), "0.0234701", message.Size)
		assert.EqualValues(suite.T(), "41801.67", message.Price)

		client.Stop()

		return consumers.ErrBadMatchMessage
	})
	client.RegisterMatchConsumer(consumer)

	client.Run()

	suite.wg.Wait()

	srv.Close()
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_Error() {
	tests := []struct {
		message  string
		expected string
	}{
		{
			message:  "Test\n",
			expected: "Failed to parse message JSON",
		},
		{
			message:  "{\"success\": true}\n",
			expected: "Skipping message with unknown type",
		},
		{
			message:  "{\"type\": true}",
			expected: "Failed to get type",
		},
		{
			message:  "{\"type\": \"test\"}",
			expected: "Skipping message with unknown type",
		},
		{
			message:  "{\"type\": \"match\", \"size\": 0.01}",
			expected: "Unable to unmarshal message to appropriate structure",
		},
	}
	hook := new(Hook)
	log.AddHook(hook)
	for _, test := range tests {
		var buf bytes.Buffer
		log.SetOutput(&buf)

		srv := httptest.NewServer(NewHelloer(suite.T(), test.message))
		defer srv.Close()

		URL := httpToWs(srv.URL)
		suite.config.URL = URL
		client, err := NewCoinbaseRateFeed(suite.wg, suite.config)
		hook.client = client
		assert.NoError(suite.T(), err)
		suite.T().Logf("Server URL: %v", URL)
		suite.T().Logf("Test message: %v", test.message)

		client.Run()

		suite.wg.Wait()

		assert.True(suite.T(), len(hook.Entries) > 0)
		assert.EqualValues(suite.T(), test.expected, hook.Entries[0].Message)

		srv.Close()
		hook.Reset()
	}
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_EchoFeed() {
	srv := httptest.NewServer(NewEchoer(suite.T()))
	defer srv.Close()

	URL := httpToWs(srv.URL)
	suite.config.URL = URL
	client, err := NewCoinbaseRateFeed(suite.wg, suite.config)
	assert.NoError(suite.T(), err)
	suite.T().Logf("Server URL: %v", URL)

	client.Run()

	suite.wg.Wait()

	srv.Close()
}
