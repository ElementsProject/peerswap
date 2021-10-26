package onchain

import (
	"github.com/sputn1ck/peerswap/swap"
	"github.com/vulpemventures/go-elements/network"
	"testing"
)

func Test_ScriptAddress(t *testing.T) {
	liquidOnCain := NewLiquidOnChain(nil, nil, nil, &network.Testnet)
	swapParams := &swap.OpeningParams{
		TakerPubkeyHash:  "02752e1beeeeb6472959117a0aa5d172900680c033ddf86b1a8318311e2b10223f",
		MakerPubkeyHash:  "02c30ff537639962f493d326a77f1c6cb591ee3d21ca8d89194bb69cb288f497e8",
		ClaimPaymentHash: "b94f26d422d5ce3a1e65dd4abb398d0d369aefe8f71d112c5591aa45eea1e75c",
		Amount:           5000,
	}
	redeemScript, err := ParamsToTxScript(swapParams, 0)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := liquidOnCain.createOpeningAddress(redeemScript)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("addr %s", addr)
}
