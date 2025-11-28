package peersync

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/messages"
)

type (
	sentCall struct {
		to      PeerID
		msgType messages.MessageType
		payload []byte
	}

	stubLightning struct {
		mu sync.Mutex

		peers    []PeerID
		ch       chan CustomMessage
		startErr error
		stopErr  error

		sends   []sentCall
		sendErr error
	}
)

func newStubLightning() *stubLightning {
	return &stubLightning{
		ch: make(chan CustomMessage),
	}
}

func (l *stubLightning) SendCustomMessage(
	ctx context.Context,
	to PeerID,
	msgType messages.MessageType,
	payload []byte,
) error {
	l.recordSend(sentCall{to: to, msgType: msgType, payload: payload})
	return l.sendErr
}

func (l *stubLightning) SubscribeCustomMessages(ctx context.Context) (<-chan CustomMessage, error) {
	return l.ch, l.startErr
}

func (l *stubLightning) Stop() error { return l.stopErr }

func (l *stubLightning) ListPeers(ctx context.Context) ([]PeerID, error) { return l.peers, nil }

func (l *stubLightning) recordSend(call sentCall) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sends = append(l.sends, call)
}

func (l *stubLightning) SentMessages() []sentCall {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]sentCall, len(l.sends))
	copy(out, l.sends)
	return out
}

func (l *stubLightning) SentCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.sends)
}

type syncDeps struct {
	store     *Store
	lightning *stubLightning
}

func newTestPeerSync(t *testing.T) (*PeerSync, syncDeps) {
	t.Helper()

	nodeID, err := NewPeerID("node-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "peers.db"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	deps := syncDeps{
		store:     store,
		lightning: newStubLightning(),
	}

	syncer := NewPeerSync(
		nodeID,
		deps.store,
		deps.lightning,
		nil,
		[]string{"btc", "lbtc"},
		nil,
	)

	return syncer, deps
}

func TestPerformInitialSync(t *testing.T) {
	syncer, deps := newTestPeerSync(t)

	ctx := context.Background()

	peerID, err := NewPeerID("peer-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deps.lightning.peers = []PeerID{peerID}
	if err := syncer.performInitialSync(ctx); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if deps.lightning.SentCount() != 1 {
		t.Fatalf("expected request poll to be sent, got %d calls", deps.lightning.SentCount())
	}
	sent := deps.lightning.SentMessages()
	if sent[0].to != peerID || sent[0].msgType != messages.MESSAGETYPE_REQUEST_POLL {
		t.Fatalf("unexpected send: %+v", sent[0])
	}

	var payload RequestPollMessageDTO
	if err := json.Unmarshal(sent[0].payload, &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if payload.Version == 0 {
		t.Fatalf("expected version to be set in request payload")
	}
	if !payload.PeerAllowed {
		t.Fatalf("expected request payload to mark peer as allowed")
	}
}

func TestPollAllPeers(t *testing.T) {
	syncer, deps := newTestPeerSync(t)

	peerID, err := NewPeerID("peer-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	peer := NewPeer(peerID, "addr-1")
	peer.SetLastPollAt(time.Now().Add(-time.Hour))
	peer.SetStatus(StatusActive)

	if err := deps.store.SavePeerState(peer); err != nil {
		t.Fatalf("failed to seed peer state: %v", err)
	}
	syncer.PollAllPeers(context.Background())

	if deps.lightning.SentCount() != 1 {
		t.Fatalf("expected poll call, got %d", deps.lightning.SentCount())
	}

	saved, err := deps.store.GetPeerState(peerID)
	if err != nil {
		t.Fatalf("failed to load peer state: %v", err)
	}
	if saved.LastPollAt().IsZero() || !saved.LastPollAt().After(peer.LastPollAt()) {
		t.Fatalf("expected peer state to be saved with updated timestamp")
	}
}

func TestHandlePollMessage(t *testing.T) {
	syncer, deps := newTestPeerSync(t)

	ctx := context.Background()

	peerID, err := NewPeerID("peer-remote")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	peer := NewPeer(peerID, "addr-remote")
	peer.SetStatus(StatusActive)
	if err := deps.store.SavePeerState(peer); err != nil {
		t.Fatalf("failed to seed peer state: %v", err)
	}
	payload := PollMessageDTO{
		Version:                   2,
		Assets:                    []string{"BTC"},
		PeerAllowed:               true,
		BTCSwapInPremiumRatePPM:   100,
		BTCSwapOutPremiumRatePPM:  200,
		LBTCSwapInPremiumRatePPM:  300,
		LBTCSwapOutPremiumRatePPM: 400,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	syncer.handler.handlePollMessage(ctx, CustomMessage{From: peerID, Type: messages.MESSAGETYPE_POLL, Payload: data})

	stored, err := deps.store.GetPeerState(peerID)
	if err != nil {
		t.Fatalf("failed to load peer state: %v", err)
	}

	if stored.Capability() == nil {
		t.Fatalf("expected capability to be set")
	}
	if stored.Capability().Version().Value() != NewVersion(2).Value() {
		t.Fatalf("unexpected capability version")
	}
}

func TestHandleRequestPollMessage(t *testing.T) {
	syncer, deps := newTestPeerSync(t)

	ctx := context.Background()

	peerID, err := NewPeerID("peer-request")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	request := RequestPollMessageDTO{}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	syncer.handler.handleRequestPollMessage(
		ctx,
		CustomMessage{
			From:    peerID,
			Type:    messages.MESSAGETYPE_REQUEST_POLL,
			Payload: data,
		},
	)

	if deps.lightning.SentCount() != 1 {
		t.Fatalf("expected response poll to be sent")
	}
}
