package timer

import (
	"context"
	"time"
)

type CallbackFactory func(...any) func()

type TimeOutService struct {
	CallbackFactory CallbackFactory
}

func NewTimeOutService(cbf CallbackFactory) *TimeOutService {
	return &TimeOutService{CallbackFactory: cbf}
}

func (s *TimeOutService) AddNewTimeOut(ctx context.Context, d time.Duration, args ...any) {
	go TimedCallback(ctx, d, s.CallbackFactory(args))
}
