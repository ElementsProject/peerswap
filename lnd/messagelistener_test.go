package lnd

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/cmd/peerswaplnd"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// TestMessageListener tests the basic functionality of the listener. A handler
// is added and a message is sent to the node that the listener is subscribed
// to. We expect the handler to be called.
func TestMessageListener(t *testing.T) {
	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	_, sender, receiver, cc, err := messageListenerNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()

	// Create new message listener.
	messageListener, err := NewMessageListener(ctx, cc)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}
	defer messageListener.Stop()

	// Start message listener.
	messageListener.Start()

	// Add a handler.
	var gotMessage bool
	messageListener.AddMessageHandler(func(peerId, msgType string, payload []byte) error {
		gotMessage = true
		return nil
	})

	// Prepare message params.
	peer, err := hex.DecodeString(receiver.Info.IdentityPubkey)
	if err != nil {
		t.Fatalf("Failed DecodeString(): %v", err)
	}
	var typ uint32 = 42069
	data := "mydata"

	sender.Rpc.SendCustomMessage(context.Background(), &lnrpc.SendCustomMessageRequest{
		Peer: peer,
		Type: typ,
		Data: []byte(data),
	})

	err = testframework.WaitFor(func() bool {
		return gotMessage
	}, 20*time.Second)
	assert.NoError(t, err)
}

// TestMessageListener_Reconnect tests that the listener is able to receive
// messages after the node that the listener is subscribed to crashed and
// restarted. After a handler was added to the listener the node that the
// listener is subscribed to is killed. After a random time between 1s and 5s
// the node is restarted and a custom message is sent to the node. We expect the
// handler to be called.
func TestMessageListener_Reconnect(t *testing.T) {
	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	_, sender, receiver, cc, err := messageListenerNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()

	// Create new message listener.
	messageListener, err := NewMessageListener(ctx, cc)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}
	defer messageListener.Stop()

	// Start message listener.
	messageListener.Start()

	// Add a handler.
	var gotMessage bool
	messageListener.AddMessageHandler(func(peerId, msgType string, payload []byte) error {
		gotMessage = true
		return nil
	})

	receiver.Kill()
	// Restart lnd
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	err = receiver.Run(true, true)
	if err != nil {
		t.Fatalf("Failed Run(): %v", err)
	}

	// We still assert the callback was not called by now.
	assert.False(t, gotMessage)

	// We have to reconnect in order to pay the invoice.
	err = sender.Connect(receiver, true)
	if err != nil {
		t.Fatalf("Failed Connect(): %v", err)
	}

	// Prepare message params.
	peer, err := hex.DecodeString(receiver.Info.IdentityPubkey)
	if err != nil {
		t.Fatalf("Failed DecodeString(): %v", err)
	}
	var typ uint32 = 42069
	data := "mydata"

	go func() {
		for {
			sender.Rpc.SendCustomMessage(context.Background(), &lnrpc.SendCustomMessageRequest{
				Peer: peer,
				Type: typ,
				Data: []byte(data),
			})
			time.Sleep(1 * time.Second)
		}
	}()

	err = testframework.WaitFor(func() bool {
		return gotMessage
	}, 20*time.Second)
	assert.NoError(t, err)
}

func messageListenerNodeSetup(t *testing.T, dir string) (
	bitcoind *testframework.BitcoinNode,
	sender *testframework.LndNode,
	receiver *testframework.LndNode,
	cc *grpc.ClientConn, err error,
) {
	bitcoind, err = testframework.NewBitcoinNode(dir, 1)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not create bitcoin node: %v", err)
	}

	sender, err = testframework.NewLndNode(dir, bitcoind, 1, nil)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not create lnd node: %v", err)
	}

	receiver, err = testframework.NewLndNode(dir, bitcoind, 1, nil)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not create lnd node: %v", err)
	}

	if err := bitcoind.Run(true); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not start bitcoind: %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	if err := sender.Run(true, true); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not start lnd: %v", err)
	}
	t.Cleanup(sender.Kill)

	if err := receiver.Run(true, true); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not start lnd: %v", err)
	}
	t.Cleanup(receiver.Kill)

	// Create a client connection to the lnd node. And a new lnd client.
	cc, err = GetClientConnection(
		context.Background(),
		&peerswaplnd.LndConfig{
			LndHost:      fmt.Sprintf("localhost:%d", receiver.RpcPort),
			TlsCertPath:  receiver.TlsPath,
			MacaroonPath: receiver.MacaroonPath,
		},
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Could not create lnd client connection: %v", err)
	}

	err = sender.Connect(receiver, true)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Could not connect to receiver")
	}

	return bitcoind, sender, receiver, cc, nil
}
