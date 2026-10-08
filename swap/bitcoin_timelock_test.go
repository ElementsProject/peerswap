package swap

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestBitcoinRejectsRefundOverlap(t *testing.T) {
	for _, tc := range []struct {
		name   string
		height uint32
		cltv   int64
	}{
		{"reported delayed notification", 800_503, 504},
		{"honest invoice at unsafe height", 800_503, 503},
		{"excessive invoice near confirmation", 800_003, 504},
		{"negative invoice cltv", 800_003, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			services, data, w := bitcoinPaymentFixture(t, tc.height, tc.cltv)
			assertEqual(t, Event_ActionFailed, (&AwaitTxConfirmationAction{}).Execute(services, data))
			if err := data.LastErr; err == nil {
				t.Fatal("expected unsafe invoice or expired window to be rejected")
			}
			assertEqual(t, 0, w.watches)
		})
	}
}

func TestBitcoinPolicyBoundsSenderDeadline(t *testing.T) {
	for _, version := range []uint8{legacyProtocolVersion, PEERSWAP_PROTOCOL_VERSION} {
		policy, err := swapDataForPolicy(btc_chain, version).getTimelockPolicy()
		assertNoError(t, err)
		assertEqual(t, uint32(506), policy.MaxTotalCLTVDelta)
		// The anchor precedes disclosure of the taker key, hence is no later
		// than confirmation. Even the last accepted payment must end before CSV.
		lastDeadline := uint64(policy.PaymentWindow-1) + policy.InvoiceFinalCLTV + 3
		assertEqual(t, uint32(60), policy.PaymentWindow)
		assertEqual(t, uint64(443), uint64(policy.CSV)-lastDeadline)
	}
}

func TestBitcoinPaymentWindowBoundaries(t *testing.T) {
	services, data, w := bitcoinPaymentFixture(t, 800_059, 503)
	policy, err := data.getTimelockPolicy()
	assertNoError(t, err)
	assertNoError(t, checkPaymentWindow(data, 800_059, policy))
	if err := checkPaymentWindow(data, 800_060, policy); err == nil {
		t.Fatal("expected payment at the exclusive deadline to be rejected")
	}
	if err := checkPaymentWindow(data, 799_999, policy); err == nil {
		t.Fatal("expected height preceding the anchor to be rejected")
	}
	assertEqual(t, NoOp, (&AwaitTxConfirmationAction{}).Execute(services, data))
	assertEqual(t, 1, w.watches)

	// A block arrives after the confirmation callback, before the pay tick.
	w.height = 800_060
	data.OpeningTxHex = bitcoinTestOpeningHex
	assertEqual(t, Event_ActionFailed, (&ValidateTxAndPayClaimInvoiceAction{}).Execute(services, data))
	if data.LastErr == nil || !strings.Contains(data.LastErr.Error(), "deadline exceeded") {
		t.Fatalf("expected payment deadline error, got %v", data.LastErr)
	}
	assertEqual(t, 0, services.lightning.(*dummyLightningClient).rebalanceCalled)

	// Deadline arithmetic must not wrap into the past.
	data.StartingBlockHeight = math.MaxUint32 - 20
	assertNoError(t, checkPaymentWindow(data, math.MaxUint32, policy))
}

func TestBitcoinSafePaymentUsesTotalCLTVLimit(t *testing.T) {
	services, data, _ := bitcoinPaymentFixture(t, 800_059, 503)
	data.OpeningTxHex = bitcoinTestOpeningHex
	assertEqual(t, Event_ActionSucceeded, (&ValidateTxAndPayClaimInvoiceAction{}).Execute(services, data))
	lc := services.lightning.(*dummyLightningClient)
	assertEqual(t, 1, lc.rebalanceCalled)
	assertEqual(t, uint32(506), lc.rebalanceMaxTotalCLTV)
	if data.ClaimPreimage == "" {
		t.Fatal("successful payment has no claim preimage")
	}
}

func TestBitcoinAnchorCannotBeReplacedAfterKeyDisclosure(t *testing.T) {
	services, data, _ := bitcoinPaymentFixture(t, 800_010, 503)
	assertEqual(t, NoOp, (&SetStartingBlockHeightAction{}).Execute(services, data))
	assertEqual(t, uint32(800_000), data.StartingBlockHeight)

	// This represents a pre-upgrade record: height exists, trusted-anchor flag does not.
	data.StartingBlockHeightSet = false
	// #nosec G117 -- This synthetic test fixture has no real private key or preimage.
	encoded, err := json.Marshal(data)
	assertNoError(t, err)
	if strings.Contains(string(encoded), "opening_block_height_set") {
		t.Fatal("legacy record unexpectedly contains trusted-anchor flag")
	}
	var old SwapData
	assertNoError(t, json.Unmarshal(encoded, &old))
	assertEqual(t, Event_ActionFailed, (&SetStartingBlockHeightAction{}).Execute(services, &old))
	assertEqual(t, uint32(800_000), old.StartingBlockHeight)
	assertEqual(t, false, old.StartingBlockHeightSet)
	old.OpeningTxHex = bitcoinTestOpeningHex
	assertEqual(t, Event_ActionFailed, (&ValidateTxAndPayClaimInvoiceAction{}).Execute(services, &old))
	assertEqual(t, 0, services.lightning.(*dummyLightningClient).rebalanceCalled)
}

// Move the tip after a failed attempt to exercise the check on the next tick.
type advancingPaymentClient struct {
	LightningClient
	watcher  *paymentWindowWatcher
	attempts int
}

func (c *advancingPaymentClient) RebalancePayment(string, string, uint32) (string, error) {
	c.attempts++
	c.watcher.height++
	return "", errors.New("payment failed")
}

func TestBitcoinRetryCannotCrossPaymentDeadline(t *testing.T) {
	services, data, watcher := bitcoinPaymentFixture(t, 800_059, 503)
	data.OpeningTxHex = bitcoinTestOpeningHex
	client := &advancingPaymentClient{LightningClient: services.lightning, watcher: watcher}
	services.lightning = client
	assertEqual(t, Event_ActionFailed, (&ValidateTxAndPayClaimInvoiceAction{}).Execute(services, data))
	assertEqual(t, 1, client.attempts)
	if data.LastErr == nil || !strings.Contains(data.LastErr.Error(), "deadline exceeded") {
		t.Fatalf("expected deadline error after the failed attempt, got %v", data.LastErr)
	}
}
