package swap

import (
	"testing"

	"github.com/sputn1ck/peerswap/messages"
	"github.com/stretchr/testify/assert"
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

	_, err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		asset:           "btc",
		protocolversion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPINAGREEMENT, msg.MessageType())
	assert.Equal(t, State_SwapInReceiver_AwaitTxBroadcastedMessage, swap.Current)

	_, err = swap.SendEvent(Event_OnTxOpenedMessage, &TxOpenedMessage{
		SwapId:          swap.Id,
		MakerPubkeyHash: makerPubkeyHash,
		Invoice:         "invoice",
		TxHex:           "txhex",
	})
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, State_SwapInReceiver_AwaitTxConfirmation, swap.Current)
	_, err = swap.SendEvent(Event_OnTxConfirmed, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, State_ClaimedPreimage, swap.Current)

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

	_, err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		asset:           "btc",
		protocolversion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPINAGREEMENT, msg.MessageType())
	assert.Equal(t, State_SwapInReceiver_AwaitTxBroadcastedMessage, swap.Current)

	_, err = swap.SendEvent(Event_OnCancelReceived, nil)
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

	_, err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &CreateSwapFromRequestContext{
		amount:          swapAmount,
		peer:            peer,
		channelId:       chanId,
		swapId:          swapId,
		asset:           "btc",
		protocolversion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPINAGREEMENT, msg.MessageType())
	assert.Equal(t, State_SwapInReceiver_AwaitTxBroadcastedMessage, swap.Current)

	_, err = swap.SendEvent(Event_OnTxOpenedMessage, &TxOpenedMessage{
		SwapId:          swap.Id,
		MakerPubkeyHash: makerPubkeyHash,
		Invoice:         "invoice",
		TxHex:           "txhex",
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapInReceiver_AwaitTxConfirmation, swap.Current)
	_, err = swap.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedCoop, swap.Current)

}
