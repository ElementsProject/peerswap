package swap

import (
	"os"
	"path"
	"testing"

	"github.com/elementsproject/peerswap/messages"
	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/premium"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func Test_SwapInSenderValidSwap(t *testing.T) {
	swapAmount := uint64(100000)

	initiator, peer, takerPubkeyHash, _, chanId := getTestParams()
	msgChan := make(chan PeerMessage)

	timeOutD := &timeOutDummy{}

	swapServices := getSwapServices(t, msgChan)
	swapServices.toService = timeOutD
	swap := newSwapInSenderFSM(swapServices, initiator, peer)

	_, err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapInRequestMessage{
		Amount:          swapAmount,
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swap.SwapId,
		Network:         "mainnet",
		Scid:            chanId,
		Pubkey:          initiator,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check if timeout was set
	assert.Equal(t, 1, timeOutD.getCalled())

	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPINREQUEST, msg.MessageType())
	assert.Equal(t, State_SwapInSender_AwaitAgreement, swap.Current)

	_, _ = swap.SendEvent(Event_SwapInSender_OnAgreementReceived, &SwapInAgreementMessage{
		SwapId: swap.SwapId,
		Pubkey: takerPubkeyHash,
	})
	msg = <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_OPENINGTXBROADCASTED, msg.MessageType())
	assert.Equal(t, State_SwapInSender_AwaitClaimPayment, swap.Current)
	_, err = swap.SendEvent(Event_OnClaimInvoicePaid, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedPreimage, swap.Current)

}

func Test_SwapInSenderCancel1(t *testing.T) {
	swapAmount := uint64(100000)
	initiator, peer, _, _, chanId := getTestParams()
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(t, msgChan)
	swap := newSwapInSenderFSM(swapServices, initiator, peer)

	_, err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapInRequestMessage{
		Amount:          swapAmount,
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swap.SwapId,
		Network:         "mainnet",
		Scid:            chanId,
		Pubkey:          initiator,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPINREQUEST, msg.MessageType())
	assert.Equal(t, State_SwapInSender_AwaitAgreement, swap.Current)
	_, err = swap.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapCanceled, swap.Current)
}

func Test_SwapInSenderCoopClose(t *testing.T) {

	swapAmount := uint64(100000)
	initiator, peer, takerPubkeyHash, _, chanId := getTestParams()
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(t, msgChan)
	swap := newSwapInSenderFSM(swapServices, initiator, peer)

	_, err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapInRequestMessage{
		Amount:          swapAmount,
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swap.SwapId,
		Network:         "mainnet",
		Scid:            chanId,
		Pubkey:          initiator,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPINREQUEST, msg.MessageType())
	assert.Equal(t, State_SwapInSender_AwaitAgreement, swap.Current)

	_, _ = swap.SendEvent(Event_SwapInSender_OnAgreementReceived, &SwapInAgreementMessage{
		SwapId: swap.SwapId,
		Pubkey: takerPubkeyHash,
	})
	msg = <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_OPENINGTXBROADCASTED, msg.MessageType())
	assert.Equal(t, State_SwapInSender_AwaitClaimPayment, swap.Current)

	_, err = swap.SendEvent(Event_OnCoopCloseReceived, &CoopCloseMessage{
		SwapId:  swap.SwapId,
		Message: "",
		Privkey: getRandom32ByteHexString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedCoop, swap.Current)

}
func getSwapServices(t *testing.T, msgChan chan PeerMessage) *SwapServices {
	store := &dummyStore{dataMap: map[string]*SwapStateMachine{}}
	reqSwapsStore := &requestedSwapsStoreMock{data: map[string][]RequestedSwap{}}
	messenger := &dummyMessenger{msgChan: msgChan}
	lc := &dummyLightningClient{preimage: "fee"}
	policy := &dummyPolicy{
		getMinSwapAmountMsatReturn: policy.DefaultPolicy().MinSwapAmountMsat,
		newSwapsAllowedReturn:      policy.DefaultPolicy().AllowNewSwaps,
	}
	chain := &dummyChain{returnGetCSVHeight: 1008}
	chain.SetBalance(1000000)

	mmgr := &MessengerManagerStub{}
	dir := t.TempDir()
	db, err := bbolt.Open(path.Join(dir, "premium-db"), os.ModePerm, nil)
	require.NoError(t, err)
	premium, err := premium.NewSetting(db)
	require.NoError(t, err)
	swapServices := NewSwapServices(store, reqSwapsStore, lc, messenger, mmgr, policy, true, chain, chain, chain, true, chain, chain, chain, premium)
	swapServices.toService = &timeOutDummy{}
	return swapServices
}
