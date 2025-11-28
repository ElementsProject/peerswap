package peersync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/messages"
	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
)

// SyncLogic contains the rules for deciding when and how to sync peers.
type SyncLogic struct {
	pollInterval time.Duration
}

// NewSyncLogic builds a logic helper with sensible defaults.
func NewSyncLogic() *SyncLogic {
	return &SyncLogic{
		pollInterval: 10 * time.Second,
	}
}

// ShouldPoll reports whether a peer requires polling based on the last poll timestamp.
func (l *SyncLogic) ShouldPoll(peer *Peer) bool {
	if peer.Status() == StatusExpired {
		return false
	}
	return time.Since(peer.LastPollAt()) > l.pollInterval
}

// MergeCapabilities merges the local and remote capabilities preferring the newer version.
func (l *SyncLogic) MergeCapabilities(
	local *PeerCapability,
	remote *PeerCapability,
) *PeerCapability {
	if local == nil {
		return remote
	}
	if remote == nil {
		return local
	}

	if remote.Version().Value() < local.Version().Value() {
		return local
	}

	return remote
}

// PeerSync orchestrates polling and capability exchange between peers.
type PeerSync struct {
	nodeID          PeerID
	logic           *SyncLogic
	store           *Store
	lightning       Lightning
	guard           PeerGuard
	supportedAssets []Asset
	premiumSetting  *premium.Setting
	version         Version

	poller  *poller
	handler *messageHandler

	pollTickerInterval    time.Duration
	cleanupTickerInterval time.Duration
	cleanupTimeout        time.Duration
}

type capabilitySender func(ctx context.Context, peer PeerID, msgType messages.MessageType) error

// NewPeerSync wires dependencies into a new synchronizer instance.
func NewPeerSync(
	nodeID PeerID,
	store *Store,
	lightning Lightning,
	policyCfg *policy.Policy,
	supportedAssets []string,
	premiumSetting *premium.Setting,
) *PeerSync {
	guard := NewPeerGuard(policyCfg, premiumSetting)

	ps := &PeerSync{
		nodeID:                nodeID,
		logic:                 NewSyncLogic(),
		store:                 store,
		lightning:             lightning,
		guard:                 guard,
		supportedAssets:       normalizeSupportedAssets(supportedAssets),
		premiumSetting:        premiumSetting,
		version:               NewVersion(swap.PEERSWAP_PROTOCOL_VERSION),
		pollTickerInterval:    10 * time.Second,
		cleanupTickerInterval: 1 * time.Minute,
		cleanupTimeout:        30 * time.Minute,
	}

	ps.poller = newPoller(
		ps.logic,
		ps.store,
		ps.guard,
		ps.sendCapability,
		ps.pollTickerInterval,
		ps.cleanupTickerInterval,
		ps.cleanupTimeout,
	)
	ps.handler = newMessageHandler(ps.store, ps.logic, ps.guard, ps.sendCapability)

	return ps
}

// Start launches the polling, cleanup, and message handling loops.
func (ps *PeerSync) Start(ctx context.Context) error {
	if ps.lightning == nil {
		return errors.New("lightning not configured")
	}
	if ps.store == nil {
		return errors.New("store not configured")
	}

	if err := ps.performInitialSync(ctx); err != nil {
		log.Printf("initial sync failed: %v", err)
	}

	msgChan, err := ps.lightning.SubscribeCustomMessages(ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe to custom messages: %w", err)
	}

	if ps.poller != nil {
		ps.poller.start(ctx)
	}
	if ps.handler != nil {
		go ps.handler.handleMessages(ctx, msgChan)
	}

	return nil
}

// Serve allows PeerSync to satisfy suture.Service. It starts the sync loops and
// blocks until the provided context is canceled, ensuring a graceful shutdown.
func (ps *PeerSync) Serve(ctx context.Context) error {
	if err := ps.Start(ctx); err != nil {
		return err
	}

	<-ctx.Done()

	if err := ps.Stop(); err != nil {
		log.Printf("peersync stop failed: %v", err)
	}

	return nil
}

func (ps *PeerSync) performInitialSync(ctx context.Context) error {
	peers, err := ps.lightning.ListPeers(ctx)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	for _, peerID := range peers {
		if ps.guard != nil && ps.guard.Suspicious(peerID) {
			continue
		}

		wg.Add(1)
		go func(pid PeerID) {
			defer wg.Done()

			if err := ps.RequestPoll(ctx, pid); err != nil {
				log.Printf("failed to send request poll to %s: %v", pid.String(), err)
			}
		}(peerID)
	}

	wg.Wait()
	return nil
}

// PollAllPeers broadcasts the local capability to peers respecting the poll cadence.
func (ps *PeerSync) PollAllPeers(ctx context.Context) {
	if ps.poller == nil {
		log.Printf("peersync: poller not configured, skipping PollAllPeers")
		return
	}
	ps.poller.PollAllPeers(ctx)
}

// ForcePollAllPeers broadcasts the local capability to all peers regardless of cadence.
func (ps *PeerSync) ForcePollAllPeers(ctx context.Context) {
	if ps.poller == nil {
		log.Printf("peersync: poller not configured, skipping ForcePollAllPeers")
		return
	}
	ps.poller.ForcePollAllPeers(ctx)
}

// RequestPoll requests a peer to send its capability information immediately.
func (ps *PeerSync) RequestPoll(ctx context.Context, peer PeerID) error {
	if ps.guard != nil && ps.guard.Suspicious(peer) {
		return nil
	}
	return ps.sendCapability(ctx, peer, messages.MESSAGETYPE_REQUEST_POLL)
}

func (ps *PeerSync) sendCapability(ctx context.Context, peer PeerID, msgType messages.MessageType) error {
	if ps.lightning == nil {
		return errors.New("lightning not configured")
	}

	capability := ps.localCapabilityForPeer(peer)
	msg := ps.capabilityToDTO(capability, peer)

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode capability payload: %w", err)
	}

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return ps.lightning.SendCustomMessage(cctx, peer, msgType, payload)
}

// Stop terminates the underlying receiver.
func (ps *PeerSync) Stop() error {
	if ps.lightning == nil {
		return errors.New("lightning not configured")
	}
	return ps.lightning.Stop()
}

func (ps *PeerSync) localCapabilityForPeer(peer PeerID) *PeerCapability {
	assets := make([]Asset, len(ps.supportedAssets))
	copy(assets, ps.supportedAssets)

	allowed := true
	if ps.guard != nil {
		allowed = ps.guard.Allow(peer)
	}

	var (
		btcIn   = premium.NewPPM(defaultPremiumRate(premium.BTC, premium.SwapIn))
		btcOut  = premium.NewPPM(defaultPremiumRate(premium.BTC, premium.SwapOut))
		lbtcIn  = premium.NewPPM(defaultPremiumRate(premium.LBTC, premium.SwapIn))
		lbtcOut = premium.NewPPM(defaultPremiumRate(premium.LBTC, premium.SwapOut))
	)

	if ps.guard != nil {
		btcIn = ps.guard.PremiumRate(peer, premium.BTC, premium.SwapIn)
		btcOut = ps.guard.PremiumRate(peer, premium.BTC, premium.SwapOut)
		lbtcIn = ps.guard.PremiumRate(peer, premium.LBTC, premium.SwapIn)
		lbtcOut = ps.guard.PremiumRate(peer, premium.LBTC, premium.SwapOut)
	}

	return NewPeerCapability(
		ps.version,
		assets,
		allowed,
		btcIn,
		btcOut,
		lbtcIn,
		lbtcOut,
	)
}

func (ps *PeerSync) capabilityToDTO(capability *PeerCapability, peer PeerID) *PollMessageDTO {
	snapshot := SnapshotFromCapability(capability)
	if snapshot == nil {
		return &PollMessageDTO{}
	}

	if ps.guard != nil {
		snapshot.PeerAllowed = ps.guard.Allow(peer)
	}

	return snapshot
}

func normalizeSupportedAssets(symbols []string) []Asset {
	if len(symbols) == 0 {
		return []Asset{AssetBTC, AssetLBTC}
	}

	assets := make([]Asset, 0, len(symbols))
	seen := make(map[Asset]struct{})
	for _, symbol := range symbols {
		s := strings.TrimSpace(symbol)
		if s == "" {
			continue
		}

		asset, err := NewAsset(strings.ToUpper(s))
		if err != nil || asset == AssetUnknown {
			log.Printf("peersync: ignoring unsupported asset %q: %v", symbol, err)
			continue
		}
		if _, ok := seen[asset]; ok {
			continue
		}
		seen[asset] = struct{}{}
		assets = append(assets, asset)
	}

	if len(assets) == 0 {
		return []Asset{AssetBTC, AssetLBTC}
	}
	return assets
}
