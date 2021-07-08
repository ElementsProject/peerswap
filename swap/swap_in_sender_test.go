package swap

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_SwapInSenderValidSwap(t *testing.T) {

	swapAmount := uint64(100)
	initiator := "ab123"
	peer := "ba123"
	takerPubkeyHash := "taker"
	chanId := "baz"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInSenderFSM(swapServices)

	err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swap.Id,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPINREQUEST, msg.MessageType())
	assert.Equal(t, State_SwapInSender_SwapInRequestSent, swap.Current)

	err = swap.SendEvent(Event_SwapInSender_OnAgreementReceived, &SwapInAgreementResponse{
		SwapId:          swap.Id,
		TakerPubkeyHash: takerPubkeyHash,
	})
	msg = <-msgChan
	assert.Equal(t, MESSAGETYPE_TXOPENEDRESPONSE, msg.MessageType())
	assert.Equal(t, State_SwapInSender_TxMsgSent, swap.Current)
	err = swap.SendEvent(Event_SwapInSender_OnClaimInvPaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapInSender_ClaimInvPaid, swap.Current)
	err = swap.SendEvent(Event_SwapInSender_OnClaimTxPreimage, &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_PREIMAGE,
		ClaimTxId: "txid",
	})
	assert.Equal(t, State_SwapInSender_ClaimedPreimage, swap.Current)
}
func Test_SwapInSenderCancel1(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "ab123"
	peer := "ba123"
	chanId := "baz"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInSenderFSM(swapServices)

	err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swap.Id,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPINREQUEST, msg.MessageType())
	assert.Equal(t, State_SwapInSender_SwapInRequestSent, swap.Current)
	err = swap.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapCanceled, swap.Current)
}
func Test_SwapInSenderCancel2(t *testing.T) {

	swapAmount := uint64(100)
	initiator := "ab123"
	peer := "ba123"
	takerPubkeyHash := "taker"
	chanId := "baz"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInSenderFSM(swapServices)

	err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swap.Id,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPINREQUEST, msg.MessageType())
	assert.Equal(t, State_SwapInSender_SwapInRequestSent, swap.Current)

	err = swap.SendEvent(Event_SwapInSender_OnAgreementReceived, &SwapInAgreementResponse{
		SwapId:          swap.Id,
		TakerPubkeyHash: takerPubkeyHash,
	})
	msg = <-msgChan
	assert.Equal(t, MESSAGETYPE_TXOPENEDRESPONSE, msg.MessageType())
	assert.Equal(t, State_SwapInSender_TxMsgSent, swap.Current)
	err = swap.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_WaitCltv, swap.Current)
	err = swap.SendEvent(Event_SwapInSender_OnCltvPassed, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapInSender_ClaimedCltv, swap.Current)
}
func getSwapServices(msgChan chan PeerMessage) *SwapServices {
	store := &dummyStore{dataMap: map[string]*StateMachine{}}
	messenger := &dummyMessenger{msgChan: msgChan}
	lc := &dummyLightningClient{preimage: "fee"}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}
	wallet := &DummyWallet{}
	utils := &DummyUtility{}

	swapServices := NewSwapServices(
		store,
		node,
		lc,
		messenger,
		policy,
		txWatcher,
		wallet,
		utils,
	)
	return swapServices
}
