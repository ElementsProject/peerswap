package testframework

import (
	"testing"
)

func TestLnd(t *testing.T) {
	testDir := t.TempDir()

	// Misc setup
	// assertions := &AssertionCounter{}

	// Setup nodes (1 bitcoind, 2 lightningd)
	bitcoind, err := NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	lnd, err := NewLndNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatalf("could not create lnd %v", err)
	}
	t.Cleanup(lnd.Kill)

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	lnd.Run(true, true)
}
