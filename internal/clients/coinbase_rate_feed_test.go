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
}

func TestCoinbaseRateFeedSuite(t *testing.T) {
	suite.Run(t, new(CoinbaseRateFeedSuite))
}

func (suite *CoinbaseRateFeedSuite) SetupTest() {
}

func (suite *CoinbaseRateFeedSuite) TearDownTest() {
}

func (suite *CoinbaseRateFeedSuite) getConfig(url string) *entity.Config {
	return &entity.Config{
		URL:        url,
		Capacity:   200,
		Channels:   []string{DefaultCoinbaseRateFeedChannel},
		ProductIDs: []string{"BTC-USD", "ETH-USD", "ETH-BTC"},
	}
}

func (suite *CoinbaseRateFeedSuite) getLogger(testName string) *log.Logger {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)
	logger.SetFormatter(&log.TextFormatter{
		DisableColors:   true,
		FullTimestamp:   true,
		TimestampFormat: testName + " " + time.RFC3339,
	})
	return logger
}

type Helloer struct {
	buffer                  []string
	subscribeDelay          time.Duration
	unsubscribeDelay        time.Duration
	sendSubscribeResponse   bool
	sendUnsubscribeResponse bool
	disconnectClient        bool
	disconnectOnMessageType string
	callbackCh              chan string
	mu                      sync.RWMutex
	logger                  *log.Logger
}

type HelloerOption func(m *Helloer)

func WithSubscribeDelay(duration time.Duration) HelloerOption {
	return func(m *Helloer) {
		m.subscribeDelay = duration
	}
}

func WithStopClientAfterConnect() HelloerOption {
	return func(m *Helloer) {
		m.disconnectClient = true
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

func NewHelloer(logger *log.Logger, opts ...HelloerOption) *Helloer {
	res := &Helloer{
		buffer:                  []string{},
		subscribeDelay:          0,
		unsubscribeDelay:        0,
		sendSubscribeResponse:   true,
		sendUnsubscribeResponse: true,
		disconnectOnMessageType: "",
		disconnectClient:        false,
		callbackCh:              make(chan string, 2),
		logger:                  logger,
	}

	for _, opt := range opts {
		opt(res)
	}

	return res
}

func (m *Helloer) ServeHTTP(ans http.ResponseWriter, req *http.Request) {
	ws, err := upgrader.Upgrade(ans, req, nil)
	if err != nil {
		m.logger.WithField("error", err).Error("HTTP upgrade error")
		return
	}
	defer ws.Close()
	for {
		_, p, err := ws.ReadMessage()
		if err != nil {
			if err != io.EOF {
				m.logger.WithField("error", err).Error("Helloer: ReadMessage error")
			}
			return
		}
		m.logger.WithField("messsage", string(p)).Debug("Helloer: read")
		if bytes.Contains(p, []byte(`"type":"subscribe"`)) {
			if m.subscribeDelay > 0 {
				<-time.After(m.subscribeDelay)
			}
			m.callbackCh <- "subscribe"
			if m.disconnectOnMessageType == "subscribe" {
				return
			} else if m.sendSubscribeResponse {
				r := `{"type":"subscriptions","channels":[{"name":"matches","product_ids":["BTC-USD","ETH-USD","ETH-BTC"]}]}`
				m.mu.Lock()
				m.buffer = append([]string{r + "\n"}, m.buffer...)
				m.mu.Unlock()
			}
		}
		if bytes.Contains(p, []byte(`"type":"unsubscribe"`)) {
			if m.unsubscribeDelay > 0 {
				<-time.After(m.unsubscribeDelay)
			}
			m.callbackCh <- "unsubscribe"
			if m.disconnectOnMessageType == "unsubscribe" {
				return
			} else if m.sendUnsubscribeResponse {
				m.mu.Lock()
				m.buffer = append([]string{`{"type":"subscriptions","channels":[]}` + "\n"}, m.buffer...)
				m.mu.Unlock()
			}
		}
		m.mu.Lock()
		for len(m.buffer) > 0 {
			var msg string
			msg, m.buffer = m.buffer[0], m.buffer[1:]
			m.logger.WithField("message", msg).Debug("Helloer: send")
			if err := ws.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				m.logger.WithField("error", err).Error("Helloer: WriteMessage error")
				m.mu.Unlock()
				return
			}
		}
		m.mu.Unlock()
	}
}

func (m *Helloer) ShouldStopClient() bool {
	return m.disconnectClient
}

func (m *Helloer) WhenShouldStopServer() string {
	return m.disconnectOnMessageType
}

type Hook struct {
	mu            sync.RWMutex
	LastError     string
	client        CoinbaseRateFeedInterface
	expectedError string
	errorOccurred bool
	logger        *log.Logger
}

func (m *Hook) Fire(e *log.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e.Message == m.expectedError {
		m.errorOccurred = true
		if m.client != nil {
			m.client.Stop()
			m.client = nil
		}
	}

	return nil
}

func (m *Hook) Levels() []log.Level {
	return []log.Level{
		log.ErrorLevel,
	}
}

func (m *Hook) ErrorOccurred() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.errorOccurred
}

func (suite *CoinbaseRateFeedSuite) runServerTest(logger *log.Logger, expectedError string, withCallback bool,
	opts ...HelloerOption) {
	helloer := NewHelloer(logger, opts...)
	srv := httptest.NewServer(helloer)
	defer srv.Close()

	URL := httpToWs(srv.URL)
	logger.WithField("URL", URL).Debug("set server URL")
	config := suite.getConfig(URL)
	wg := new(sync.WaitGroup)
	client, err := NewCoinbaseRateFeed(logger, wg, config)
	assert.NoError(suite.T(), err)
	hook := &Hook{
		expectedError: expectedError,
		errorOccurred: false,
		logger:        logger,
	}
	hook.client = client
	logger.AddHook(hook)

	client.Run()

	if withCallback {
		for {
			messageType, ok := <-helloer.callbackCh
			if !ok {
				break
			}

			if helloer.WhenShouldStopServer() == messageType {
				logger.Debug("Stopping test server...")
				srv.Listener.Close()
				srv.Close()
				break
			}

			if messageType == "subscribe" && helloer.ShouldStopClient() {
				logger.Debug("Stopping client...")
				client.Stop()
				break
			}
		}
	}

	wg.Wait()

	assert.True(suite.T(), hook.ErrorOccurred(), expectedError)

	srv.Close()
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_ConstructorError() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_ConstructorError")
	wg := new(sync.WaitGroup)
	_, err := NewCoinbaseRateFeed(logger, wg, &entity.Config{})
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrBadConfiguration)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_ConstructorOk() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_ConstructorOk")
	wg := new(sync.WaitGroup)
	_, err := NewCoinbaseRateFeed(logger, wg, suite.getConfig(DefaultCoinbaseRateFeedWebsocketURL))
	assert.NoError(suite.T(), err)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_RegisterConsumerOk() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_RegisterConsumerOk")
	wg := new(sync.WaitGroup)
	consumer := new(mockConsumers.Consumer)
	client := &coinbaseRateFeed{
		wg:          wg,
		wsm:         NewWebSocket(logger, DefaultCoinbaseRateFeedWebsocketURL, http.Header{}),
		config:      suite.getConfig(DefaultCoinbaseRateFeedWebsocketURL),
		state:       WS_CONNECTING,
		subscribers: make([]consumers.Consumer, 0),
	}
	client.RegisterMatchConsumer(consumer)
	assert.Len(suite.T(), client.subscribers, 1)
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_Ok() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_Ok")
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
	srv := httptest.NewServer(NewHelloer(logger, WithMessage(testMessage)))
	defer srv.Close()

	URL := httpToWs(srv.URL)
	logger.WithField("URL", URL).Debug("set server URL")
	config := suite.getConfig(URL)
	wg := new(sync.WaitGroup)
	client, err := NewCoinbaseRateFeed(logger, wg, config)
	assert.NoError(suite.T(), err)
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

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_BadMessage() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_BadMessage")
	suite.runServerTest(logger, "Failed to parse message JSON", false,
		WithMessage("Test\n"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_BadJSONMessage() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_BadJSONMessage")
	suite.runServerTest(logger, "Skipping message with unknown type", false,
		WithMessage("{\"success\": true}\n"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_BadTypeMessage() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_BadTypeMessage")
	suite.runServerTest(logger, "Failed to get type", false,
		WithMessage("{\"type\": true}"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_UnknownTypeMessage() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_UnknownTypeMessage")
	suite.runServerTest(logger, "Skipping message with unknown type", false,
		WithMessage("{\"type\": \"test\"}"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_IncorrectMessage() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_IncorrectMessage")
	suite.runServerTest(logger, "Unable to unmarshal message to appropriate structure", false,
		WithMessage("{\"type\": \"match\", \"size\": 0.01}"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_DisconnectOnSubscribe() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_DisconnectOnSubscribe")
	suite.runServerTest(logger, "Failed to change subscription", true,
		WithDisconnectOnMessageType("subscribe"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_DisconnectOnUnsubscribe() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_DisconnectOnUnsubscribe")
	suite.runServerTest(logger, "Failed to change subscription", true,
		WithStopClientAfterConnect(), WithDisconnectOnMessageType("unsubscribe"))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_DelayedSubscribeResponse() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_DelayedSubscribeResponse")
	suite.runServerTest(logger, "Failed to change subscription", false,
		WithSubscribeDelay(CommandTimeout+100*time.Millisecond))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_DelayedUnsubscribeResponse() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_DelayedUnsubscribeResponse")
	suite.runServerTest(logger, "Failed to change subscription", true,
		WithStopClientAfterConnect(), WithUnsubscribeDelay(CommandTimeout+100*time.Millisecond))
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_NoSubscribeResponse() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_NoSubscribeResponse")
	suite.runServerTest(logger, "Failed to change subscription", false,
		WithDoNotSendSubscribeResponse())
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_NoUnsubscribeResponse() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_NoUnsubscribeResponse")
	suite.runServerTest(logger, "Failed to change subscription", true,
		WithStopClientAfterConnect(), WithDoNotSendUnsubscribeResponse())
}

func (suite *CoinbaseRateFeedSuite) Test_CoinbaseRateFeed_EchoFeed() {
	logger := suite.getLogger("Test_CoinbaseRateFeed_EchoFeed")
	srv := httptest.NewServer(NewEchoer())
	defer srv.Close()

	URL := httpToWs(srv.URL)
	logger.WithField("URL", URL).Debug("set server URL")
	config := suite.getConfig(URL)
	wg := new(sync.WaitGroup)
	client, err := NewCoinbaseRateFeed(logger, wg, config)
	assert.NoError(suite.T(), err)

	client.Run()

	wg.Wait()

	srv.Close()
}
