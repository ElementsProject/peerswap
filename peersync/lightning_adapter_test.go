package peersync

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/messages"
	psmocks "github.com/elementsproject/peerswap/peersync/mocks"
	"github.com/lightningnetwork/lnd/lnrpc"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type streamMessage struct {
	msg *lnrpc.CustomMessage
	err error
}

type mockCustomMessageStream struct {
	ctx    context.Context
	result chan streamMessage
}

func newMockCustomMessageStream(buffer int) *mockCustomMessageStream {
	return &mockCustomMessageStream{
		result: make(chan streamMessage, buffer),
	}
}

func (m *mockCustomMessageStream) push(msg *lnrpc.CustomMessage) {
	m.result <- streamMessage{msg: msg}
}

func (m *mockCustomMessageStream) close() {
	close(m.result)
}

func (m *mockCustomMessageStream) Recv() (*lnrpc.CustomMessage, error) {
	res, ok := <-m.result
	if !ok {
		return nil, io.EOF
	}
	if res.err != nil {
		return nil, res.err
	}
	return res.msg, nil
}

func (m *mockCustomMessageStream) Header() (metadata.MD, error) {
	return metadata.MD{}, nil
}

func (m *mockCustomMessageStream) Trailer() metadata.MD {
	return metadata.MD{}
}

func (m *mockCustomMessageStream) CloseSend() error {
	return nil
}

func (m *mockCustomMessageStream) Context() context.Context {
	return m.ctx
}

func (m *mockCustomMessageStream) SendMsg(any) error {
	return nil
}

func (m *mockCustomMessageStream) RecvMsg(any) error {
	return nil
}

func TestLightningAdapterSendCustomMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	client := psmocks.NewMockLightningClient(ctrl)
	adapter := NewLightningAdapter(client)

	peerIDValue := "022222222222222222222222222222222222222222222222222222222222222222"
	peerID, err := NewPeerID(peerIDValue)
	if err != nil {
		t.Fatalf("unexpected error creating peer id: %v", err)
	}

	payload := []byte("hello")
	client.EXPECT().SendCustomMessage(gomock.Any(), gomock.AssignableToTypeOf(&lnrpc.SendCustomMessageRequest{})).
		DoAndReturn(func(ctx context.Context, req *lnrpc.SendCustomMessageRequest, _ ...grpc.CallOption) (*lnrpc.SendCustomMessageResponse, error) {
			expectedPeer, _ := hex.DecodeString(peerIDValue)
			if !bytes.Equal(req.Peer, expectedPeer) {
				t.Fatalf("unexpected peer bytes: got %x want %x", req.Peer, expectedPeer)
			}
			if req.Type != uint32(messages.MESSAGETYPE_POLL) {
				t.Fatalf("unexpected message type: got %d want %d", req.Type, messages.MESSAGETYPE_POLL)
			}
			if !bytes.Equal(req.Data, payload) {
				t.Fatalf("unexpected payload: got %s want %s", string(req.Data), string(payload))
			}
			return &lnrpc.SendCustomMessageResponse{}, nil
		})

	if err := adapter.SendCustomMessage(context.Background(), peerID, messages.MESSAGETYPE_POLL, payload); err != nil {
		t.Fatalf("SendCustomMessage returned error: %v", err)
	}
}

func TestLightningAdapterSendCustomMessageInvalidPeer(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	client := psmocks.NewMockLightningClient(ctrl)
	adapter := NewLightningAdapter(client)

	invalidPeer, err := NewPeerID("not-hex")
	if err != nil {
		t.Fatalf("unexpected error creating peer id: %v", err)
	}

	err = adapter.SendCustomMessage(context.Background(), invalidPeer, messages.MESSAGETYPE_POLL, nil)
	if err == nil {
		t.Fatalf("expected error for invalid peer id")
	}
}

func TestLightningAdapterSubscribeCustomMessages(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	stream := newMockCustomMessageStream(1)
	client := psmocks.NewMockLightningClient(ctrl)
	client.EXPECT().
		SubscribeCustomMessages(gomock.Any(), gomock.AssignableToTypeOf(&lnrpc.SubscribeCustomMessagesRequest{})).
		DoAndReturn(func(ctx context.Context, req *lnrpc.SubscribeCustomMessagesRequest, _ ...grpc.CallOption) (lnrpc.Lightning_SubscribeCustomMessagesClient, error) {
			stream.ctx = ctx
			return stream, nil
		})

	adapter := NewLightningAdapter(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := adapter.SubscribeCustomMessages(ctx)
	if err != nil {
		t.Fatalf("SubscribeCustomMessages returned error: %v", err)
	}

	peerID := "033333333333333333333333333333333333333333333333333333333333333333"
	payload := []byte("payload")

	peerBytes, err := hex.DecodeString(peerID)
	if err != nil {
		t.Fatalf("unexpected error decoding peer id: %v", err)
	}

	stream.push(&lnrpc.CustomMessage{
		Peer: peerBytes,
		Type: uint32(messages.MESSAGETYPE_POLL),
		Data: payload,
	})

	var msg CustomMessage
	select {
	case msg = <-ch:
		if msg.From.String() != peerID {
			t.Fatalf("unexpected peer id: got %s want %s", msg.From.String(), peerID)
		}
		if msg.Type != messages.MESSAGETYPE_POLL {
			t.Fatalf("unexpected message type: got %v want %v", msg.Type, messages.MESSAGETYPE_POLL)
		}
		if !bytes.Equal(msg.Payload, payload) {
			t.Fatalf("unexpected payload: got %s want %s", string(msg.Payload), string(payload))
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for message")
	}

	stream.close()
}

func TestLightningAdapterListPeers(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	client := psmocks.NewMockLightningClient(ctrl)
	client.EXPECT().
		ListPeers(gomock.Any(), gomock.AssignableToTypeOf(&lnrpc.ListPeersRequest{})).
		Return(&lnrpc.ListPeersResponse{
			Peers: []*lnrpc.Peer{
				{PubKey: "044444444444444444444444444444444444444444444444444444444444444444"},
				{PubKey: "invalid"},
			},
		}, nil)

	adapter := NewLightningAdapter(client)

	peers, err := adapter.ListPeers(context.Background())
	if err != nil {
		t.Fatalf("ListPeers returned error: %v", err)
	}

	if len(peers) != 2 {
		t.Fatalf("expected two peers, got %d", len(peers))
	}

	if peers[0].String() != "044444444444444444444444444444444444444444444444444444444444444444" {
		t.Fatalf("unexpected first peer id: %s", peers[0].String())
	}
}

func TestLightningAdapterListPeersError(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	client := psmocks.NewMockLightningClient(ctrl)
	client.EXPECT().
		ListPeers(gomock.Any(), gomock.AssignableToTypeOf(&lnrpc.ListPeersRequest{})).
		Return(nil, errors.New("boom"))

	adapter := NewLightningAdapter(client)

	if _, err := adapter.ListPeers(context.Background()); err == nil {
		t.Fatalf("expected error from ListPeers")
	}
}
