package repository

import "github.com/khaliullov/coinbasevwap/internal/entity"

type Repository interface {
	Match() MatchRepository
}

type repository struct {
	match MatchRepository
}

func NewRepository(config *entity.Config) Repository {
	return &repository{
		match: newMatchRepository(config),
	}
}

func (m *repository) Match() MatchRepository {
	return m.match
}
