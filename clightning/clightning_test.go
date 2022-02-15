package clightning

import (
	"errors"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func Test_MppPayments(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	paymentSize := uint64(5100000 * 1000)
	noErrorPayer := &DummyPayerWaiter{
		waitSendPayError: nil,
		waitSendPayRes: &glightning.SendPayFields{
			PaymentPreimage: "preimage",
		},
		sendPayError: nil,
	}
	preimage, err := MppPayment(noErrorPayer, noErrorPayer, "", "", &glightning.DecodedBolt11{
		MilliSatoshis: paymentSize,
	})
	assert.NoError(t, err)
	assert.Equal(t, "preimage", preimage)
	assert.Equal(t, 6, noErrorPayer.sendPayCall)
	assert.Equal(t, 6, noErrorPayer.waitSendCalls)
	assert.Equal(t, paymentSize, noErrorPayer.totalPayed)
}

func Test_MppPaymentsError1(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	paymentSize := uint64(5100000 * 1000)
	sendpayError := errors.New("sendpay error")
	noErrorPayer := &DummyPayerWaiter{
		waitSendPayError: nil,
		waitSendPayRes: &glightning.SendPayFields{
			PaymentPreimage: "preimage",
		},
		sendPayError: sendpayError,
	}
	preimage, err := MppPayment(noErrorPayer, noErrorPayer, "", "", &glightning.DecodedBolt11{
		MilliSatoshis: paymentSize,
	})
	assert.Error(t, err)
	assert.Equal(t, "", preimage)
	assert.Equal(t, 1, noErrorPayer.sendPayCall)
	assert.Equal(t, 0, noErrorPayer.waitSendCalls)
	assert.Equal(t, uint64(1000000 * 1000), noErrorPayer.totalPayed)
}

func Test_MppPaymentsError2(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	paymentSize := uint64(5100000 * 1000)

	waitSendError := errors.New("sendpay error")
	noErrorPayer := &DummyPayerWaiter{
		waitSendPayError: waitSendError,
		waitSendPayRes: &glightning.SendPayFields{
			PaymentPreimage: "preimage",
		},
		sendPayError: nil,
	}
	preimage, err := MppPayment(noErrorPayer, noErrorPayer, "", "", &glightning.DecodedBolt11{
		MilliSatoshis: paymentSize,
	})
	assert.Error(t, err)
	assert.Equal(t, "", preimage)
	assert.Equal(t, 6, noErrorPayer.sendPayCall)
	assert.Equal(t, 6, noErrorPayer.waitSendCalls)
	assert.Equal(t, paymentSize, noErrorPayer.totalPayed)
}

type DummyPayerWaiter struct {
	waitSendPayError error
	waitSendCalls    int
	waitSendPayRes   *glightning.SendPayFields

	sendPayError error
	sendPayCall  int

	totalPayed uint64

	sync.Mutex
}

func (d *DummyPayerWaiter) WaitSendPayPart(paymentHash string, timeout uint, partId uint64) (*glightning.SendPayFields, error) {
	time.Sleep(time.Second * time.Duration(rand.Intn(5)))
	d.Lock()
	defer d.Unlock()
	d.waitSendCalls++
	return d.waitSendPayRes, d.waitSendPayError
}

func (d *DummyPayerWaiter) SendPayChannel(payreq string, bolt11 *glightning.DecodedBolt11, amountMsat uint64, channel string, label string, partId uint64) (string, error) {
	d.Lock()
	defer d.Unlock()
	d.sendPayCall++
	d.totalPayed += amountMsat
	return "", d.sendPayError
}
