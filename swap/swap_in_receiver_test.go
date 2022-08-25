package swap

import (
	"fmt"
	"testing"

	"github.com/elementsproject/peerswap/messages"
	"github.com/stretchr/testify/assert"
)

func Test_SwapInReceiverValid(t *testing.T) {

	swapId := NewSwapId()
	swapAmount := uint64(100000)
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
	swapAmount := uint64(100000)
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
	swapAmount := uint64(100000)
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

// Test_SwapInReceiver_PeerIsSuspicious checks that a swap request is rejected
// if the peer is on the suspicious peer list.
func Test_SwapInReceiver_PeerIsSuspicious(t *testing.T) {
	swapAmount := uint64(100000)
	swapId := NewSwapId()
	_, initiator, _, _, chanId := getTestParams()

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	// Setup the peer to be suspicious.
	swapServices.policy = &dummyPolicy{isPeerSuspiciousReturn: true}

	swap := newSwapInReceiverFSM(swapId, swapServices, initiator)

	_, err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &SwapInRequestMessage{
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swapId,
		Network:         "mainnet",
		Asset:           "",
		Scid:            chanId,
		Amount:          swapAmount,
		Pubkey:          initiator,
	})
	if err != nil {
		t.Fatal(err)
	}

	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapCanceled, swap.Data.GetCurrentState())
	assert.Equal(t, fmt.Sprintf("peer %s not allowed to request swaps", initiator), swap.Data.CancelMessage)
}
