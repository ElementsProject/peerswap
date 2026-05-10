package peersync

import (
	"context"
	"log"
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
		log.Printf("failed to list connected peers for cleanup: %v", err)
		_, cleanupErr := p.store.CleanupExpired(p.timeout)
		return cleanupErr
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

	knownPeers := make(map[string]*Peer, len(peers))

	for _, peer := range peers {
		if peer == nil {
			continue
		}
		knownPeers[peer.ID().String()] = peer
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

	p.requestUnknownConnectedPeers(ctx, knownPeers)
}

func (p *poller) requestUnknownConnectedPeers(ctx context.Context, knownPeers map[string]*Peer) {
	connected, err := p.connectedPeers(ctx)
	if err != nil {
		log.Printf("failed to list connected peers for poll: %v", err)
		return
	}

	for peerID := range connected {
		if _, ok := knownPeers[peerID.String()]; ok {
			continue
		}
		if p.guard != nil && p.guard.Suspicious(peerID) {
			continue
		}
		if err := p.send(ctx, peerID, messages.MESSAGETYPE_REQUEST_POLL); err != nil {
			log.Printf("failed to request poll from %s: %v", peerID.String(), err)
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
