package swap

import (
	"errors"
	"fmt"
	"testing"

	"github.com/elementsproject/peerswap/messages"
	"github.com/elementsproject/peerswap/policy"
	"github.com/stretchr/testify/assert"
)

func Test_SwapOutReceiverValidSwap(t *testing.T) {
	swapAmount := uint64(100000)
	swapId := NewSwapId()
	_, peer, takerPubkeyHash, _, chanId := getTestParams()

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(t, msgChan)
	swapFSM := newSwapOutReceiverFSM(swapId, swapServices, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapId,
		Pubkey:          takerPubkeyHash,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.SwapOutRequest.Pubkey)
	assert.NotEqual(t, "", swapFSM.Data.SwapOutAgreement.Pubkey)

	// The opening transaction was prechecked before the fee invoice was sent.
	bitcoinWallet := swapServices.bitcoinWallet.(*dummyChain)
	assert.EqualValues(t, 1, bitcoinWallet.precheckOpeningTxCalled)
	assert.Equal(t, swapAmount, bitcoinWallet.precheckOpeningTxParams.Amount)
	assert.Equal(t, takerPubkeyHash, bitcoinWallet.precheckOpeningTxParams.TakerPubkey)

	_, err = swapFSM.SendEvent(Event_OnFeeInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_AwaitClaimInvoicePayment, swapFSM.Current)
	_, err = swapFSM.SendEvent(Event_OnClaimInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedPreimage, swapFSM.Current)

}

func Test_SwapOutReceiverClaimCoop(t *testing.T) {
	swapAmount := uint64(100000)
	swapId := NewSwapId()
	_, peer, takerPubkeyHash, _, chanId := getTestParams()

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(t, msgChan)

	swapFSM := newSwapOutReceiverFSM(swapId, swapServices, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapId,
		Pubkey:          takerPubkeyHash,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.SwapOutRequest.Pubkey)
	assert.NotEqual(t, "", swapFSM.Data.SwapOutAgreement.Pubkey)

	_, err = swapFSM.SendEvent(Event_OnFeeInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutReceiver_AwaitClaimInvoicePayment, swapFSM.Data.GetCurrentState())
	_, err = swapFSM.SendEvent(Event_OnCoopCloseReceived, &CoopCloseMessage{SwapId: swapId, Privkey: getRandom32ByteHexString()})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedCoop, swapFSM.Data.GetCurrentState())

}

func Test_SwapOutReceiverCancelReceived(t *testing.T) {
	swapAmount := uint64(100000)
	swapId := NewSwapId()
	_, peer, takerPubkeyHash, _, chanId := getTestParams()

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(t, msgChan)

	swapFSM := newSwapOutReceiverFSM(swapId, swapServices, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapId,
		Pubkey:          takerPubkeyHash,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.SwapOutRequest.Pubkey)
	assert.NotEqual(t, "", swapFSM.Data.SwapOutAgreement.Pubkey)

	_, err = swapFSM.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())
}

func Test_SwapOutReceiverCancelInternal(t *testing.T) {
	swapAmount := uint64(100000)
	swapId := NewSwapId()
	_, peer, takerPubkeyHash, _, chanId := getTestParams()
	FeePreimage := "err"

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(t, msgChan)
	swapServices.lightning.(*dummyLightningClient).preimage = FeePreimage
	swapFSM := newSwapOutReceiverFSM(swapId, swapServices, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapId,
		Pubkey:          takerPubkeyHash,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.SwapOutRequest.Pubkey)
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())
}

func Test_SwapOutReceiverInsufficientBalance(t *testing.T) {
	swapAmount := uint64(100000)
	swapId := NewSwapId()
	_, peer, takerPubkeyHash, _, chanId := getTestParams()
	FeePreimage := "err"

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(t, msgChan)
	swapServices.bitcoinWallet.(*dummyChain).SetBalance(0)

	swapServices.lightning.(*dummyLightningClient).preimage = FeePreimage
	swapFSM := newSwapOutReceiverFSM(swapId, swapServices, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapId,
		Pubkey:          takerPubkeyHash,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, peer, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.SwapOutRequest.Pubkey)
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())

}

// Test_SwapOutReceiver_PrecheckFailed checks that the swap is canceled before
// the fee invoice is sent if the opening transaction precheck fails, so that
// the peer never pays a prepayment for a swap that can not be performed.
func Test_SwapOutReceiver_PrecheckFailed(t *testing.T) {
	swapAmount := uint64(100000)
	swapId := NewSwapId()
	_, peer, takerPubkeyHash, _, chanId := getTestParams()

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(t, msgChan)
	swapServices.bitcoinWallet.(*dummyChain).precheckOpeningTxErr =
		errors.New("wallet couldn't fund PSBT: insufficient funds available to construct transaction")

	swapFSM := newSwapOutReceiverFSM(swapId, swapServices, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapId,
		Pubkey:          takerPubkeyHash,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}

	// No fee invoice was created; the only message sent to the peer is the
	// cancel with the precheck failure as the reason.
	assert.Nil(t, swapFSM.Data.SwapOutAgreement)
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())
	assert.Contains(t, swapFSM.Data.GetCancelMessage(), "opening transaction precheck failed")
}

// Test_SwapOutReceiver_PeerIsSuspicious checks that a swap request is rejected
// if the peer is on the suspicious peer list.
func Test_SwapOutReceiver_PeerIsSuspicious(t *testing.T) {
	swapAmount := uint64(100000)
	swapId := NewSwapId()
	_, peer, _, _, chanId := getTestParams()
	FeePreimage := "err"

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(t, msgChan)

	// Setup the peer to be suspicious.
	swapServices.policy = &dummyPolicy{
		isPeerSuspiciousReturn:     true,
		getMinSwapAmountMsatReturn: policy.DefaultPolicy().MinSwapAmountMsat,
		newSwapsAllowedReturn:      policy.DefaultPolicy().AllowNewSwaps,
	}

	swapServices.lightning.(*dummyLightningClient).preimage = FeePreimage
	swapFSM := newSwapOutReceiverFSM(swapId, swapServices, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutRequestReceived, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapId,
		Pubkey:          peer,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}

	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())
	assert.Equal(t, fmt.Sprintf("peer %s not allowed to request swaps", peer), swapFSM.Data.CancelMessage)
}
