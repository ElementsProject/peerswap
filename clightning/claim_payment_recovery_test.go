// Package clightning integrates PeerSwap with Core Lightning.
package clightning

import (
	"errors"
	"testing"

	"github.com/elementsproject/glightning/glightning"
	"github.com/elementsproject/peerswap/swap"
)

const claimRecoveryTestPreimage = "preimage"

type recoveryTracker struct {
	lists   [][]glightning.SendPayFields
	listErr error
}

func (*recoveryTracker) DecodeBolt11(string) (*glightning.DecodedBolt11, error) {
	return &glightning.DecodedBolt11{PaymentHash: "hash"}, nil
}

func (r *recoveryTracker) ListSendPaysByHash(string) ([]glightning.SendPayFields, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	payments := r.lists[0]
	r.lists = r.lists[1:]
	return payments, nil
}

func TestRecoverClaimPayment(t *testing.T) {
	pending := glightning.SendPayFields{Status: "pending"}
	failed := glightning.SendPayFields{Status: claimPaymentFailed}
	paid := glightning.SendPayFields{Status: claimPaymentComplete, PaymentPreimage: claimRecoveryTestPreimage}
	for _, test := range []struct {
		name     string
		tracker  recoveryTracker
		wantErr  error
		wantPaid bool
	}{
		{name: "missing", tracker: recoveryTracker{lists: [][]glightning.SendPayFields{nil}}, wantErr: swap.ErrClaimPaymentNotFound},
		{name: "all failed", tracker: recoveryTracker{lists: [][]glightning.SendPayFields{{failed, failed}}}, wantErr: swap.ErrClaimPaymentFailed},
		{name: "completed retry", tracker: recoveryTracker{lists: [][]glightning.SendPayFields{{failed, paid}}}, wantPaid: true},
		{name: "pending after failed attempt", tracker: recoveryTracker{lists: [][]glightning.SendPayFields{{failed, pending}}}},
		{name: "pending snapshot", tracker: recoveryTracker{lists: [][]glightning.SendPayFields{{pending}}}},
		{name: "status unavailable", tracker: recoveryTracker{listErr: errors.New("disconnected")}},
		{name: "missing preimage", tracker: recoveryTracker{lists: [][]glightning.SendPayFields{{{Status: claimPaymentComplete}}}}},
		{name: "unknown state", tracker: recoveryTracker{lists: [][]glightning.SendPayFields{{failed, {Status: "unknown"}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			preimage, err := recoverClaimPayment(&test.tracker, "invoice")
			assertRecoveryOutcome(t, preimage, err, test.wantPaid, test.wantErr)
		})
	}
}

func TestRecoverClaimPaymentRechecksPendingSnapshot(t *testing.T) {
	for _, outcome := range []glightning.SendPayFields{
		{Status: claimPaymentComplete, PaymentPreimage: claimRecoveryTestPreimage},
		{Status: claimPaymentFailed},
	} {
		t.Run(outcome.Status, func(t *testing.T) {
			tracker := &recoveryTracker{lists: [][]glightning.SendPayFields{
				{{Status: "pending"}}, {outcome},
			}}
			preimage, err := recoverClaimPayment(tracker, "invoice")
			assertRecoveryOutcome(t, preimage, err, false, nil)
			preimage, err = recoverClaimPayment(tracker, "invoice")
			var wantErr error
			if outcome.Status == "failed" {
				wantErr = swap.ErrClaimPaymentFailed
			}
			assertRecoveryOutcome(t, preimage, err, outcome.Status == claimPaymentComplete, wantErr)
		})
	}
}

func assertRecoveryOutcome(t *testing.T, preimage string, err error, wantPaid bool, wantErr error) {
	t.Helper()
	switch {
	case wantPaid:
		if err != nil || preimage != claimRecoveryTestPreimage {
			t.Fatalf("expected recovered preimage, got %q, %v", preimage, err)
		}
	case wantErr != nil:
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected %v, got %v", wantErr, err)
		}
	default:
		if err == nil || errors.Is(err, swap.ErrClaimPaymentNotFound) || errors.Is(err, swap.ErrClaimPaymentFailed) {
			t.Fatalf("unresolved payment must not authorize key release: %v", err)
		}
	}
}
