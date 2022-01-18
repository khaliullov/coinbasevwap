package consumers

import (
	"errors"

	log "github.com/sirupsen/logrus"

	"github.com/khaliullov/coinbasevwap/internal/entity"
	"github.com/khaliullov/coinbasevwap/internal/usecase"
)

type matchConsumer struct {
	config  *entity.Config
	useCase usecase.UseCase
	logger  *log.Logger
}

var (
	// ErrBadMatchMessage – message in wrong format was received
	ErrBadMatchMessage = errors.New("bad message")
)

// NewMatchConsumer – create new match consumer that processes *entity.Match events
func NewMatchConsumer(logger *log.Logger, useCase usecase.UseCase, config *entity.Config) Consumer {
	return &matchConsumer{
		config:  config,
		useCase: useCase,
		logger:  logger,
	}
}

func (m *matchConsumer) Consume(msg interface{}) error {
	message, ok := msg.(*entity.Match)

	if !ok {
		m.logger.WithFields(log.Fields{
			"msg": msg,
		}).Error("matchConsumer.Consume Bad message")

		return ErrBadMatchMessage
	}

	m.logger.WithFields(log.Fields{
		"message": message,
	}).Info("Got message")

	return m.useCase.Match().UpdateVWAP(message)
}
