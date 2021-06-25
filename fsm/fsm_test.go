package fsm

import (
	"github.com/stretchr/testify/assert"
	"log"
	"testing"
)

func Test_Fsm(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeeInvoice := "feeinv"
	FeePreimage := "preimage"

	store := &dummyStore{dataMap: map[string]Data{}}
	msg := &dummyMessenger{}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}

	serviceMap := map[string]interface{}{
		"messenger": msg,
		"lightning": lc,
		"policy":    policy,
	}

	swapFSM := newSwapOutSenderFSM("", store, serviceMap)

	err := swapFSM.SendEvent(CreateSwapOut, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, initiator, swapFSM.Data.(*Swap).InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).TakerPubkeyHash)

	swap := &Swap{Id: "whatevs", FSMState: SwapOutRequestSent}
	store = &dummyStore{dataMap: map[string]Data{swap.Id: swap}}
	swapFsm2 := newSwapOutSenderFSM("whatevs", store, serviceMap)
	err = swapFsm2.SendEvent(OnFeeInvReceived, &FeeRequestContext{
		FeeInvoice: FeeInvoice,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, FeeInvoicePaid, swapFsm2.Current)
	assert.Equal(t, FeePreimage, swapFsm2.Data.(*Swap).FeePreimage)
}

type dummyStore struct {
	dataMap map[string]Data
}

func (d *dummyStore) UpdateData(id string, data Data) error {
	d.dataMap[id] = data
	return nil
}

func (d *dummyStore) GetData(id string) (Data, error) {
	if _, ok := d.dataMap[id]; !ok {
		return nil, ErrDataNotAvailable
	}
	return d.dataMap[id], nil
}

type dummyMessenger struct {
}

func (d *dummyMessenger) SendMessage(peerId string, hexMsg string) error {
	log.Printf("Dummy sending message %s to %s", hexMsg, peerId)
	return nil
}

type dummyLightningClient struct {
	preimage string
}

func (d *dummyLightningClient) DecodeInvoice(payreq string) (peerId string, amount uint64, err error) {
	return "foo", uint64(100), nil
}

func (d *dummyLightningClient) PayInvoice(payreq string) (preImage string, err error) {
	return d.preimage, nil
}

type dummyPolicy struct {
}

func (d *dummyPolicy) ShouldPayFee(feeAmount uint64, peerId, channelId string) bool {
	return true
}
