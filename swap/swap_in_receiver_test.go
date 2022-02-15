package swap

import (
	"testing"

	"github.com/sputn1ck/peerswap/messages"
	"github.com/stretchr/testify/assert"
)

func Test_SwapInReceiverValid(t *testing.T) {

	swapId := NewSwapId()
	swapAmount := uint64(100)
	initiator, peer, _, _, chanId := getTestParams()
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInReceiverFSM(swapId, swapServices, peer)
	_, err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &SwapInRequestMessage{
		Amount:          swapAmount,
		Pubkey:          initiator,
		Scid:            chanId,
		SwapId:          swapId,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPINAGREEMENT, msg.MessageType())
	assert.Equal(t, State_SwapInReceiver_AwaitTxBroadcastedMessage, swap.Current)

	_, err = swap.SendEvent(Event_OnTxOpenedMessage, &OpeningTxBroadcastedMessage{
		SwapId:      swap.SwapId,
		Payreq:      "invoice",
		TxId:        getRandom32ByteHexString(),
		ScriptOut:   0,
		BlindingKey: "",
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

	swapId := NewSwapId()
	swapAmount := uint64(100)
	//initiator := "ab123"
	initiator, peer, _, _, chanId := getTestParams()
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInReceiverFSM(swapId, swapServices, peer)

	_, err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &SwapInRequestMessage{
		Amount:          swapAmount,
		Pubkey:          initiator,
		Scid:            chanId,
		SwapId:          swapId,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
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

	swapId := NewSwapId()
	swapAmount := uint64(100)
	initiator, peer, _, _, chanId := getTestParams()
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swap := newSwapInReceiverFSM(swapId, swapServices, peer)

	_, err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &SwapInRequestMessage{
		Amount:          swapAmount,
		Pubkey:          initiator,
		Scid:            chanId,
		SwapId:          swapId,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPINAGREEMENT, msg.MessageType())
	assert.Equal(t, State_SwapInReceiver_AwaitTxBroadcastedMessage, swap.Current)

	_, err = swap.SendEvent(Event_OnTxOpenedMessage, &OpeningTxBroadcastedMessage{
		SwapId:      swap.SwapId,
		Payreq:      "invoice",
		TxId:        getRandom32ByteHexString(),
		ScriptOut:   0,
		BlindingKey: "",
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
