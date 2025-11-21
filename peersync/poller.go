package peersync

import (
	"context"
	"log"
	"time"

	"github.com/elementsproject/peerswap/messages"
)

type poller struct {
	logic   *SyncLogic
	store   *Store
	guard   PeerGuard
	send    capabilitySender
	timeout time.Duration

	pollTickerInterval    time.Duration
	cleanupTickerInterval time.Duration
}

func newPoller(
	logic *SyncLogic,
	store *Store,
	guard PeerGuard,
	send capabilitySender,
	pollTickerInterval time.Duration,
	cleanupTickerInterval time.Duration,
	cleanupTimeout time.Duration,
) *poller {
	return &poller{
		logic:                 logic,
		store:                 store,
		guard:                 guard,
		send:                  send,
		timeout:               cleanupTimeout,
		pollTickerInterval:    pollTickerInterval,
		cleanupTickerInterval: cleanupTickerInterval,
	}
}

func (p *poller) start(ctx context.Context) {
	go p.runPollLoop(ctx)
	go p.runCleanupLoop(ctx)
}

func (p *poller) runPollLoop(ctx context.Context) {
	ticker := time.NewTicker(p.pollTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.PollAllPeers(ctx)
		}
	}
}

func (p *poller) runCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(p.cleanupTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := p.store.CleanupExpired(p.timeout); err != nil {
				log.Printf("cleanup expired peers failed: %v", err)
			}
		}
	}
}

func (p *poller) PollAllPeers(ctx context.Context) {
	p.pollPeers(ctx, false)
}

func (p *poller) ForcePollAllPeers(ctx context.Context) {
	p.pollPeers(ctx, true)
}

func (p *poller) pollPeers(ctx context.Context, force bool) {
	peers, err := p.store.GetAllPeerStates()
	if err != nil {
		log.Printf("failed to get peers: %v", err)
		return
	}

	for _, peer := range peers {
		if !force && !p.logic.ShouldPoll(peer) {
			continue
		}

		if p.guard != nil && p.guard.Suspicious(peer.ID()) {
			continue
		}

		if err := p.send(ctx, peer.ID(), messages.MESSAGETYPE_POLL); err != nil {
			log.Printf("failed to poll %s: %v", peer.ID().String(), err)
			continue
		}

		peer.MarkAsPolled()
		if err := p.store.SavePeerState(peer); err != nil {
			log.Printf("failed to persist peer state for %s: %v", peer.ID().String(), err)
		}
	}
}
