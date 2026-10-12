package clightning

import (
	"math"
	"sync"
	"testing"

	"github.com/elementsproject/glightning/glightning"
)

type DummyPayerWaiter struct {
	sync.Mutex

	sendPayPartsAndWaitCalled      int
	sendPayPartsAndWaitReturn      *glightning.SendPayFields
	sendPayPartsAndWaitErrorReturn *error

	totalPayed uint64
}

func TestBuildDirectClaimRouteCLTVBoundary(t *testing.T) {
	const channelID = "1x2x3"
	invoice := &glightning.DecodedBolt11{
		Payee:              "peer",
		AmountMsat:         glightning.AmountFromMSat(1_000),
		MinFinalCltvExpiry: 29,
	}
	route, err := buildDirectClaimRoute(invoice, "1:2:3", 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(route) != 1 || route[0].Delay != 30 || route[0].ShortChannelId != channelID {
		t.Fatalf("unexpected direct route: %+v", route)
	}

	invoice.MinFinalCltvExpiry = 30
	_, err = buildDirectClaimRoute(invoice, "1:2:3", 32)
	if err == nil || err.Error() != "invoice requires CLTV delta 33, maximum is 32" {
		t.Fatalf("unexpected boundary error: %v", err)
	}

	invoice.MinFinalCltvExpiry = 503
	route, err = buildDirectClaimRoute(invoice, "1:2:3", 506)
	if err != nil || route[0].Delay != 504 {
		t.Fatalf("Bitcoin route should fit the sender deadline: route=%+v, err=%v", route, err)
	}
	invoice.MinFinalCltvExpiry = 504
	_, err = buildDirectClaimRoute(invoice, "1:2:3", 506)
	if err == nil || err.Error() != "invoice requires CLTV delta 507, maximum is 506" {
		t.Fatalf("unexpected Bitcoin boundary error: %v", err)
	}

	invoice.MinFinalCltvExpiry = 503
	route, err = buildDirectClaimRoute(invoice, "1:2:3", 0)
	if err != nil || route[0].Delay != 504 {
		t.Fatalf("legacy route changed: route=%+v, err=%v", route, err)
	}
}

func TestBuildDirectClaimRouteRejectsInvalidCLTV(t *testing.T) {
	for _, expiry := range []int{-1, math.MaxInt} {
		invoice := &glightning.DecodedBolt11{MinFinalCltvExpiry: expiry}
		if _, err := buildDirectClaimRoute(invoice, "1:2:3", 506); err == nil {
			t.Fatalf("invalid CLTV delta %d was accepted", expiry)
		}
	}
}

func (d *DummyPayerWaiter) SendPayPartAndWait(paymentRequest string, bolt11 *glightning.DecodedBolt11, amountMsat uint64, channel string, label string, partId uint64) (*glightning.SendPayFields, error) {
	d.Lock()
	defer d.Unlock()
	d.sendPayPartsAndWaitCalled++
	d.totalPayed += amountMsat

	if d.sendPayPartsAndWaitErrorReturn != nil {
		return nil, *d.sendPayPartsAndWaitErrorReturn
	}

	return d.sendPayPartsAndWaitReturn, nil
}
