package swap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_SwapOutReceiverValidSwap(t *testing.T) {
	swapAmount := uint64(100)
	swapId := "swapid"
	takerpubkeyhash := "abcdef"
	peer := "bar"
	chanId := "baz"

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swapFSM := newSwapOutReceiverFSM(swapId, swapServices)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		takerPubkeyHash: takerpubkeyhash,
		asset:           "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.MakerPubkeyHash)

	_, err = swapFSM.SendEvent(Event_OnFeeInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_AwaitClaimInvoicePayment, swapFSM.Data.GetCurrentState())
	_, err = swapFSM.SendEvent(Event_OnClaimInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedPreimage, swapFSM.Data.GetCurrentState())

}
func Test_SwapOutReceiverClaimCoop(t *testing.T) {
	swapAmount := uint64(100)
	swapId := "swapid"
	initiator := "foo"
	peer := "bar"
	chanId := "baz"

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)

	swapFSM := newSwapOutReceiverFSM(swapId, swapServices)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		takerPubkeyHash: initiator,
		asset:           "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.MakerPubkeyHash)

	_, err = swapFSM.SendEvent(Event_OnFeeInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_AwaitClaimInvoicePayment, swapFSM.Data.GetCurrentState())
	_, err = swapFSM.SendEvent(Event_OnCoopCloseReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedCoop, swapFSM.Data.GetCurrentState())

}

func Test_SwapOutReceiverCancelReceived(t *testing.T) {
	swapAmount := uint64(100)
	swapId := "swapid"
	initiator := "foo"
	peer := "bar"
	chanId := "baz"

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)

	swapFSM := newSwapOutReceiverFSM(swapId, swapServices)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		takerPubkeyHash: initiator,
		asset:           "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.MakerPubkeyHash)

	_, err = swapFSM.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())
}

func Test_SwapOutReceiverCancelInternal(t *testing.T) {
	swapAmount := uint64(100)
	swapId := "swapid"
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeePreimage := "err"

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swapServices.lightning.(*dummyLightningClient).preimage = FeePreimage
	swapFSM := newSwapOutReceiverFSM(swapId, swapServices)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		takerPubkeyHash: initiator,
		asset:           "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.MakerPubkeyHash)
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())
}
