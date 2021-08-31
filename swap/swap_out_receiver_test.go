package swap

import (
	"github.com/stretchr/testify/assert"
	"testing"
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

	err := swapFSM.SendEvent(Event_SwapOutReceiver_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		takerPubkeyHash: takerpubkeyhash,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.MakerPubkeyHash)

	err = swapFSM.SendEvent(Event_SwapOutReceiver_OnFeeInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_TxMsgSent, swapFSM.Data.GetCurrentState())
	err = swapFSM.SendEvent(Event_OnClaimInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_ClaimInvoicePaid, swapFSM.Data.GetCurrentState())

	err = swapFSM.SendEvent(Event_OnClaimedPreimage, &ClaimedMessage{
		ClaimTxId: "txId",
	})
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, State_ClaimedPreimage, swapFSM.Data.GetCurrentState())
}
func Test_SwapOutReceiverAbortCltv(t *testing.T) {
	swapAmount := uint64(100)
	swapId := "swapid"
	initiator := "foo"
	peer := "bar"
	chanId := "baz"

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)

	swapFSM := newSwapOutReceiverFSM(swapId, swapServices)

	err := swapFSM.SendEvent(Event_SwapOutReceiver_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		takerPubkeyHash: initiator,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.MakerPubkeyHash)

	err = swapFSM.SendEvent(Event_SwapOutReceiver_OnFeeInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_TxMsgSent, swapFSM.Data.GetCurrentState())
	err = swapFSM.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_SwapAborted, swapFSM.Data.GetCurrentState())

	err = swapFSM.SendEvent(Event_OnCltvPassed, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, State_ClaimedCltv, swapFSM.Data.GetCurrentState())
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

	err := swapFSM.SendEvent(Event_SwapOutReceiver_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		takerPubkeyHash: initiator,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.MakerPubkeyHash)

	err = swapFSM.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOut_Canceled, swapFSM.Data.GetCurrentState())
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

	err := swapFSM.SendEvent(Event_SwapOutReceiver_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		takerPubkeyHash: initiator,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.MakerPubkeyHash)
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapOut_Canceled, swapFSM.Data.GetCurrentState())
}
