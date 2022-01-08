package clients

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"

	"github.com/khaliullov/coinbasevwap/internal/consumers"
	"github.com/khaliullov/coinbasevwap/internal/entity"
)

const (
	// DefaultCoinbaseRateFeedWebsocketURL – WebSocket scheme/host/port of Coinbase rate feed to connect to
	DefaultCoinbaseRateFeedWebsocketURL = "wss://ws-feed.exchange.coinbase.com"
	// DefaultCoinbaseRateFeedChannel – default channel with transactions
	DefaultCoinbaseRateFeedChannel = "matches"

	// TypeSubscribe – request type for subscribing
	TypeSubscribe = "subscribe"
	// TypeUnsubscribe – request type for unsubscribing
	TypeUnsubscribe = "unsubscribe"
)

var (
	// CommandTimeout – maximum timeout for waiting for the command response
	CommandTimeout = 500 * time.Millisecond

	// ErrBadConfiguration – wrong configuration of service
	ErrBadConfiguration = errors.New("bad configuration")
	// ErrDisconnected – disconnected from Coinbase rate feed WebSocket
	ErrDisconnected = errors.New("disconnected")
	// ErrBadJSON – could not parse JSON from Coinbase rate feed WebSocket
	ErrBadJSON = errors.New("could not parse JSON")
	// ErrFailedToDeserialize – failed to deserialize data from JSON to corresponding entity
	ErrFailedToDeserialize = errors.New("failed to deserialize")
	// ErrUnsupportedMessageType – received message of unknown type
	ErrUnsupportedMessageType = errors.New("skipping unsupported message with unknown type")
)

// CoinbaseRateFeedInterface – interface of Coinbase rate feed client
type CoinbaseRateFeedInterface interface {
	RegisterMatchConsumer(consumer consumers.Consumer)
	Run()
	Stop()
}

type coinbaseRateFeed struct {
	wg           *sync.WaitGroup
	wsm          *WSMachine
	config       *entity.Config
	state        WSState
	subscribers  []consumers.Consumer
	cmdTimeoutCh chan bool
}

// NewCoinbaseRateFeed – create WebSocket client for Coinbase rate feed
func NewCoinbaseRateFeed(wg *sync.WaitGroup, config *entity.Config) (CoinbaseRateFeedInterface, error) {
	if config.URL == "" || len(config.Channels) == 0 || len(config.ProductIDs) == 0 {
		return nil, ErrBadConfiguration
	}

	res := &coinbaseRateFeed{
		wg:           wg,
		wsm:          NewWebSocket(config.URL, http.Header{}),
		config:       config,
		state:        WS_CONNECTING,
		subscribers:  make([]consumers.Consumer, 0),
		cmdTimeoutCh: make(chan bool, 2),
	}

	return res, nil
}

func (m *coinbaseRateFeed) RegisterMatchConsumer(consumer consumers.Consumer) {
	m.subscribers = append(m.subscribers, consumer)
}

func (m *coinbaseRateFeed) publishMatchMessage(packet *entity.Match) {
	log.WithFields(log.Fields{
		"type":   "Match",
		"packet": packet,
	}).Debug("Got Match packet")

	for _, subscriber := range m.subscribers {
		err := subscriber.Consume(packet)
		if err != nil {
			log.WithFields(log.Fields{
				"packet": packet,
				"error":  err,
			}).Error("Match Consumer returned an error")
		}
	}
}

func (m *coinbaseRateFeed) useTextProtocol(command chan<- WSCommand) {
	command <- WS_USE_TEXT
}

func (m *coinbaseRateFeed) subscriptionTimeout() {
	log.Debug("subscriptionTimeout has started")

	timer := time.NewTimer(CommandTimeout)

	for {
		select {
		case <-m.cmdTimeoutCh: // closed or success
			timer.Stop()
			return
		case <-timer.C:
			log.Error("Failed to change subscription")
			cmdCh := m.wsm.Command()
			cmdCh <- WS_QUIT
			return
		}
	}
}

func (m *coinbaseRateFeed) changeSubscription(output chan<- []byte, command string) {
	msg, _ := json.Marshal(entity.SubscriptionRequest{
		Type:       command,
		Channels:   m.config.Channels,
		ProductIds: m.config.ProductIDs,
	})
	go m.subscriptionTimeout()
	output <- msg
}

func (m *coinbaseRateFeed) unsubscribe(output chan<- []byte) {
	m.changeSubscription(output, TypeUnsubscribe)
}

func (m *coinbaseRateFeed) subscribe(output chan<- []byte) {
	m.changeSubscription(output, TypeSubscribe)
}

func (m *coinbaseRateFeed) processStatus(command chan<- WSCommand, output chan<- []byte, status WSStatus) error {
	if status.Error != nil {
		return status.Error
	}
	m.state = status.State
	switch status.State {
	case WS_CONNECTED:
		m.useTextProtocol(command)
		m.subscribe(output)
	case WS_DISCONNECTED:
		m.wg.Done()
		return ErrDisconnected
	}
	return nil
}

func (m *coinbaseRateFeed) processMessage(command chan<- WSCommand, msg []byte) error {
	var payload map[string]interface{}
	err := json.Unmarshal(msg, &payload)
	if err != nil {
		log.WithFields(log.Fields{
			"msg":   string(msg),
			"error": err,
		}).Error("Failed to parse message JSON")

		return ErrBadJSON
	}
	basePacket := new(entity.Base)
	var packet interface{}
	err = mapstructure.Decode(payload, basePacket)
	if err != nil {
		log.WithFields(log.Fields{
			"msg":   string(msg),
			"error": err,
		}).Error("Failed to get type")

		return ErrFailedToDeserialize
	}
	switch basePacket.Type {
	case "subscriptions":
		packet = new(entity.Subscription)
	case "last_match", "match":
		packet = new(entity.Match)
	default:
		log.WithFields(log.Fields{
			"msg":  string(msg),
			"type": basePacket.Type,
		}).Error("Skipping message with unknown type")

		return ErrUnsupportedMessageType
	}
	err = mapstructure.Decode(payload, packet)
	if err != nil {
		log.WithFields(log.Fields{
			"msg":   string(msg),
			"type":  basePacket.Type,
			"error": err,
		}).Error("Unable to unmarshal message to appropriate structure")

		return ErrFailedToDeserialize
	}
	switch packet := packet.(type) {
	case *entity.Match:
		m.publishMatchMessage(packet)
	case *entity.Subscription:
		m.cmdTimeoutCh <- true
		log.WithFields(log.Fields{
			"type":   "Subscription",
			"packet": packet,
		}).Debug("Got Subscription packet")

		if len(packet.Channels) == 0 {
			log.Debug("Successfully unsubscribed. Disconnecting")

			command <- WS_QUIT
		}
	}
	return nil
}

// Run – start websocket connection with callbacks
func (m *coinbaseRateFeed) Run() {
	m.wg.Add(1)

	go m.wsm.MainLoop()

	command := m.wsm.Command()
	status := m.wsm.Status()
	input := m.wsm.Input()
	output := m.wsm.Output()

	go func() {
	status:
		for {
			select {
			case st := <-status:
				if m.processStatus(command, output, st) != nil {
					break status
				}
			case msg, ok := <-input:
				if ok {
					_ = m.processMessage(command, msg) // ignore protocol errors
				}
			}
		}
	}()
}

// Stop - stop websocket connection
func (m *coinbaseRateFeed) Stop() {
	m.unsubscribe(m.wsm.Output())
}
