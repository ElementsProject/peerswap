package peersync

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/messages"
	psmocks "github.com/elementsproject/peerswap/peersync/mocks"
	"go.uber.org/mock/gomock"
)

func TestClnLightningAdapter_SendCustomMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	client := psmocks.NewMockGlightningClient(ctrl)
	client.EXPECT().AddMessageHandler(gomock.Any())
	adapter := NewClnLightningAdapter(client)

	// Given a peer id and payload
	peerIDStr := "022222222222222222222222222222222222222222222222222222222222222222"
	peerID, err := NewPeerID(peerIDStr)
	if err != nil {
		t.Fatalf("unexpected error creating peer id: %v", err)
	}
	payload := []byte("hello")

	// Expect SendMessage(peerId, payload, type)
	client.
		EXPECT().
		SendMessage(peerIDStr, payload, int(messages.MESSAGETYPE_POLL)).
		Return(nil)

	if err := adapter.SendCustomMessage(context.Background(), peerID, messages.MESSAGETYPE_POLL, payload); err != nil {
		t.Fatalf("SendCustomMessage returned error: %v", err)
	}
}

func TestClnLightningAdapter_SendCustomMessage_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	client := psmocks.NewMockGlightningClient(ctrl)
	client.EXPECT().AddMessageHandler(gomock.Any())
	adapter := NewClnLightningAdapter(client)

	peerIDStr := "03aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	peerID, _ := NewPeerID(peerIDStr)

	client.
		EXPECT().
		SendMessage(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("boom"))

	if err := adapter.SendCustomMessage(context.Background(), peerID, messages.MESSAGETYPE_POLL, nil); err == nil {
		t.Fatalf("expected error from SendCustomMessage")
	}
}

func TestClnLightningAdapter_SubscribeAndPush(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	client := psmocks.NewMockGlightningClient(ctrl)
	// ListPeers should not be called here; no expectations are needed.
	client.EXPECT().AddMessageHandler(gomock.Any())
	adapter := NewClnLightningAdapter(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := adapter.SubscribeCustomMessages(ctx)
	if err != nil {
		t.Fatalf("SubscribeCustomMessages returned error: %v", err)
	}

	// Feed one message
	peerIDStr := "033333333333333333333333333333333333333333333333333333333333333333"
	payload := []byte("payload")
	adapter.PushCustomMessage(peerIDStr, messages.MessageTypeToHexString(messages.MESSAGETYPE_POLL), payload)

	select {
	case msg := <-ch:
		if msg.From.String() != peerIDStr {
			t.Fatalf("unexpected peer id: got %s want %s", msg.From.String(), peerIDStr)
		}
		if msg.Type != messages.MESSAGETYPE_POLL {
			t.Fatalf("unexpected message type: got %v want %v", msg.Type, messages.MESSAGETYPE_POLL)
		}
		if hex.EncodeToString(msg.Payload) != hex.EncodeToString(payload) {
			t.Fatalf("unexpected payload: got %x want %x", msg.Payload, payload)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for message")
	}
}

func TestClnLightningAdapter_ListPeers(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	client := psmocks.NewMockGlightningClient(ctrl)
	client.EXPECT().AddMessageHandler(gomock.Any())
	client.EXPECT().GetPeers().Return([]string{
		"044444444444444444444444444444444444444444444444444444444444444444",
		"invalid",
	})

	adapter := NewClnLightningAdapter(client)

	peers, err := adapter.ListPeers(context.Background())
	if err != nil {
		t.Fatalf("ListPeers returned error: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}
	if peers[0].String() != "044444444444444444444444444444444444444444444444444444444444444444" {
		t.Fatalf("unexpected first peer id: %s", peers[0].String())
	}
}

func TestClnLightningAdapter_ListPeers_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	client := psmocks.NewMockGlightningClient(ctrl)
	client.EXPECT().AddMessageHandler(gomock.Any())
	client.EXPECT().GetPeers().Return(nil)

	adapter := NewClnLightningAdapter(client)
	peers, err := adapter.ListPeers(context.Background())
	if err != nil {
		t.Fatalf("ListPeers returned error: %v", err)
	}
	if len(peers) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(peers))
	}
}
