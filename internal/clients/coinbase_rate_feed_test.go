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
	logBuffer bytes.Buffer
	oldOutput io.Writer
}

func TestCoinbaseRateFeedSuite(t *testing.T) {
	suite.Run(t, new(CoinbaseRateFeedSuite))
}

func (suite *CoinbaseRateFeedSuite) SetupTest() {
	suite.oldOutput = log.StandardLogger().Out
	log.SetOutput(&suite.logBuffer)
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
	wg := new(sync.WaitGroup)
	_, err := NewCoinbaseRateFeed(wg, &entity.Config{})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrBadConfiguration)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_ConstructorOk() {
	wg := new(sync.WaitGroup)
	_, err := NewCoinbaseRateFeed(wg, suite.getConfig(DefaultCoinbaseRateFeedWebsocketURL))
	assert.NoError(suite.T(), err)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_RegisterConsumerOk() {
	wg := new(sync.WaitGroup)
	consumer := new(mockConsumers.Consumer)
	client := &coinbaseRateFeed{
		wg:          wg,
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
	wg := new(sync.WaitGroup)
	client, err := NewCoinbaseRateFeed(wg, config)
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

	wg.Wait()

	srv.Close()
}

func (suite *CoinbaseRateFeedSuite) runServerTest(expectedError string, withCallback bool, opts ...HelloerOption) {
	helloer := NewHelloer(opts...)
	srv := httptest.NewServer(helloer)
	defer srv.Close()

	<-time.After(500 * time.Millisecond) // wait to server start

	URL := httpToWs(srv.URL)
	config := suite.getConfig(URL)
	hook := new(Hook)
	log.AddHook(hook)
	wg := new(sync.WaitGroup)
	client, err := NewCoinbaseRateFeed(wg, config)
	assert.NoError(suite.T(), err)
	hook.client = client
	log.WithField("URL", URL).Debug("set server URL")

	client.Run()

	if withCallback {
		for {
			if <-helloer.callbackCh {
				srv.Listener.Close()
				srv.Close()
				break
			}
		}
	}

	wg.Wait()

	assert.True(suite.T(), len(hook.Entries) > 0)
	assert.EqualValues(suite.T(), expectedError, hook.Entries[0].Message)

	srv.Close()
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_BadMessage() {
	suite.runServerTest("Failed to parse message JSON", false, WithMessage("Test\n"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_BadJSONMessage() {
	suite.runServerTest("Skipping message with unknown type", false,
		WithMessage("{\"success\": true}\n"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_BadTypeMessage() {
	suite.runServerTest("Failed to get type", false,
		WithMessage("{\"type\": true}"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_UnknownTypeMessage() {
	suite.runServerTest("Skipping message with unknown type", false,
		WithMessage("{\"type\": \"test\"}"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_IncorrectMessage() {
	suite.runServerTest("Unable to unmarshal message to appropriate structure", false,
		WithMessage("{\"type\": \"match\", \"size\": 0.01}"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_DisconnectOnSubscribe() {
	suite.runServerTest("Failed to change subscription", true,
		WithDisconnectOnMessageType("subscribe"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_DisconnectOnUnsubscribe() {
	suite.runServerTest("Failed to change subscription", true,
		WithDisconnectOnMessageType("unsubscribe"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_DelayedSubscribeResponse() {
	suite.runServerTest("Failed to change subscription", false,
		WithSubscribeDelay(CommandTimeout+100*time.Millisecond))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_DelayedUnsubscribeResponse() {
	suite.runServerTest("Failed to change subscription", false,
		WithUnsubscribeDelay(CommandTimeout+100*time.Millisecond))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_NoSubscribeResponse() {
	suite.runServerTest("Failed to change subscription", false,
		WithDoNotSendSubscribeResponse())
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_NoUnsubscribeResponse() {
	suite.runServerTest("Failed to change subscription", false,
		WithDoNotSendUnsubscribeResponse())
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_EchoFeed() {
	srv := httptest.NewServer(NewEchoer())
	defer srv.Close()

	<-time.After(500 * time.Millisecond) // wait to server start

	URL := httpToWs(srv.URL)
	config := suite.getConfig(URL)
	wg := new(sync.WaitGroup)
	client, err := NewCoinbaseRateFeed(wg, config)
	assert.NoError(suite.T(), err)
	log.WithField("URL", URL).Debug("set server URL")

	client.Run()

	wg.Wait()

	srv.Close()
}
