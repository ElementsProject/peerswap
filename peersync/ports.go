package peersync

import (
	"context"

	"github.com/elementsproject/peerswap/messages"
)

// CustomMessage is a generic lightning custom message flowing over the wire.
// The peersync package encodes/decodes its own payloads (DTOs) to/from this envelope.
type CustomMessage struct {
	From    PeerID
	Type    messages.MessageType
	Payload []byte
}

// Lightning groups lightning-node-dependent facilities: custom message I/O and peer listing.
// Implementations should bridge to the underlying node (e.g. lnd, cln).
type Lightning interface {
	// SendCustomMessage sends a raw custom message to a peer identified by its pubkey.
	SendCustomMessage(ctx context.Context, to PeerID, msgType messages.MessageType, payload []byte) error

	// SubscribeCustomMessages yields inbound custom messages until the context is canceled.
	SubscribeCustomMessages(ctx context.Context) (<-chan CustomMessage, error)

	// Stop terminates any internal listeners created by SubscribeCustomMessages.
	Stop() error

	// ListPeers returns currently connected peer IDs (node pubkeys).
	ListPeers(ctx context.Context) ([]PeerID, error)
}
