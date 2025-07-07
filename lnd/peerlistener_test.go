package lnd

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/test"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/stretchr/testify/assert"
)

// TestPeerListener tests the basic functionality of the peer listener. The peer
// disconnects and reconnects. We expect to have a handler called for both
// events.
func TestPeerListener(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	_, peer, node, cc, err := messageListenerNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()

	// Create new message listener.
	peerListener, err := NewPeerListener(ctx, cc)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}
	defer peerListener.Stop()

	// Add handlers
	var gotOfflineEvent bool
	peerListener.AddHandler(lnrpc.PeerEvent_PEER_OFFLINE, func(_ string) {
		gotOfflineEvent = true
	})

	var gotOnlineEvent bool
	peerListener.AddHandler(lnrpc.PeerEvent_PEER_ONLINE, func(_ string) {
		gotOnlineEvent = true
	})

	peer.Kill()

	err = testframework.WaitFor(func() bool {
		return gotOfflineEvent
	}, 50*time.Second)
	assert.NoError(t, err)

	err = peer.Run(true, true)
	if err != nil {
		t.Fatalf("Failed Run(): %v", err)
	}

	err = peer.Connect(node, true)
	if err != nil {
		t.Fatalf("Failed Connect(): %v", err)
	}

	err = testframework.WaitFor(func() bool {
		return gotOnlineEvent
	}, 50*time.Second)
	assert.NoError(t, err)
}

// TestPeerListener tests that the PeerListener reconnects to the lnd node if
// the lnd node was killed and is online again.
func TestPeerListener_Reconnect(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	_, peer, node, cc, err := messageListenerNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()

	// Create new message listener.
	peerListener, err := NewPeerListener(ctx, cc)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}
	defer peerListener.Stop()

	var gotOnlineEvent bool
	peerListener.AddHandler(lnrpc.PeerEvent_PEER_ONLINE, func(_ string) {
		gotOnlineEvent = true
	})

	// Kill the node and restart after a few seconds to simulate a disconnect to
	// the lnd node.
	node.Kill()
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	err = node.Run(true, true)
	if err != nil {
		t.Fatalf("Failed Run(): %v", err)
	}

	peer.Kill()
	err = peer.Run(true, true)
	if err != nil {
		t.Fatalf("Failed Run(): %v", err)
	}

	err = peer.Connect(node, true)
	if err != nil {
		t.Fatalf("Failed Connect(): %v", err)
	}

	err = testframework.WaitFor(func() bool {
		return gotOnlineEvent
	}, 50*time.Second)
	assert.NoError(t, err)
}

// TestPeerListener_Reconnect_OnGracefulStop tests that the PeerListener reconnects 
// to the lnd node if the lnd node was gracefully shutdown and then restarted again.
func TestPeerListener_Reconnect_OnGracefulStop(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	_, peer, node, cc, err := messageListenerNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()

	// Create new message listener.
	peerListener, err := NewPeerListener(ctx, cc)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}
	defer peerListener.Stop()

	var gotOnlineEvent bool
	peerListener.AddHandler(lnrpc.PeerEvent_PEER_ONLINE, func(_ string) {
		gotOnlineEvent = true
	})

	// Kill the node and restart after a few seconds to simulate a disconnect to
	// the lnd node.
	_, err = node.Rpc.StopDaemon(context.Background(), &lnrpc.StopRequest{})
	if err != nil {
		t.Fatalf("Failed Stop(): %v", err)
	}
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	err = node.Run(true, true)
	if err != nil {
		t.Fatalf("Failed Run(): %v", err)
	}

	peer.Kill()
	err = peer.Run(true, true)
	if err != nil {
		t.Fatalf("Failed Run(): %v", err)
	}

	err = peer.Connect(node, true)
	if err != nil {
		t.Fatalf("Failed Connect(): %v", err)
	}

	err = testframework.WaitFor(func() bool {
		return gotOnlineEvent
	}, 50*time.Second)
	assert.NoError(t, err)
}
