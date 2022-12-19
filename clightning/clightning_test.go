package clightning

import (
	"sync"

	"github.com/elementsproject/glightning/glightning"
)

type DummyPayerWaiter struct {
	sync.Mutex

	sendPayPartsAndWaitCalled      int
	sendPayPartsAndWaitReturn      *glightning.SendPayFields
	sendPayPartsAndWaitErrorReturn *error

	totalPayed uint64
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
