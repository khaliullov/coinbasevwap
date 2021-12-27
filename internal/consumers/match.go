package consumers

import (
	"errors"

	log "github.com/sirupsen/logrus"

	"github.com/khaliullov/coinbasevwap/internal/entity"
	"github.com/khaliullov/coinbasevwap/internal/usecase"
)

type matchConsumer struct {
	config *entity.Config
	useCase usecase.UseCase
}

var (
	ErrBadMatchMessage = errors.New("bad message")
)

func NewMatchConsumer(useCase usecase.UseCase, config *entity.Config) Consumer {
	return &matchConsumer{
		config: config,
		useCase: useCase,
	}
}

func (m *matchConsumer) Consume(msg interface{}) error {
	message, ok := msg.(*entity.Match)

	if !ok {
		log.WithFields(log.Fields{
			"msg": msg,
		}).Error("matchConsumer.Consume Bad message")

		return ErrBadMatchMessage
	}

	log.WithFields(log.Fields{
		"message": message,
	}).Info("Got message")

	return m.useCase.Match().UpdateVWAP(message)
}
