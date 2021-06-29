package fsm

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_SwapOutReceiverValidSwap(t *testing.T) {
	swapAmount := uint64(100)
	swapId := "swapid"
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeePreimage := "preimage"

	store := &dummyStore{dataMap: map[string]Data{}}
	msg := &dummyMessenger{}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}

	swapServices := &SwapServices{
		messenger: msg,
		swapStore: store,
		node:      node,
		lightning: lc,
		policy:    policy,
		txWatcher: txWatcher,
	}
	swapFSM := newSwapOutReceiverFSM("", store, swapServices)

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
	assert.Equal(t, peer, swapFSM.Data.(*Swap).InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).MakerPubkeyHash)

	err = swapFSM.SendEvent(Event_SwapOutReceiver_OnFeeInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_OpeningTxBroadcasted, swapFSM.Data.GetCurrentState())
	err = swapFSM.SendEvent(Event_SwapOutReceiver_OnClaimInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_ClaimInvoicePaid, swapFSM.Data.GetCurrentState())

	err = swapFSM.SendEvent(Event_SwapOutReceiver_OnClaimMsgReceived, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, State_SwapOutReceiver_ClaimedPreimage, swapFSM.Data.GetCurrentState())
}
func Test_SwapOutReceiverAbortCltv(t *testing.T) {
	swapAmount := uint64(100)
	swapId := "swapid"
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeePreimage := "preimage"

	store := &dummyStore{dataMap: map[string]Data{}}
	msg := &dummyMessenger{}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}

	swapServices := &SwapServices{
		messenger: msg,
		swapStore: store,
		node:      node,
		lightning: lc,
		policy:    policy,
		txWatcher: txWatcher,
	}

	swapFSM := newSwapOutReceiverFSM("", store, swapServices)

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
	assert.Equal(t, peer, swapFSM.Data.(*Swap).InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).MakerPubkeyHash)

	err = swapFSM.SendEvent(Event_SwapOutReceiver_OnFeeInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_OpeningTxBroadcasted, swapFSM.Data.GetCurrentState())
	err = swapFSM.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_SwapAborted, swapFSM.Data.GetCurrentState())

	err = swapFSM.SendEvent(Event_SwapOutReceiver_OnCltvPassed, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, State_SwapOutReceiver_ClaimedCltv, swapFSM.Data.GetCurrentState())
}

func Test_SwapOutReceiverCancelReceived(t *testing.T) {
	swapAmount := uint64(100)
	swapId := "swapid"
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeePreimage := "preimage"

	store := &dummyStore{dataMap: map[string]Data{}}
	msg := &dummyMessenger{}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}

	swapServices := &SwapServices{
		messenger: msg,
		swapStore: store,
		node:      node,
		lightning: lc,
		policy:    policy,
		txWatcher: txWatcher,
	}

	swapFSM := newSwapOutReceiverFSM("", store, swapServices)

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
	assert.Equal(t, peer, swapFSM.Data.(*Swap).InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).MakerPubkeyHash)

	err = swapFSM.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutCanceled, swapFSM.Data.GetCurrentState())
}

func Test_SwapOutReceiverCancelInternal(t *testing.T) {
	swapAmount := uint64(100)
	swapId := "swapid"
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeePreimage := "err"
	msgChan := make(chan PeerMessage)
	store := &dummyStore{dataMap: map[string]Data{}}
	messenger := &dummyMessenger{msgChan: msgChan}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}

	swapServices := &SwapServices{
		messenger: messenger,
		swapStore: store,
		node:      node,
		lightning: lc,
		policy:    policy,
		txWatcher: txWatcher,
	}

	swapFSM := newSwapOutReceiverFSM("", store, swapServices)

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
	assert.Equal(t, peer, swapFSM.Data.(*Swap).InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).TakerPubkeyHash)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).MakerPubkeyHash)
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapOutCanceled, swapFSM.Data.GetCurrentState())
}
