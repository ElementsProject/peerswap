package lnd

import (
	"testing"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/routing"
)

func TestBuildDirectClaimPaymentRequestCLTVBoundary(t *testing.T) {
	invoice := &lnrpc.PayReq{
		Destination: "peer",
		CltvExpiry:  29,
	}
	channel := &lnrpc.Channel{
		RemotePubkey: "peer",
		ChanId:       42,
	}
	request, err := buildDirectClaimPaymentRequest("invoice", invoice, channel, 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.CltvLimit != 33 {
		t.Fatalf("unexpected CLTV limit: got %d, want 33", request.CltvLimit)
	}
	if len(request.OutgoingChanIds) != 1 || request.OutgoingChanIds[0] != 42 {
		t.Fatalf("unexpected outgoing channels: %v", request.OutgoingChanIds)
	}
	if request.MaxParts != 1 {
		t.Fatalf("unexpected max parts: got %d, want 1", request.MaxParts)
	}

	invoice.CltvExpiry = 30
	_, err = buildDirectClaimPaymentRequest("invoice", invoice, channel, 32)
	if err == nil || err.Error() != "invoice requires CLTV delta 33, maximum is 32" {
		t.Fatalf("unexpected boundary error: %v", err)
	}

	invoice.CltvExpiry = 503
	request, err = buildDirectClaimPaymentRequest("invoice", invoice, channel, 0)
	if err != nil {
		t.Fatalf("legacy request failed: %v", err)
	}
	wantLegacyLimit := int32(int64(503) + int64(routing.BlockPadding) + 1)
	if request.CltvLimit != wantLegacyLimit {
		t.Fatalf("legacy CLTV limit changed: got %d, want %d", request.CltvLimit, wantLegacyLimit)
	}
}
