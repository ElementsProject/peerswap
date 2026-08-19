package swap

import (
	"errors"
	"testing"
)

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %q, got nil", want)
	}
	if err.Error() != want {
		t.Fatalf("unexpected error: got %q, want %q", err, want)
	}
}

func assertEqual[T comparable](t *testing.T, want, got T) {
	t.Helper()
	if got != want {
		t.Fatalf("unexpected value: got %v, want %v", got, want)
	}
}

func TestTimelockPolicy(t *testing.T) {
	tests := []struct {
		name    string
		chain   string
		version uint8
		want    timelockPolicy
	}{
		{
			name:    "protocol 6 Bitcoin",
			chain:   btc_chain,
			version: legacyProtocolVersion,
			want: timelockPolicy{
				CSV:                  1008,
				PaymentWindow:        504,
				InvoiceFinalCLTV:     503,
				AllowNewClaimPayment: true,
			},
		},
		{
			name:    "protocol 7 Bitcoin",
			chain:   btc_chain,
			version: PEERSWAP_PROTOCOL_VERSION,
			want: timelockPolicy{
				CSV:                  1008,
				PaymentWindow:        504,
				InvoiceFinalCLTV:     503,
				AllowNewClaimPayment: true,
			},
		},
		{
			name:    "protocol 6 Liquid recovery",
			chain:   l_btc_chain,
			version: legacyProtocolVersion,
			want: timelockPolicy{
				CSV:                  60,
				PaymentWindow:        30,
				InvoiceFinalCLTV:     29,
				AllowNewClaimPayment: false,
			},
		},
		{
			name:    "protocol 7 Liquid",
			chain:   l_btc_chain,
			version: PEERSWAP_PROTOCOL_VERSION,
			want: timelockPolicy{
				CSV:                  10080,
				PaymentWindow:        60,
				InvoiceFinalCLTV:     29,
				MaxTotalCLTVDelta:    32,
				AllowNewClaimPayment: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := swapDataForPolicy(test.chain, test.version)
			policy, err := data.getTimelockPolicy()
			assertNoError(t, err)
			assertEqual(t, test.want, policy)
		})
	}
}

func TestTimelockPolicyRejectsUnknownVersion(t *testing.T) {
	data := swapDataForPolicy(l_btc_chain, 5)
	_, err := data.getTimelockPolicy()
	assertError(t, err, "unsupported protocol version: 5")
}

func TestPaymentWindowBoundary(t *testing.T) {
	data := swapDataForPolicy(l_btc_chain, PEERSWAP_PROTOCOL_VERSION)
	data.StartingBlockHeight = 100
	policy, err := data.getTimelockPolicy()
	assertNoError(t, err)

	assertError(t, checkPaymentWindow(data, 100, policy), "could not get starting block height of the swap")
	data.StartingBlockHeightSet = true
	assertNoError(t, checkPaymentWindow(data, 159, policy))
	assertError(
		t,
		checkPaymentWindow(data, 160, policy),
		"claim payment deadline exceeded: current height 160, deadline 160",
	)
}

func TestSetLiquidPaymentWindowAnchor(t *testing.T) {
	services := getSwapServices(t, make(chan PeerMessage, 1))
	data := swapDataForPolicy(l_btc_chain, PEERSWAP_PROTOCOL_VERSION)

	assertNoError(t, setLiquidPaymentWindowAnchor(services, data))
	assertEqual(t, uint32(1), data.StartingBlockHeight)
	assertEqual(t, true, data.StartingBlockHeightSet)
}

func TestValidateClaimInvoiceBoundaries(t *testing.T) {
	policy := timelockPolicy{InvoiceFinalCLTV: 29}

	assertNoError(t, validateClaimInvoice(100_000_000, 29, 100_000, policy))
	assertError(
		t,
		validateClaimInvoice(100_000_000, 30, 100_000, policy),
		"unsafe invoice cltv: 30, maximum: 29",
	)
	assertError(
		t,
		validateClaimInvoice(99_999_000, 29, 100_000, policy),
		"invoice amount does not equal swap amount, invoice amount: 99999000 msat, swap amount: 100000 sat",
	)
}

func TestValidateTotalCLTVDelta(t *testing.T) {
	assertNoError(t, ValidateTotalCLTVDelta(32, 32))
	assertNoError(t, ValidateTotalCLTVDelta(503, 0))
	assertError(
		t,
		ValidateTotalCLTVDelta(33, 32),
		"invoice requires CLTV delta 33, maximum is 32",
	)
}

func TestAwaitConfirmationRejectsUnsafeLiquidInvoiceCLTV(t *testing.T) {
	services := getSwapServices(t, make(chan PeerMessage, 1))
	lightning := services.lightning.(*dummyLightningClient)
	lightning.decodeAmount = 100_000 * 1000
	lightning.decodeFinalCLTV = 30

	data := swapDataForPolicy(l_btc_chain, PEERSWAP_PROTOCOL_VERSION)
	data.SwapInRequest.SwapId = NewSwapId()
	data.SwapInRequest.Amount = 100_000
	data.StartingBlockHeight = 1
	data.StartingBlockHeightSet = true
	data.OpeningTxBroadcasted = &OpeningTxBroadcastedMessage{Payreq: "unsafe"}

	event := (&AwaitTxConfirmationAction{}).Execute(services, data)
	assertEqual(t, Event_ActionFailed, event)
	assertError(t, data.LastErr, "unsafe invoice cltv: 30, maximum: 29")
}

func TestLegacyLiquidRecoveryDoesNotCreatePayment(t *testing.T) {
	services := getSwapServices(t, make(chan PeerMessage, 1))
	lightning := services.lightning.(*dummyLightningClient)
	lightning.recoverPaymentPreimage = "preimage"

	data := swapDataForPolicy(l_btc_chain, legacyProtocolVersion)
	data.SwapInRequest.SwapId = NewSwapId()
	data.StartingBlockHeight = 1
	data.OpeningTxBroadcasted = &OpeningTxBroadcastedMessage{Payreq: "legacy"}
	data.OpeningTxHex = "opening"

	event := (&ValidateTxAndPayClaimInvoiceAction{}).Execute(services, data)
	assertEqual(t, Event_ActionSucceeded, event)
	assertEqual(t, "preimage", data.ClaimPreimage)
	assertEqual(t, 1, lightning.recoverPaymentCalled)
	assertEqual(t, 0, lightning.rebalanceCalled)

	lightning.recoverPaymentError = errors.New("payment not found")
	data.ClaimPreimage = ""
	event = (&ValidateTxAndPayClaimInvoiceAction{}).Execute(services, data)
	assertEqual(t, Event_ActionFailed, event)
	assertError(t, data.LastErr, "recover legacy claim payment: payment not found")
}

func swapDataForPolicy(chain string, version uint8) *SwapData {
	request := &SwapInRequestMessage{ProtocolVersion: version}
	if chain == l_btc_chain {
		request.Asset = "asset"
	} else {
		request.Network = "regtest"
	}
	return &SwapData{
		SwapInRequest:   request,
		SwapInAgreement: &SwapInAgreementMessage{ProtocolVersion: version},
	}
}
