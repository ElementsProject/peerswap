package clightning

import (
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/elementsproject/glightning/glightning"
	"github.com/stretchr/testify/assert"
)

func Test_MppPayments(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	paymentSize := uint64(5100000 * 1000)
	noErrorPayer := &DummyPayerWaiter{
		sendPayPartsAndWaitErrorReturn: nil,
		sendPayPartsAndWaitReturn: &glightning.SendPayFields{
			PaymentPreimage: "preimage",
		},
	}
	preimage, err := MppPayment(noErrorPayer, "", "", &glightning.DecodedBolt11{
		MilliSatoshis: paymentSize,
	})
	assert.NoError(t, err)
	assert.Equal(t, "preimage", preimage)
	assert.Equal(t, 10, noErrorPayer.sendPayPartsAndWaitCalled)
	assert.Equal(t, paymentSize, noErrorPayer.totalPayed)
}

func Test_MppPaymentsError1(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	paymentSize := uint64(5100000 * 1000)
	sendpayError := errors.New("sendpay error")
	noErrorPayer := &DummyPayerWaiter{
		sendPayPartsAndWaitErrorReturn: &sendpayError,
		sendPayPartsAndWaitReturn: &glightning.SendPayFields{
			PaymentPreimage: "preimage",
		},
	}
	preimage, err := MppPayment(noErrorPayer, "", "", &glightning.DecodedBolt11{
		MilliSatoshis: paymentSize,
	})
	assert.Error(t, err)
	assert.Equal(t, "", preimage)
	assert.Equal(t, 10, noErrorPayer.sendPayPartsAndWaitCalled)
	assert.EqualValues(t, uint64(5100000*1000), noErrorPayer.totalPayed)
}

func Test_MppPaymentsError2(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	paymentSize := uint64(5100000 * 1000)

	waitSendError := errors.New("sendpay error")
	noErrorPayer := &DummyPayerWaiter{
		sendPayPartsAndWaitErrorReturn: &waitSendError,
		sendPayPartsAndWaitReturn: &glightning.SendPayFields{
			PaymentPreimage: "preimage",
		},
	}
	preimage, err := MppPayment(noErrorPayer, "", "", &glightning.DecodedBolt11{
		MilliSatoshis: paymentSize,
	})
	assert.Error(t, err)
	assert.Equal(t, "", preimage)
	assert.Equal(t, 10, noErrorPayer.sendPayPartsAndWaitCalled)
	assert.Equal(t, paymentSize, noErrorPayer.totalPayed)
}

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
