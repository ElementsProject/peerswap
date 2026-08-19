package clightning

import (
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
	invoice := &glightning.DecodedBolt11{
		Payee:              "peer",
		AmountMsat:         glightning.AmountFromMSat(1_000),
		MinFinalCltvExpiry: 31,
	}
	route, err := buildDirectClaimRoute(invoice, "1:2:3", 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(route) != 1 || route[0].Delay != 32 || route[0].ShortChannelId != "1x2x3" {
		t.Fatalf("unexpected direct route: %+v", route)
	}

	invoice.MinFinalCltvExpiry = 32
	_, err = buildDirectClaimRoute(invoice, "1:2:3", 32)
	if err == nil || err.Error() != "invoice requires CLTV delta 33, maximum is 32" {
		t.Fatalf("unexpected boundary error: %v", err)
	}

	invoice.MinFinalCltvExpiry = 503
	route, err = buildDirectClaimRoute(invoice, "1:2:3", 0)
	if err != nil || route[0].Delay != 504 {
		t.Fatalf("legacy route changed: route=%+v, err=%v", route, err)
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
