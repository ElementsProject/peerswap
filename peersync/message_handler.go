package peersync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	pslog "github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/messages"
)

type messageHandler struct {
	store *Store
	logic *SyncLogic
	guard PeerGuard
	send  capabilitySender
}

func newMessageHandler(store *Store, logic *SyncLogic, guard PeerGuard, send capabilitySender) *messageHandler {
	return &messageHandler{
		store: store,
		logic: logic,
		guard: guard,
		send:  send,
	}
}

func (h *messageHandler) handleMessages(ctx context.Context, msgChan <-chan CustomMessage) {
	if h == nil {
		return
	}
	if h.store == nil || h.logic == nil {
		log.Printf("peersync: message handler not properly configured, dropping messages")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgChan:
			if !ok {
				return
			}
			h.processMessage(ctx, msg)
		}
	}
}

func (h *messageHandler) processMessage(ctx context.Context, msg CustomMessage) {
	switch msg.Type {
	case messages.MESSAGETYPE_POLL:
		h.handlePollMessage(ctx, msg)
	case messages.MESSAGETYPE_REQUEST_POLL:
		h.handleRequestPollMessage(ctx, msg)
	default:
		log.Printf("unknown message type: %v", msg.Type)
	}
}

func (h *messageHandler) handlePollMessage(ctx context.Context, msg CustomMessage) {
	if err := ctx.Err(); err != nil {
		return
	}

	peerID, snapshot, capability, err := h.parsePollMessage(msg)
	if err != nil {
		log.Printf("failed to parse poll message: %v", err)
		return
	}

	if h.guard != nil && h.guard.Suspicious(peerID) {
		return
	}

	peer, err := h.findPeer(peerID)
	if err != nil {
		log.Printf("unknown peer %s: %v", peerID.String(), err)
		return
	}

	if existing := peer.Capability(); existing != nil {
		capability = h.logic.MergeCapabilities(existing, capability)
	}

	peer.UpdateCapability(capability)
	if snapshot != nil && snapshot.ChannelAdjacency != nil {
		peer.UpdateChannelAdjacency(snapshot.ChannelAdjacency)
	}

	if err := h.store.SavePeerState(peer); err != nil {
		log.Printf("failed to store peer state: %v", err)
	}

	pslog.Infof("Received poll from peer %s", peerID.String())
}

func (h *messageHandler) handleRequestPollMessage(ctx context.Context, msg CustomMessage) {
	var payload RequestPollMessageDTO
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Printf("failed to decode request poll message: %v", err)
		return
	}

	fromPeerID := msg.From

	if h.guard != nil && h.guard.Suspicious(fromPeerID) {
		return
	}

	if h.send == nil {
		log.Printf("peersync: capability sender not configured, cannot respond to poll for %s", fromPeerID.String())
		return
	}

	if err := h.send(ctx, fromPeerID, messages.MESSAGETYPE_POLL); err != nil {
		log.Printf("failed to respond with poll to %s: %v", fromPeerID.String(), err)
	}
}

func (h *messageHandler) parsePollMessage(msg CustomMessage) (PeerID, *PollMessageDTO, *PeerCapability, error) {
	peerID := msg.From
	var payload PollMessageDTO
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return peerID, nil, nil, fmt.Errorf("decode poll message: %w", err)
	}

	capability, err := payload.ToCapability()
	if err != nil {
		return peerID, nil, nil, fmt.Errorf("invalid poll payload: %w", err)
	}

	return peerID, &payload, capability, nil
}

func (h *messageHandler) findPeer(peerID PeerID) (*Peer, error) {
	if h.store == nil {
		return nil, errors.New("store not configured")
	}

	peer, err := h.store.GetPeerState(peerID)
	if err == nil {
		return peer, nil
	}
	if errors.Is(err, ErrPeerNotFound) {
		return NewPeer(peerID, ""), nil
	}
	return nil, err
}
