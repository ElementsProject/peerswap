package peersync

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/messages"
)

type poller struct {
	logic     *SyncLogic
	store     *Store
	lightning Lightning
	guard     PeerGuard
	send      capabilitySender
	timeout   time.Duration

	pollTickerInterval    time.Duration
	cleanupTickerInterval time.Duration
	requestInterval       time.Duration

	mu              sync.Mutex
	lastRequestedAt map[PeerID]time.Time
}

func newPoller(
	logic *SyncLogic,
	store *Store,
	lightning Lightning,
	guard PeerGuard,
	send capabilitySender,
	pollTickerInterval time.Duration,
	cleanupTickerInterval time.Duration,
	cleanupTimeout time.Duration,
	requestInterval time.Duration,
) *poller {
	return &poller{
		logic:                 logic,
		store:                 store,
		lightning:             lightning,
		guard:                 guard,
		send:                  send,
		timeout:               cleanupTimeout,
		pollTickerInterval:    pollTickerInterval,
		cleanupTickerInterval: cleanupTickerInterval,
		requestInterval:       requestInterval,
		lastRequestedAt:       make(map[PeerID]time.Time),
	}
}

func (p *poller) start(ctx context.Context) {
	if p == nil {
		return
	}
	if p.store == nil || p.logic == nil || p.send == nil {
		log.Printf("peersync: poller not properly configured, skipping start")
		return
	}
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
	if p.store == nil {
		log.Printf("peersync: poller store not configured, skipping cleanup loop")
		return
	}

	ticker := time.NewTicker(p.cleanupTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.cleanupExpired(ctx); err != nil {
				log.Printf("cleanup expired peers failed: %v", err)
			}
		}
	}
}

func (p *poller) cleanupExpired(ctx context.Context) error {
	connected, err := p.connectedPeers(ctx)
	if err != nil {
		return fmt.Errorf("skipping cleanup sweep, list connected peers: %w", err)
	}

	_, err = p.store.CleanupExpiredExcept(p.timeout, connected)
	return err
}

func (p *poller) PollAllPeers(ctx context.Context) {
	p.pollPeers(ctx, false)
}

func (p *poller) ForcePollAllPeers(ctx context.Context) {
	p.pollPeers(ctx, true)
}

func (p *poller) pollPeers(ctx context.Context, force bool) {
	if p.store == nil {
		log.Printf("peersync: poller store not configured, skipping poll")
		return
	}
	if p.logic == nil {
		log.Printf("peersync: poller logic not configured, skipping poll")
		return
	}
	if p.send == nil {
		log.Printf("peersync: capability sender not configured, skipping poll")
		return
	}

	peers, err := p.store.GetAllPeerStates()
	if err != nil {
		log.Printf("failed to get peers: %v", err)
		return
	}

	knownPeers := make(map[PeerID]struct{}, len(peers))
	now := time.Now()

	for _, peer := range peers {
		if peer == nil {
			continue
		}
		knownPeers[peer.ID()] = struct{}{}
		if !force && !p.logic.ShouldPoll(peer) {
			continue
		}

		if p.guard != nil && p.guard.Suspicious(peer.ID()) {
			continue
		}

		msgType := messages.MESSAGETYPE_POLL
		if p.capabilityIsStale(peer, now) {
			msgType = messages.MESSAGETYPE_REQUEST_POLL
		}

		if err := p.send(ctx, peer.ID(), msgType); err != nil {
			log.Printf("failed to poll %s: %v", peer.ID().String(), err)
			continue
		}

		peer.MarkAsPolled()
		if err := p.store.SavePeerState(peer); err != nil {
			log.Printf("failed to persist peer state for %s: %v", peer.ID().String(), err)
		}
	}

	p.requestUnknownConnectedPeers(ctx, knownPeers, force)
}

func (p *poller) requestUnknownConnectedPeers(ctx context.Context, knownPeers map[PeerID]struct{}, force bool) {
	connected, err := p.connectedPeers(ctx)
	if err != nil {
		log.Printf("failed to list connected peers for poll: %v", err)
		return
	}

	p.pruneRequestTimes(connected)

	now := time.Now()
	for peerID := range connected {
		if _, ok := knownPeers[peerID]; ok {
			continue
		}
		if p.guard != nil && p.guard.Suspicious(peerID) {
			continue
		}
		if !p.allowRequest(peerID, now, force) {
			continue
		}
		if err := p.send(ctx, peerID, messages.MESSAGETYPE_REQUEST_POLL); err != nil {
			log.Printf("failed to request poll from %s: %v", peerID.String(), err)
		}
	}
}

// capabilityIsStale reports whether the peer's capability has not been
// refreshed by an inbound poll for more than half the cleanup timeout.
// Stale peers are sent a request poll instead of a plain poll so that
// peers that only answer requests keep their stored capability current.
func (p *poller) capabilityIsStale(peer *Peer, now time.Time) bool {
	last := peer.LastObservedAt()
	if last.IsZero() {
		return false
	}
	return now.Sub(last) > p.timeout/2
}

// allowRequest records the request attempt and reports whether a poll
// request may be sent to the peer. Attempts are recorded even when the
// send later fails so that unresponsive peers are contacted at most once
// per request interval.
func (p *poller) allowRequest(peerID PeerID, now time.Time, force bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if last, ok := p.lastRequestedAt[peerID]; ok && !force && now.Sub(last) < p.requestInterval {
		return false
	}
	p.lastRequestedAt[peerID] = now
	return true
}

// pruneRequestTimes drops request timestamps of peers that are no longer
// connected, so a peer that reconnects is requested again immediately.
func (p *poller) pruneRequestTimes(connected map[PeerID]struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id := range p.lastRequestedAt {
		if _, ok := connected[id]; !ok {
			delete(p.lastRequestedAt, id)
		}
	}
}

func (p *poller) connectedPeers(ctx context.Context) (map[PeerID]struct{}, error) {
	connected := make(map[PeerID]struct{})
	if p.lightning == nil {
		return connected, nil
	}

	peers, err := p.lightning.ListPeers(ctx)
	if err != nil {
		return nil, err
	}
	for _, peerID := range peers {
		connected[peerID] = struct{}{}
	}
	return connected, nil
}
