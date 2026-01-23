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

	// ListChannels returns channels known to the local node.
	// Implementations should include at least peer id, short channel id, and
	// whether the channel is public/active.
	ListChannels(ctx context.Context) ([]Channel, error)
}

// Channel describes a local channel in a transport-friendly way.
// It is used to build optional metadata such as ChannelAdjacency.
type Channel struct {
	Peer           PeerID
	ChannelID      uint64
	ShortChannelID string
	Active         bool
	Public         bool
}
