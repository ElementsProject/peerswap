package swap

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	_, err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swap.Id,
		asset:       "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPINREQUEST, msg.MessageType())
	assert.Equal(t, State_SwapInSender_AwaitAgreement, swap.Current)

	_, _ = swap.SendEvent(Event_SwapInSender_OnAgreementReceived, &SwapInAgreementMessage{
		SwapId:          swap.Id,
		TakerPubkeyHash: takerPubkeyHash,
	})
	msg = <-msgChan
	assert.Equal(t, MESSAGETYPE_TXOPENEDRESPONSE, msg.MessageType())
	assert.Equal(t, State_SwapInSender_AwaitClaimPayment, swap.Current)
	_, err = swap.SendEvent(Event_OnClaimInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedPreimage, swap.Current)

}
func Test_SwapInSenderCancel1(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "ab123"
	peer := "ba123"
	chanId := "baz"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInSenderFSM(swapServices)

	_, err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swap.Id,
		asset:       "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPINREQUEST, msg.MessageType())
	assert.Equal(t, State_SwapInSender_AwaitAgreement, swap.Current)
	_, err = swap.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapCanceled, swap.Current)
}
func Test_SwapInSenderCoopClose(t *testing.T) {

	swapAmount := uint64(100)
	initiator := "ab123"
	peer := "ba123"
	takerPubkeyHash := "taker"
	chanId := "baz"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInSenderFSM(swapServices)

	_, err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swap.Id,
		asset:       "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPINREQUEST, msg.MessageType())
	assert.Equal(t, State_SwapInSender_AwaitAgreement, swap.Current)

	_, _ = swap.SendEvent(Event_SwapInSender_OnAgreementReceived, &SwapInAgreementMessage{
		SwapId:          swap.Id,
		TakerPubkeyHash: takerPubkeyHash,
	})
	msg = <-msgChan
	assert.Equal(t, MESSAGETYPE_TXOPENEDRESPONSE, msg.MessageType())
	assert.Equal(t, State_SwapInSender_AwaitClaimPayment, swap.Current)
	_, err = swap.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedCoop, swap.Current)

}
func getSwapServices(msgChan chan PeerMessage) *SwapServices {
	store := &dummyStore{dataMap: map[string]*SwapStateMachine{}}
	messenger := &dummyMessenger{msgChan: msgChan}
	lc := &dummyLightningClient{preimage: "fee"}
	policy := &dummyPolicy{}
	chain := &dummyChain{}

	swapServices := NewSwapServices(
		store,
		lc,
		messenger,
		policy,
		true,
		chain,
		true,
		chain,
	)
	return swapServices
}
