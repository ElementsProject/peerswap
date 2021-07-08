package swap

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_SwapInReceiverValid(t *testing.T) {

	swapId := "swapid"
	swapAmount := uint64(100)
	//initiator := "ab123"
	peer := "ba123"
	makerPubkeyHash := "maker"
	chanId := "baz"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInReceiverFSM(swapId, swapServices)

	err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &CreateSwapFromRequestContext{
		amount:    swapAmount,
		peer:      peer,
		channelId: chanId,
		swapId:    swapId,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPINAGREEMENT, msg.MessageType())
	assert.Equal(t, State_SwapInReceiver_AgreementSent, swap.Current)

	err = swap.SendEvent(Event_SwapInReceiver_OnTxBroadcasted, &TxOpenedResponse{
		SwapId:          swap.Id,
		MakerPubkeyHash: makerPubkeyHash,
		Invoice:         "invoice",
		TxId:            "txid",
		TxHex:           "txhext",
		Cltv:            0,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapInReceiver_WaitForConfirmations, swap.Current)
	err = swap.SendEvent(Event_SwapInReceiver_OnOpeningTxConfirmed, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg = <-msgChan
	assert.Equal(t, MESSAGETYPE_CLAIMED, msg.MessageType())
	assert.Equal(t, State_SwapInReceiver_ClaimedPreimage, swap.Current)

}
func Test_SwapInReceiverCancel1(t *testing.T) {

	swapId := "swapid"
	swapAmount := uint64(100)
	//initiator := "ab123"
	peer := "ba123"
	chanId := "baz"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInReceiverFSM(swapId, swapServices)

	err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &CreateSwapFromRequestContext{
		amount:    swapAmount,
		peer:      peer,
		channelId: chanId,
		swapId:    swapId,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPINAGREEMENT, msg.MessageType())
	assert.Equal(t, State_SwapInReceiver_AgreementSent, swap.Current)

	err = swap.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapCanceled, swap.Current)

}
func Test_SwapInReceiverCancel2(t *testing.T) {

	swapId := "swapid"
	swapAmount := uint64(100)
	//initiator := "ab123"
	peer := "ba123"
	makerPubkeyHash := "maker"
	chanId := "baz"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInReceiverFSM(swapId, swapServices)

	err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &CreateSwapFromRequestContext{
		amount:    swapAmount,
		peer:      peer,
		channelId: chanId,
		swapId:    swapId,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPINAGREEMENT, msg.MessageType())
	assert.Equal(t, State_SwapInReceiver_AgreementSent, swap.Current)

	err = swap.SendEvent(Event_SwapInReceiver_OnTxBroadcasted, &TxOpenedResponse{
		SwapId:          swap.Id,
		MakerPubkeyHash: makerPubkeyHash,
		Invoice:         "invoice",
		TxId:            "txid",
		TxHex:           "txhext",
		Cltv:            0,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapInReceiver_WaitForConfirmations, swap.Current)
	err = swap.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapCanceled, swap.Current)

}
