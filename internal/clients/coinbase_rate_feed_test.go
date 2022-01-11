package clients

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

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
	wg        *sync.WaitGroup
	logBuffer bytes.Buffer
	oldOutput io.Writer
}

func TestCoinbaseRateFeedSuite(t *testing.T) {
	suite.Run(t, new(CoinbaseRateFeedSuite))
}

func (suite *CoinbaseRateFeedSuite) SetupTest() {
	suite.oldOutput = log.StandardLogger().Out
	log.SetOutput(&suite.logBuffer)
	suite.wg = new(sync.WaitGroup)
}

func (suite *CoinbaseRateFeedSuite) getConfig(url string) *entity.Config {
	return &entity.Config{
		URL:        url,
		Capacity:   200,
		Channels:   []string{DefaultCoinbaseRateFeedChannel},
		ProductIDs: []string{"BTC-USD", "ETH-USD", "ETH-BTC"},
	}
}

func (suite *CoinbaseRateFeedSuite) TearDownTest() {
	log.SetOutput(suite.oldOutput)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_ConstructorError() {
	_, err := NewCoinbaseRateFeed(suite.wg, &entity.Config{})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrBadConfiguration)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_ConstructorOk() {
	_, err := NewCoinbaseRateFeed(suite.wg, suite.getConfig(DefaultCoinbaseRateFeedWebsocketURL))
	assert.NoError(suite.T(), err)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_RegisterConsumerOk() {
	consumer := new(mockConsumers.Consumer)
	client := &coinbaseRateFeed{
		wg:          suite.wg,
		wsm:         NewWebSocket(DefaultCoinbaseRateFeedWebsocketURL, http.Header{}),
		config:      suite.getConfig(DefaultCoinbaseRateFeedWebsocketURL),
		state:       WS_CONNECTING,
		subscribers: make([]consumers.Consumer, 0),
	}
	client.RegisterMatchConsumer(consumer)
	assert.Len(suite.T(), client.subscribers, 1)
}

type Helloer struct {
	buffer                  []string
	subscribeDelay          time.Duration
	unsubscribeDelay        time.Duration
	sendSubscribeResponse   bool
	sendUnsubscribeResponse bool
	disconnectOnMessageType string
	callbackCh              chan bool
}

type HelloerOption func(m *Helloer)

func WithSubscribeDelay(duration time.Duration) HelloerOption {
	return func(m *Helloer) {
		m.subscribeDelay = duration
	}
}

func WithUnsubscribeDelay(duration time.Duration) HelloerOption {
	return func(m *Helloer) {
		m.unsubscribeDelay = duration
	}
}

func WithDoNotSendSubscribeResponse() HelloerOption {
	return func(m *Helloer) {
		m.sendSubscribeResponse = false
	}
}

func WithDoNotSendUnsubscribeResponse() HelloerOption {
	return func(m *Helloer) {
		m.sendUnsubscribeResponse = false
	}
}

func WithDisconnectOnMessageType(msgType string) HelloerOption {
	return func(m *Helloer) {
		m.disconnectOnMessageType = msgType
	}
}

func WithMessage(message string) HelloerOption {
	return func(m *Helloer) {
		m.buffer = append(m.buffer, message)
	}
}

func NewHelloer(opts ...HelloerOption) *Helloer {
	res := &Helloer{
		buffer:                  []string{},
		subscribeDelay:          0,
		unsubscribeDelay:        0,
		sendSubscribeResponse:   true,
		sendUnsubscribeResponse: true,
		disconnectOnMessageType: "",
		callbackCh:              make(chan bool, 2),
	}

	for _, opt := range opts {
		opt(res)
	}

	return res
}

func (m *Helloer) ServeHTTP(ans http.ResponseWriter, req *http.Request) {
	ws, err := upgrader.Upgrade(ans, req, nil)
	if err != nil {
		log.WithField("error", err).Error("HTTP upgrade error")
		return
	}
	defer ws.Close()
	for {
		_, p, err := ws.ReadMessage()
		if err != nil {
			if err != io.EOF {
				log.WithField("error", err).Error("Helloer: ReadMessage error")
			}
			return
		}
		log.WithField("messsage", string(p)).Debug("Helloer: read")
		if bytes.Contains(p, []byte(`"type":"subscribe"`)) {
			if m.subscribeDelay > 0 {
				<-time.After(m.subscribeDelay)
			}
			if m.disconnectOnMessageType == "subscribe" {
				m.callbackCh <- true
			} else if m.sendSubscribeResponse {
				r := `{"type":"subscriptions","channels":[{"name":"matches","product_ids":["BTC-USD","ETH-USD","ETH-BTC"]}]}`
				m.buffer = append([]string{r + "\n"}, m.buffer...)
			}
		}
		if bytes.Contains(p, []byte(`"type":"unsubscribe"`)) {
			if m.unsubscribeDelay > 0 {
				<-time.After(m.unsubscribeDelay)
			}
			if m.disconnectOnMessageType == "unsubscribe" {
				m.callbackCh <- true
			} else if m.sendUnsubscribeResponse {
				m.buffer = append([]string{`{"type":"subscriptions","channels":[]}` + "\n"}, m.buffer...)
			}
		}
		for len(m.buffer) > 0 {
			var msg string
			msg, m.buffer = m.buffer[0], m.buffer[1:]
			if err := ws.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				log.WithField("error", err).Error("Helloer: WriteMessage error")
				return
			}
			log.WithField("message", msg).Debug("Helloer: send")
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
	srv := httptest.NewServer(NewHelloer(WithMessage(testMessage)))
	defer srv.Close()

	<-time.After(500 * time.Millisecond) // wait to server start

	URL := httpToWs(srv.URL)
	config := suite.getConfig(URL)
	client, err := NewCoinbaseRateFeed(suite.wg, config)
	assert.NoError(suite.T(), err)
	log.WithField("URL", URL).Debug("set server URL")
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
		srv := httptest.NewServer(NewHelloer(WithMessage(test.message)))
		defer srv.Close()

		<-time.After(500 * time.Millisecond) // wait to server start

		URL := httpToWs(srv.URL)
		config := suite.getConfig(URL)
		client, err := NewCoinbaseRateFeed(suite.wg, config)
		hook.client = client
		assert.NoError(suite.T(), err)
		log.WithField("URL", URL).Debug("set server URL")
		log.WithField("message", test.message).Debug("test message")

		client.Run()

		suite.wg.Wait()

		assert.True(suite.T(), len(hook.Entries) > 0)
		assert.EqualValues(suite.T(), test.expected, hook.Entries[0].Message)

		srv.Close()
		hook.Reset()
	}
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_EchoFeed() {
	srv := httptest.NewServer(NewEchoer())
	defer srv.Close()

	<-time.After(500 * time.Millisecond) // wait to server start

	URL := httpToWs(srv.URL)
	config := suite.getConfig(URL)
	client, err := NewCoinbaseRateFeed(suite.wg, config)
	assert.NoError(suite.T(), err)
	log.WithField("URL", URL).Debug("set server URL")

	client.Run()

	suite.wg.Wait()

	srv.Close()
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_ServerErrors() {
	tests := []struct {
		Name          string
		ExpectedError string
		Opt           HelloerOption
		WithCallback  bool
	}{
		{
			Name:          "Test failing subscription",
			ExpectedError: "Failed to change subscription",
			Opt:           WithDisconnectOnMessageType("subscribe"),
			WithCallback:  true,
		},
		{
			Name:          "Test failing unsubscription",
			ExpectedError: "Failed to change subscription",
			Opt:           WithDisconnectOnMessageType("unsubscribe"),
			WithCallback:  true,
		},
		{
			Name:          "Test failing subscribe response timeout",
			ExpectedError: "Failed to change subscription",
			Opt:           WithSubscribeDelay(CommandTimeout + 100*time.Millisecond),
			WithCallback:  false,
		},
		{
			Name:          "Test failing unsubscribe response timeout",
			ExpectedError: "Failed to change subscription",
			Opt:           WithUnsubscribeDelay(CommandTimeout + 100*time.Millisecond),
			WithCallback:  false,
		},
		{
			Name:          "Test failing sending subscribe response",
			ExpectedError: "Failed to change subscription",
			Opt:           WithDoNotSendSubscribeResponse(),
			WithCallback:  false,
		},
		{
			Name:          "Test failing sending unsubscribe response",
			ExpectedError: "Failed to change subscription",
			Opt:           WithDoNotSendUnsubscribeResponse(),
			WithCallback:  false,
		},
	}
	for _, test := range tests {
		helloer := NewHelloer(test.Opt)
		srv := httptest.NewServer(helloer)
		defer srv.Close()

		<-time.After(500 * time.Millisecond) // wait to server start

		URL := httpToWs(srv.URL)
		config := suite.getConfig(URL)
		hook := new(Hook)
		log.AddHook(hook)
		client, err := NewCoinbaseRateFeed(suite.wg, config)
		assert.NoError(suite.T(), err)
		hook.client = client
		log.WithField("URL", URL).Debug("set server URL")

		client.Run()

		if test.WithCallback {
			for {
				if <-helloer.callbackCh {
					srv.Listener.Close()
					srv.Close()
					break
				}
			}
		}

		suite.wg.Wait()

		assert.True(suite.T(), len(hook.Entries) > 0)
		assert.EqualValues(suite.T(), test.ExpectedError, hook.Entries[0].Message)

		srv.Close()
	}
}
