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

	peerID, err := h.storeCapabilityMessage(msg)
	if err != nil {
		log.Printf("failed to store poll message: %v", err)
		return
	}

	pslog.Infof("Received poll from peer %s", peerID.String())
}

func (h *messageHandler) handleRequestPollMessage(ctx context.Context, msg CustomMessage) {
	fromPeerID, err := h.storeCapabilityMessage(msg)
	if err != nil {
		log.Printf("failed to store request poll message: %v", err)
		return
	}

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

func (h *messageHandler) storeCapabilityMessage(msg CustomMessage) (PeerID, error) {
	peerID := msg.From
	capability, err := h.parseCapabilityMessage(msg)
	if err != nil {
		return peerID, err
	}

	if h.guard != nil && h.guard.Suspicious(peerID) {
		return peerID, nil
	}

	peer, err := h.findPeer(peerID)
	if err != nil {
		return peerID, fmt.Errorf("unknown peer %s: %w", peerID.String(), err)
	}

	if existing := peer.Capability(); existing != nil {
		capability = h.logic.MergeCapabilities(existing, capability)
	}

	peer.UpdateCapability(capability)

	if err := h.store.SavePeerState(peer); err != nil {
		return peerID, fmt.Errorf("failed to store peer state: %w", err)
	}

	return peerID, nil
}

func (h *messageHandler) parseCapabilityMessage(msg CustomMessage) (*PeerCapability, error) {
	var payload PeerCapabilitySnapshot
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode capability message: %w", err)
	}

	capability, err := payload.ToCapability()
	if err != nil {
		return nil, fmt.Errorf("invalid capability payload: %w", err)
	}

	return capability, nil
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
