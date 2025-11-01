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
    pslog "github.com/elementsproject/peerswap/log"
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
	if remote.Version().Value() > local.Version().Value() {
		return remote
	}
	return local
}

// PeerSync orchestrates polling and capability exchange between peers.
type PeerSync struct {
	nodeID          PeerID
	logic           *SyncLogic
	store           *Store
	lightning       Lightning
	policy          *policy.Policy
	supportedAssets []Asset
	premiumSetting  *premium.Setting
	version         Version

	pollTickerInterval    time.Duration
	cleanupTickerInterval time.Duration
	cleanupTimeout        time.Duration
}

// NewPeerSync wires dependencies into a new synchronizer instance.
func NewPeerSync(
	nodeID PeerID,
	store *Store,
	lightning Lightning,
	policyCfg *policy.Policy,
	supportedAssets []string,
	premiumSetting *premium.Setting,
) *PeerSync {
	ps := &PeerSync{
		nodeID:                nodeID,
		logic:                 NewSyncLogic(),
		store:                 store,
		lightning:             lightning,
		policy:                policyCfg,
		supportedAssets:       normalizeSupportedAssets(supportedAssets),
		premiumSetting:        premiumSetting,
		version:               NewVersion(swap.PEERSWAP_PROTOCOL_VERSION),
		pollTickerInterval:    10 * time.Second,
		cleanupTickerInterval: 1 * time.Minute,
		cleanupTimeout:        30 * time.Minute,
	}

	if premiumSetting != nil {
		premiumSetting.AddObserver(ps)
	}

	return ps
}

// Start launches the polling, cleanup, and message handling loops.
func (ps *PeerSync) Start(ctx context.Context) error {
	if err := ps.performInitialSync(ctx); err != nil {
		log.Printf("initial sync failed: %v", err)
	}

	msgChan, err := ps.lightning.SubscribeCustomMessages(ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe to custom messages: %w", err)
	}

	go ps.runPollLoop(ctx)
	go ps.runCleanupLoop(ctx)
	go ps.handleMessages(ctx, msgChan)

	return nil
}

func (ps *PeerSync) performInitialSync(ctx context.Context) error {
	peers, err := ps.lightning.ListPeers(ctx)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	for _, peerID := range peers {
		if ps.policy != nil && ps.policy.IsPeerSuspicious(peerID.String()) {
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

func (ps *PeerSync) runPollLoop(ctx context.Context) {
	ticker := time.NewTicker(ps.pollTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ps.PollAllPeers(ctx)
		}
	}
}

func (ps *PeerSync) runCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(ps.cleanupTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := ps.store.CleanupExpired(ps.cleanupTimeout); err != nil {
				log.Printf("cleanup expired peers failed: %v", err)
			}
		}
	}
}

// PollAllPeers broadcasts the local capability to peers respecting the poll cadence.
func (ps *PeerSync) PollAllPeers(ctx context.Context) {
	ps.pollPeers(ctx, false)
}

// ForcePollAllPeers broadcasts the local capability to all peers regardless of cadence.
func (ps *PeerSync) ForcePollAllPeers(ctx context.Context) {
	ps.pollPeers(ctx, true)
}

func (ps *PeerSync) pollPeers(ctx context.Context, force bool) {
	peers, err := ps.store.GetAllPeerStates()
	if err != nil {
		log.Printf("failed to get peers: %v", err)
		return
	}

	for _, peer := range peers {
		if !force && !ps.logic.ShouldPoll(peer) {
			continue
		}

		if ps.policy != nil && ps.policy.IsPeerSuspicious(peer.ID().String()) {
			continue
		}

		if err := ps.sendCapability(ctx, peer.ID(), messages.MESSAGETYPE_POLL); err != nil {
			log.Printf("failed to poll %s: %v", peer.ID().String(), err)
			continue
		}

		peer.MarkAsPolled()
		if err := ps.store.SavePeerState(peer); err != nil {
			log.Printf("failed to persist peer state for %s: %v", peer.ID().String(), err)
		}
	}
}

func (ps *PeerSync) handleMessages(ctx context.Context, msgChan <-chan CustomMessage) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgChan:
			if !ok {
				return
			}
			ps.processMessage(ctx, msg)
		}
	}
}

func (ps *PeerSync) processMessage(ctx context.Context, msg CustomMessage) {
	switch msg.Type {
	case messages.MESSAGETYPE_POLL:
		ps.handlePollMessage(ctx, msg)
	case messages.MESSAGETYPE_REQUEST_POLL:
		ps.handleRequestPollMessage(ctx, msg)
	default:
		log.Printf("unknown message type: %v", msg.Type)
	}
}

func (ps *PeerSync) handlePollMessage(ctx context.Context, msg CustomMessage) {
	if err := ctx.Err(); err != nil {
		return
	}

	peerID, capability, err := ps.parsePollMessage(msg)
	if err != nil {
		log.Printf("failed to parse poll message: %v", err)
		return
	}

	if ps.policy != nil && ps.policy.IsPeerSuspicious(peerID.String()) {
		return
	}

    peer, err := ps.findPeer(peerID)
    if err != nil {
        log.Printf("unknown peer %s: %v", peerID.String(), err)
        return
    }

	if existing := peer.Capability(); existing != nil {
		capability = ps.logic.MergeCapabilities(existing, capability)
	}

	peer.UpdateCapability(capability)

    if err := ps.store.SavePeerState(peer); err != nil {
        log.Printf("failed to store peer state: %v", err)
    }

    // Emit an informational log for successful poll handling to aid
    // integration tests and operational debugging. Historic tests wait
    // for this message to confirm that peers have exchanged poll data.
    // Use project logger to ensure logs are written to stdout.
    pslog.Infof("Received poll from peer %s", peerID.String())
}

func (ps *PeerSync) handleRequestPollMessage(ctx context.Context, msg CustomMessage) {
	var payload RequestPollMessageDTO
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Printf("failed to decode request poll message: %v", err)
		return
	}

	fromPeerID := msg.From

	if ps.policy != nil && ps.policy.IsPeerSuspicious(fromPeerID.String()) {
		return
	}

	if err := ps.sendCapability(ctx, fromPeerID, messages.MESSAGETYPE_POLL); err != nil {
		log.Printf("failed to respond with poll to %s: %v", fromPeerID.String(), err)
	}
}

// RequestPoll requests a peer to send its capability information immediately.
func (ps *PeerSync) RequestPoll(ctx context.Context, peer PeerID) error {
	if ps.policy != nil && ps.policy.IsPeerSuspicious(peer.String()) {
		return nil
	}
	return ps.sendCapability(ctx, peer, messages.MESSAGETYPE_REQUEST_POLL)
}

func (ps *PeerSync) sendCapability(ctx context.Context, peer PeerID, msgType messages.MessageType) error {
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

func (ps *PeerSync) localCapabilityForPeer(peer PeerID) *PeerCapability {
	assets := make([]Asset, len(ps.supportedAssets))
	copy(assets, ps.supportedAssets)

	allowed := true
	if ps.policy != nil {
		allowed = ps.policy.IsPeerAllowed(peer.String())
	}

	btcIn := ps.premiumRateForPeer(peer, premium.BTC, premium.SwapIn)
	btcOut := ps.premiumRateForPeer(peer, premium.BTC, premium.SwapOut)
	lbtcIn := ps.premiumRateForPeer(peer, premium.LBTC, premium.SwapIn)
	lbtcOut := ps.premiumRateForPeer(peer, premium.LBTC, premium.SwapOut)

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

func (ps *PeerSync) premiumRateForPeer(
	peer PeerID,
	asset premium.AssetType,
	operation premium.OperationType,
) *premium.PPM {
	defaultValue := defaultPremiumRate(asset, operation)

	if ps.premiumSetting == nil {
		return premium.NewPPM(defaultValue)
	}

	rate, err := ps.premiumSetting.GetRate(peer.String(), asset, operation)
	if err != nil {
		log.Printf("failed to get premium rate for %s (%s/%s): %v", peer.String(), asset, operation, err)
		return premium.NewPPM(defaultValue)
	}

	if rate == nil || rate.PremiumRatePPM() == nil {
		return premium.NewPPM(defaultValue)
	}

	return rate.PremiumRatePPM()
}

// OnPremiumUpdate is invoked when premium settings change, forcing a broadcast.
func (ps *PeerSync) OnPremiumUpdate() {
	go ps.ForcePollAllPeers(context.Background())
}

func (ps *PeerSync) capabilityToDTO(capability *PeerCapability, peer PeerID) *PollMessageDTO {
	assets := make([]string, 0, len(capability.SupportedAssets()))
	for _, a := range capability.SupportedAssets() {
		assets = append(assets, a.String())
	}

	allowed := capability.IsAllowed()
	peerIDValue := peer.String()
	if ps.policy != nil && peerIDValue != "" {
		allowed = ps.policy.IsPeerAllowed(peerIDValue)
	}

	return &PollMessageDTO{
		Version:                   capability.Version().Value(),
		Assets:                    assets,
		PeerAllowed:               allowed,
		BTCSwapInPremiumRatePPM:   ppmValue(capability.GetPremiumRate(premium.BTC, premium.SwapIn)),
		BTCSwapOutPremiumRatePPM:  ppmValue(capability.GetPremiumRate(premium.BTC, premium.SwapOut)),
		LBTCSwapInPremiumRatePPM:  ppmValue(capability.GetPremiumRate(premium.LBTC, premium.SwapIn)),
		LBTCSwapOutPremiumRatePPM: ppmValue(capability.GetPremiumRate(premium.LBTC, premium.SwapOut)),
	}
}

func (ps *PeerSync) dtoToCapability(dto PollMessageDTO) (*PeerCapability, error) {
	assets := make([]Asset, 0, len(dto.Assets))
	for _, symbol := range dto.Assets {
		asset, err := NewAsset(symbol)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}

	btcIn, err := NewPremiumRate(dto.BTCSwapInPremiumRatePPM)
	if err != nil {
		return nil, err
	}
	btcOut, err := NewPremiumRate(dto.BTCSwapOutPremiumRatePPM)
	if err != nil {
		return nil, err
	}
	lbtcIn, err := NewPremiumRate(dto.LBTCSwapInPremiumRatePPM)
	if err != nil {
		return nil, err
	}
	lbtcOut, err := NewPremiumRate(dto.LBTCSwapOutPremiumRatePPM)
	if err != nil {
		return nil, err
	}

	return NewPeerCapability(
		NewVersion(dto.Version),
		assets,
		dto.PeerAllowed,
		btcIn,
		btcOut,
		lbtcIn,
		lbtcOut,
	), nil
}

func (ps *PeerSync) parsePollMessage(
	msg CustomMessage,
) (PeerID, *PeerCapability, error) {
	peerID := msg.From
	var payload PollMessageDTO
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return peerID, nil, fmt.Errorf("decode poll message: %w", err)
	}

	capability, err := ps.dtoToCapability(payload)
	if err != nil {
		return peerID, nil, fmt.Errorf("invalid poll payload: %w", err)
	}

	return peerID, capability, nil
}

func (ps *PeerSync) findPeer(peerID PeerID) (*Peer, error) {
	peer, err := ps.store.GetPeerState(peerID)
	if err == nil {
		return peer, nil
	}
	if errors.Is(err, ErrPeerNotFound) {
		return NewPeer(peerID, ""), nil
	}
	return nil, err
}

// Stop terminates the underlying receiver.
func (ps *PeerSync) Stop() error {
	if ps.lightning == nil {
		return errors.New("lightning not configured")
	}
	return ps.lightning.Stop()
}

func defaultPremiumRate(asset premium.AssetType, operation premium.OperationType) int64 {
	if ratesByAsset, ok := premium.DefaultPremiumRate[asset]; ok {
		if value, ok := ratesByAsset[operation]; ok {
			return value
		}
	}
	return 0
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

// PollMessageDTO carries capability information in transport messages.
type PollMessageDTO struct {
	Version                   uint64   `json:"version"`
	Assets                    []string `json:"assets"`
	PeerAllowed               bool     `json:"peer_allowed"`
	BTCSwapInPremiumRatePPM   int64    `json:"btc_swap_in_premium_rate_ppm"`
	BTCSwapOutPremiumRatePPM  int64    `json:"btc_swap_out_premium_rate_ppm"`
	LBTCSwapInPremiumRatePPM  int64    `json:"lbtc_swap_in_premium_rate_ppm"`
	LBTCSwapOutPremiumRatePPM int64    `json:"lbtc_swap_out_premium_rate_ppm"`
}

// RequestPollMessageDTO requests a peer to send an updated poll message.
type RequestPollMessageDTO = PollMessageDTO
