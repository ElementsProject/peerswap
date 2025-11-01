package peersync

import (
	"context"
	"errors"
	"sync"

	"github.com/elementsproject/peerswap/messages"
)

// GlightningClient abstracts the subset of the glightning.Lightning that is
// required by the CLN LightningAdapter. This keeps production wiring simple
// while enabling focused unit tests.
type GlightningClient interface {
	//go:generate go run go.uber.org/mock/mockgen -destination=mocks/mock_glightning_client.go -package=mocks github.com/elementsproject/peerswap/peersync GlightningClient
	// SendMessage sends a message with a numeric type to a peer.
	SendMessage(peerID string, message []byte, messageType int) error
	// GetPeers returns connected peer IDs (pubkeys) as strings.
	GetPeers() []string
	// AddMessageHandler registers a callback for inbound custom messages.
	// The callback receives (peerID, msgTypeHex, payloadBytes).
	AddMessageHandler(func(peerID, msgType string, payload []byte) error)
}

// ClnLightningAdapter bridges a GlightningClient to the peersync Lightning interface.
// For inbound messages, integrate by calling PushCustomMessage from the CLN plugin hook.
type ClnLightningAdapter struct {
	client GlightningClient

	mu          sync.Mutex
	subscribers []chan CustomMessage
}

// NewClnLightningAdapter returns a Lightning implementation backed by glightning.
func NewClnLightningAdapter(client GlightningClient) *ClnLightningAdapter {
	a := &ClnLightningAdapter{client: client}
	if a.client != nil {
		// Register a handler once to forward inbound messages into our subscribers.
		a.client.AddMessageHandler(func(peerID, msgType string, payload []byte) error {
			a.PushCustomMessage(peerID, msgType, payload)
			return nil
		})
	}
	return a
}

// SendCustomMessage encodes the message type and payload as expected by CLN and sends it.
func (a *ClnLightningAdapter) SendCustomMessage(
	ctx context.Context,
	to PeerID,
	msgType messages.MessageType,
	payload []byte,
) error {
	if a.client == nil {
		return errors.New("peersync cln adapter: client not configured")
	}

	// The cln client utility method takes raw payload and numeric type.
	return a.client.SendMessage(to.String(), payload, int(msgType))
}

// SubscribeCustomMessages returns a channel that receives inbound custom messages.
// Use PushCustomMessage to feed messages from the CLN plugin hook into this stream.
func (a *ClnLightningAdapter) SubscribeCustomMessages(ctx context.Context) (<-chan CustomMessage, error) {
	if a.client == nil {
		return nil, errors.New("peersync cln adapter: client not configured")
	}

	ch := make(chan CustomMessage, 8)

	a.mu.Lock()
	a.subscribers = append(a.subscribers, ch)
	a.mu.Unlock()

	go func() {
		<-ctx.Done()
		a.mu.Lock()
		// Remove ch from subscribers and close it
		for i, c := range a.subscribers {
			if c == ch {
				a.subscribers = append(a.subscribers[:i], a.subscribers[i+1:]...)
				break
			}
		}
		a.mu.Unlock()
		close(ch)
	}()

	return ch, nil
}

// PushCustomMessage feeds an inbound custom message (as delivered by the CLN plugin hook)
// into all active subscribers created via SubscribeCustomMessages.
// msgType must be a hex-encoded string per CLN (e.g. messages.MessageTypeToHexString(...)).
func (a *ClnLightningAdapter) PushCustomMessage(peerID, msgType string, payload []byte) {
	messageType, err := messages.PeerswapCustomMessageType(msgType)
	if err != nil {
		return
	}
	id, err := NewPeerID(peerID)
	if err != nil {
		return
	}
	a.mu.Lock()
	subs := append([]chan CustomMessage(nil), a.subscribers...)
	a.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- CustomMessage{From: id, Type: messageType, Payload: payload}:
		default:
			// Drop if subscriber is slow; upstream applies backpressure on its own.
		}
	}
}

// Stop is a no-op because lifecycle is owned by the plugin / caller.
func (a *ClnLightningAdapter) Stop() error { return nil }

// ListPeers converts CLN peers to peersync PeerIDs.
func (a *ClnLightningAdapter) ListPeers(ctx context.Context) ([]PeerID, error) {
	if a.client == nil {
		return nil, errors.New("peersync cln adapter: client not configured")
	}
	peerIDs := a.client.GetPeers()
	res := make([]PeerID, 0, len(peerIDs))
	for _, pid := range peerIDs {
		id, err := NewPeerID(pid)
		if err != nil {
			continue
		}
		res = append(res, id)
	}
	return res, nil
}
