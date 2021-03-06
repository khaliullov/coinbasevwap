// Code generated by mockery v2.9.4. DO NOT EDIT.

package repository

import (
	entity "github.com/khaliullov/coinbasevwap/internal/entity"
	mock "github.com/stretchr/testify/mock"
)

// MatchRepository is an autogenerated mock type for the MatchRepository type
type MatchRepository struct {
	mock.Mock
}

// Append provides a mock function with given fields: tradingPair, deal
func (_m *MatchRepository) Append(tradingPair string, deal *entity.Deal) error {
	ret := _m.Called(tradingPair, deal)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, *entity.Deal) error); ok {
		r0 = rf(tradingPair, deal)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetVWAP provides a mock function with given fields: tradingPair
func (_m *MatchRepository) GetVWAP(tradingPair string) (*entity.VWAP, error) {
	ret := _m.Called(tradingPair)

	var r0 *entity.VWAP
	if rf, ok := ret.Get(0).(func(string) *entity.VWAP); ok {
		r0 = rf(tradingPair)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*entity.VWAP)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(tradingPair)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Len provides a mock function with given fields: tradingPair
func (_m *MatchRepository) Len(tradingPair string) (int, error) {
	ret := _m.Called(tradingPair)

	var r0 int
	if rf, ok := ret.Get(0).(func(string) int); ok {
		r0 = rf(tradingPair)
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(tradingPair)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PopFirst provides a mock function with given fields: tradingPair
func (_m *MatchRepository) PopFirst(tradingPair string) (*entity.Deal, error) {
	ret := _m.Called(tradingPair)

	var r0 *entity.Deal
	if rf, ok := ret.Get(0).(func(string) *entity.Deal); ok {
		r0 = rf(tradingPair)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*entity.Deal)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(tradingPair)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
