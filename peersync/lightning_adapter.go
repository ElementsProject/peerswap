package peersync

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"sync"

	"github.com/elementsproject/peerswap/lightning"
	"github.com/elementsproject/peerswap/messages"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LightningClient abstracts the subset of the lnrpc.LightningClient that is
// required by the LightningAdapter. This allows us to accept the real client in
// production while keeping tests small and isolated.
type LightningClient interface {
	//go:generate go run go.uber.org/mock/mockgen -destination=mocks/mock_lightning_client.go -package=mocks github.com/elementsproject/peerswap/peersync LightningClient
	SendCustomMessage(
		ctx context.Context,
		in *lnrpc.SendCustomMessageRequest,
		opts ...grpc.CallOption,
	) (*lnrpc.SendCustomMessageResponse, error)
	ListPeers(
		ctx context.Context,
		in *lnrpc.ListPeersRequest,
		opts ...grpc.CallOption,
	) (*lnrpc.ListPeersResponse, error)
	ListChannels(
		ctx context.Context,
		in *lnrpc.ListChannelsRequest,
		opts ...grpc.CallOption,
	) (*lnrpc.ListChannelsResponse, error)
	SubscribeCustomMessages(
		ctx context.Context,
		in *lnrpc.SubscribeCustomMessagesRequest,
		opts ...grpc.CallOption,
	) (lnrpc.Lightning_SubscribeCustomMessagesClient, error)
}

// LightningAdapter bridges a LightningClient to the peersync Lightning interface.
type LightningAdapter struct {
	client LightningClient
	bus    *messageBus

	mu            sync.Mutex
	streamStarted bool
	streamCancel  context.CancelFunc
}

// NewLightningAdapter returns a Lightning implementation backed by lnrpc types.
func NewLightningAdapter(client LightningClient) *LightningAdapter {
	return &LightningAdapter{
		client: client,
		bus:    newMessageBus(),
	}
}

// SendCustomMessage propagates the payload to the remote peer using the lightning client.
func (a *LightningAdapter) SendCustomMessage(
	ctx context.Context,
	to PeerID,
	msgType messages.MessageType,
	payload []byte,
) error {
	if a.client == nil {
		return errors.New("peersync adapter: client not configured")
	}

	peerBytes, err := hex.DecodeString(to.String())
	if err != nil {
		return fmt.Errorf("peersync adapter: invalid peer id: %w", err)
	}

	msgTypeValue := int64(msgType)
	if msgTypeValue < 0 || msgTypeValue > math.MaxUint32 {
		return fmt.Errorf("peersync adapter: message type %d out of range", msgTypeValue)
	}

	_, err = a.client.SendCustomMessage(ctx, &lnrpc.SendCustomMessageRequest{
		Peer: peerBytes,
		Type: uint32(msgTypeValue),
		Data: payload,
	})
	if err != nil {
		return err
	}

	return nil
}

// SubscribeCustomMessages consumes the lightning SubscribeCustomMessages stream and emits peersync messages.
func (a *LightningAdapter) SubscribeCustomMessages(ctx context.Context) (<-chan CustomMessage, error) {
	if a.client == nil {
		return nil, errors.New("peersync adapter: client not configured")
	}

	if err := a.ensureStream(ctx); err != nil {
		return nil, err
	}

	return a.bus.subscribe(ctx)
}

func (a *LightningAdapter) ensureStream(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.streamStarted {
		return nil
	}

	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := a.client.SubscribeCustomMessages(streamCtx, &lnrpc.SubscribeCustomMessagesRequest{})
	if err != nil {
		cancel()
		return err
	}

	a.streamStarted = true
	a.streamCancel = cancel

	go a.consumeStream(streamCtx, stream)

	return nil
}

func (a *LightningAdapter) consumeStream(ctx context.Context, stream lnrpc.Lightning_SubscribeCustomMessagesClient) {
	for {
		msg, err := stream.Recv()
		if err != nil {
			if shouldStopReceiving(err, ctx) {
				break
			}
			break
		}

		customMsg, ok := a.customMessageFrom(msg)
		if !ok {
			continue
		}

		a.bus.publish(customMsg)
	}

	a.mu.Lock()
	cancel := a.streamCancel
	a.streamCancel = nil
	a.streamStarted = false
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// Stop is a no-op because the underlying listener lifecycle is managed elsewhere.
func (a *LightningAdapter) Stop() error {
	a.mu.Lock()
	cancel := a.streamCancel
	a.streamCancel = nil
	a.streamStarted = false
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return nil
}

func (a *LightningAdapter) customMessageFrom(msg *lnrpc.CustomMessage) (CustomMessage, bool) {
	msgTypeHex := strconv.FormatUint(uint64(msg.Type), 16)

	messageType, err := messages.PeerswapCustomMessageType(msgTypeHex)
	if err != nil {
		return CustomMessage{}, false
	}

	id, err := NewPeerID(hex.EncodeToString(msg.Peer))
	if err != nil {
		return CustomMessage{}, false
	}

	return CustomMessage{From: id, Type: messageType, Payload: msg.Data}, true
}

func shouldStopReceiving(err error, ctx context.Context) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	if ctx.Err() != nil {
		return true
	}
	return status.Code(err) == codes.Canceled
}

// ListPeers converts the lightning peer list into peersync identifiers.
func (a *LightningAdapter) ListPeers(ctx context.Context) ([]PeerID, error) {
	if a.client == nil {
		return nil, errors.New("peersync adapter: client not configured")
	}

	resp, err := a.client.ListPeers(ctx, &lnrpc.ListPeersRequest{})
	if err != nil {
		return nil, err
	}

	result := make([]PeerID, 0, len(resp.Peers))
	for _, peer := range resp.Peers {
		id, err := NewPeerID(peer.PubKey)
		if err != nil {
			continue
		}
		result = append(result, id)
	}
	return result, nil
}

func (a *LightningAdapter) ListChannels(ctx context.Context) ([]Channel, error) {
	if a.client == nil {
		return nil, errors.New("peersync adapter: client not configured")
	}

	// We need both active and public channels to build optional 2-hop discovery
	// hints such as ChannelAdjacency.
	// We fetch all channels and filter in the caller so that future extensions
	// can adjust filtering without changing this adapter signature.
	resp, err := a.client.ListChannels(ctx, &lnrpc.ListChannelsRequest{
		ActiveOnly:   false,
		InactiveOnly: false,
		PublicOnly:   false,
		PrivateOnly:  false,
	})
	if err != nil {
		return nil, err
	}

	channels := make([]Channel, 0, len(resp.Channels))
	for _, ch := range resp.Channels {
		peer, err := NewPeerID(ch.RemotePubkey)
		if err != nil {
			continue
		}

		shortID := lnwire.NewShortChanIDFromInt(ch.ChanId)
		shortChannelID := lightning.Scid(shortID.String()).ClnStyle()

		channels = append(channels, Channel{
			Peer:           peer,
			ChannelID:      ch.ChanId,
			ShortChannelID: shortChannelID,
			Active:         ch.Active,
			Public:         !ch.Private,
		})
	}
	return channels, nil
}
