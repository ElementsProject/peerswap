package poll

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"
)

const version uint64 = 0

type Messenger interface {
	SendMessage(peerId string, message []byte, messageType int) error
}

type PeerGetter interface {
	GetPeers() []string
}

type Policy interface {
	IsPeerAllowed(peerId string) bool
}

type Service struct {
	sync.RWMutex
	clock *time.Ticker
	ctx   context.Context
	done  context.CancelFunc

	assets    []string
	messenger Messenger
	policy    Policy
	peers     PeerGetter
}

func NewPollService(tickDuration time.Duration, messenger Messenger, policy Policy, peers PeerGetter, allowedAssets []string) *Service {
	clock := time.NewTicker(tickDuration)
	ctx, done := context.WithCancel(context.Background())
	return &Service{
		clock:     clock,
		ctx:       ctx,
		done:      done,
		assets:    allowedAssets,
		messenger: messenger,
		policy:    policy,
		peers:     peers,
	}
}

func (s *Service) Start() {
	go func() {
		for {
			select {
			case <-s.clock.C:
				for _, peer := range s.peers.GetPeers() {
					go s.Poll(peer)
				}
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Service) Stop() {
	s.clock.Stop()
	s.done()
}

func (s *Service) Poll(peer string) {
	poll := PollMessage{
		Version:     version,
		Assets:      s.assets,
		PeerAllowed: s.policy.IsPeerAllowed(peer),
	}

	msg, err := json.Marshal(poll)
	if err != nil {
		log.Printf("poll_service: could not marshal poll msg: %v", err)
		return
	}

	if err := s.messenger.SendMessage(peer, msg, 0); err != nil {
		log.Printf("poll_service: could not send poll msg: %v", err)
	}
}
