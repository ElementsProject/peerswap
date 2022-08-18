package lnd

import (
	"context"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/testframework"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/stretchr/testify/assert"
)

// TestPeerListener tests the basic functionality of the peer listener. The peer
// disconnects and reconnects. We expect to have a handler called for both
// events.
func TestPeerListener(t *testing.T) {
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
	}, 20*time.Second)
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
	}, 20*time.Second)
	assert.NoError(t, err)
}
