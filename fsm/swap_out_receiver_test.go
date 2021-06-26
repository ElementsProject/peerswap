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

	serviceMap := map[string]interface{}{
		"messenger": msg,
		"lightning": lc,
		"policy":    policy,
		"txwatcher": txWatcher,
		"node":      node,
	}

	swapFSM := newSwapOutReceiverFSM("", store, serviceMap)

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

	serviceMap := map[string]interface{}{
		"messenger": msg,
		"lightning": lc,
		"policy":    policy,
		"txwatcher": txWatcher,
		"node":      node,
	}

	swapFSM := newSwapOutReceiverFSM("", store, serviceMap)

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
	err = swapFSM.SendEvent(Event_SwapOutReceiver_OnAbortMsgReceived, nil)
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

	serviceMap := map[string]interface{}{
		"messenger": msg,
		"lightning": lc,
		"policy":    policy,
		"txwatcher": txWatcher,
		"node":      node,
	}

	swapFSM := newSwapOutReceiverFSM("", store, serviceMap)

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

	err = swapFSM.SendEvent(Event_SwapOutReceiver_OnCancelReceived, nil)
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
	msgChan := make(chan string)
	store := &dummyStore{dataMap: map[string]Data{}}
	messenger := &dummyMessenger{msgChan: msgChan}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}

	serviceMap := map[string]interface{}{
		"messenger": messenger,
		"lightning": lc,
		"policy":    policy,
		"txwatcher": txWatcher,
		"node":      node,
	}

	swapFSM := newSwapOutReceiverFSM("", store, serviceMap)

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
	assert.Equal(t, "cancel", msg)
	assert.Equal(t, State_SwapOutCanceled, swapFSM.Data.GetCurrentState())
}
