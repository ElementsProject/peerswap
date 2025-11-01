package peersync

import (
	"context"
	"errors"
	"log"
	"time"
)

// Synchronizer abstracts the sync loop that the supervisor manages.
type Synchronizer interface {
	Start(ctx context.Context) error
}

// Supervisor monitors the lifecycle of the peer sync loop.
type Supervisor struct {
	ctx          context.Context
	cancel       context.CancelFunc
	syncer       Synchronizer
	config       *Config
	restartCount int
	lastRestart  time.Time
}

// Config controls restart behaviour for the supervisor.
type Config struct {
	MaxRestarts    int
	RestartWindow  time.Duration
	RestartBackoff time.Duration
}

// New constructs a Supervisor for the given synchronizer.
func New(config *Config, syncer Synchronizer) *Supervisor {
	return newSupervisor(config, syncer)
}

func newSupervisor(config *Config, syncer Synchronizer) *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Supervisor{
		ctx:          ctx,
		cancel:       cancel,
		syncer:       syncer,
		config:       config,
		restartCount: 0,
		lastRestart:  time.Time{},
	}
}

// Start begins supervising the synchronizer.
func (s *Supervisor) Start() error {
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		if !s.canRestart() {
			return errors.New("max restart limit reached")
		}

		log.Printf("starting peersync service (attempt #%d)", s.restartCount+1)

		err := s.syncer.Start(s.ctx)
		if err != nil {
			log.Printf("service start failed: %v", err)
			s.handleRestart()
			continue
		}

		<-s.ctx.Done()
		return s.ctx.Err()
	}
}

// Stop cancels the supervisor context and stops the managed service.
func (s *Supervisor) Stop() {
	s.cancel()
}

func (s *Supervisor) canRestart() bool {
	if s.config == nil {
		return true
	}

	if s.config.MaxRestarts <= 0 {
		return true
	}

	if !s.lastRestart.IsZero() && time.Since(s.lastRestart) > s.config.RestartWindow {
		s.restartCount = 0
	}
	return s.restartCount < s.config.MaxRestarts
}

func (s *Supervisor) handleRestart() {
	s.restartCount++
	s.lastRestart = time.Now()

	if s.config == nil {
		return
	}

	backoff := time.Duration(s.restartCount) * s.config.RestartBackoff
	if limit := 30 * time.Second; backoff > limit {
		backoff = limit
	}

	if backoff > 0 {
		time.Sleep(backoff)
	}
}
