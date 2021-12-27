package clients

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"

	"github.com/khaliullov/coinbasevwap/internal/consumers"
	"github.com/khaliullov/coinbasevwap/internal/entity"
)

const (
	DefaultCoinbaseRateFeedWebsocketURL = "wss://ws-feed.exchange.coinbase.com"
	DefaultCoinbaseRateFeedChannel      = "matches"

	TypeSubscribe   = "subscribe"
	TypeUnsubscribe = "unsubscribe"
)

var (
	ErrBadConfiguration = errors.New("bad configuration")
	ErrDisconnected = errors.New("disconnected")
	ErrBadJSON = errors.New("could not parse JSON")
	ErrFailedToDeserialize = errors.New("failed to deserialize")
	ErrUnsupportedMessageType = errors.New("skipping unsupported message with unknown type")
)

type CoinbaseRateFeedInterface interface {
	RegisterMatchConsumer(consumer consumers.Consumer)
	Run()
	Stop()
}

type coinbaseRateFeed struct {
	wg          *sync.WaitGroup
	wsm         *WSMachine
	config      *entity.Config
	state       WSState
	subscribers []consumers.Consumer
}

func NewCoinbaseRateFeed(wg *sync.WaitGroup, config *entity.Config) (CoinbaseRateFeedInterface, error) {
	if config.URL == "" || len(config.Channels) == 0 || len(config.ProductIDs) == 0 {
		return nil, ErrBadConfiguration
	}

	res := &coinbaseRateFeed{
		wg:          wg,
		wsm:         NewWebSocket(config.URL, http.Header{}),
		config:      config,
		state:       WS_CONNECTING,
		subscribers: make([]consumers.Consumer, 0),
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
				"error": err,
			}).Error("Match Consumer returned an error")
		}
	}
}

func (m *coinbaseRateFeed) useTextProtocol(command chan<- WSCommand) {
	command <- WS_USE_TEXT
}

func (m *coinbaseRateFeed) changeSubscription(output chan<- []byte, command string) {
	msg, _ := json.Marshal(entity.SubscriptionRequest{
		Type:       command,
		Channels:   m.config.Channels,
		ProductIds: m.config.ProductIDs,
	})
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
		for {
			select {
			case st := <-status:
				if m.processStatus(command, output, st) != nil {
					break
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
