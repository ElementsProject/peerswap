package lwk_test

import (
	"testing"

	"github.com/elementsproject/peerswap/lwk"
	"github.com/vulpemventures/go-elements/network"
)

func Test_confBuilder_DefaultConf(t *testing.T) {
	t.Parallel()
	b, err := lwk.NewConfBuilder(lwk.NetworkTestnet).DefaultConf()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c, err := b.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.GetChain() != &network.Testnet {
		t.Fatalf("unexpected chain: %v", c.GetChain())
	}
	if c.GetElectrumEndpoint() != "blockstream.info:465" {
		t.Fatalf("unexpected electrum endpoint: %v", c.GetElectrumEndpoint())
	}
	if c.GetLWKEndpoint() != "http://localhost:32111" {
		t.Fatalf("unexpected lwk endpoint: %v", c.GetLWKEndpoint())
	}
	if c.GetLiquidSwaps() != true {
		t.Fatalf("unexpected liquid swaps: %v", c.GetLiquidSwaps())
	}
	if c.GetNetwork() != lwk.NetworkTestnet.String() {
		t.Fatalf("unexpected network: %v", c.GetNetwork())
	}
	if c.GetSignerName() != "defaultPeerswapSigner" {
		t.Fatalf("unexpected signer name: %v", c.GetSignerName())
	}
	if c.GetWalletName() != "defaultPeerswapWallet" {
		t.Fatalf("unexpected wallet name: %v", c.GetWalletName())
	}
}

func Test_confBuilder_SetConfs(t *testing.T) {
	t.Parallel()
	t.Run("OK if it called with valid arguments", func(t *testing.T) {
		b, err := lwk.NewConfBuilder(lwk.NetworkTestnet).DefaultConf()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		c, err := b.SetSignerName("testSigner").SetWalletName("testSigner").Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.GetChain() != &network.Testnet {
			t.Fatalf("unexpected chain: %v", c.GetChain())
		}
		if c.GetElectrumEndpoint() != "blockstream.info:465" {
			t.Fatalf("unexpected electrum endpoint: %v", c.GetElectrumEndpoint())
		}
		if c.GetLWKEndpoint() != "http://localhost:32111" {
			t.Fatalf("unexpected lwk endpoint: %v", c.GetLWKEndpoint())
		}
		if c.GetLiquidSwaps() != true {
			t.Fatalf("unexpected liquid swaps: %v", c.GetLiquidSwaps())
		}
		if c.GetNetwork() != lwk.NetworkTestnet.String() {
			t.Fatalf("unexpected network: %v", c.GetNetwork())
		}
		if c.GetSignerName() != "testSigner" {
			t.Fatalf("unexpected signer name: %v", c.GetSignerName())
		}
		if c.GetWalletName() != "testSigner" {
			t.Fatalf("unexpected wallet name: %v", c.GetWalletName())
		}
	})
	t.Run("Error if it called with empty signer name", func(t *testing.T) {
		b, err := lwk.NewConfBuilder(lwk.NetworkTestnet).DefaultConf()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = b.SetSignerName("").Build()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})
}
