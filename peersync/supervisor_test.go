package peersync

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSyncer struct {
	failures int
	starts   int
	err      error
}

func (f *fakeSyncer) Start(ctx context.Context) error {
	f.starts++
	if f.failures > 0 {
		f.failures--
		if f.err != nil {
			return f.err
		}
		return errors.New("start failed")
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestSupervisorRespectsMaxRestarts(t *testing.T) {
	syncer := &fakeSyncer{failures: 3, starts: 0, err: nil}
	cfg := &Config{
		MaxRestarts:    2,
		RestartWindow:  time.Second,
		RestartBackoff: 0,
	}

	s := newSupervisor(cfg, syncer)

	startErr := s.Start()
	if startErr == nil {
		t.Fatalf("expected error, got nil")
	}
	if syncer.starts != 2 {
		t.Fatalf("expected 2 starts, got %d", syncer.starts)
	}
}

func TestSupervisorStopsOnCancel(t *testing.T) {
	syncer := &fakeSyncer{failures: 0, starts: 0, err: nil}
	cfg := &Config{
		MaxRestarts:    1,
		RestartWindow:  time.Second,
		RestartBackoff: 0,
	}

	s := newSupervisor(cfg, syncer)

	done := make(chan struct{})
	go func() {
		err := s.Start()
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context canceled, got %v", err)
		}
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	s.Stop()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("supervisor did not stop in time")
	}
}
