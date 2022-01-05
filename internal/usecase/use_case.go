package usecase

import (
	"github.com/khaliullov/coinbasevwap/internal/entity"
	"github.com/khaliullov/coinbasevwap/internal/producers"
	"github.com/khaliullov/coinbasevwap/internal/repository"
)

//go:generate mockery --name UseCase --case underscore --output ../../pkg/mocks/usecase --outpkg usecase
//go:generate mockery --name MatchUseCase --case underscore --output ../../pkg/mocks/usecase --outpkg usecase

type UseCase interface {
	Match() MatchUseCase
}

type useCase struct {
	match MatchUseCase
}

func NewUseCase(
	repo repository.Repository,
	producer producers.Producer,
	config *entity.Config,
) UseCase {
	return &useCase{
		match: newMatchUseCase(repo, producer, config),
	}
}

func (m *useCase) Match() MatchUseCase {
	return m.match
}
