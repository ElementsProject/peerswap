package swap

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GoodCase(t *testing.T) {

	channelId := "chanId"
	amount := uint64(100)
	peer := "bob"
	initiator := "alice"

	aliceSwapService := getTestSetup("alice")
	bobSwapService := getTestSetup("bob")
	aliceSwapService.swapServices.messenger.(*ConnectedMessenger).other = bobSwapService.swapServices.messenger.(*ConnectedMessenger)
	bobSwapService.swapServices.messenger.(*ConnectedMessenger).other = aliceSwapService.swapServices.messenger.(*ConnectedMessenger)

	aliceSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan = make(chan messages.MessageType)
	bobSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan = make(chan messages.MessageType)

	aliceMsgChan := aliceSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan
	bobMsgChan := bobSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan

	err := aliceSwapService.Start()
	if err != nil {
		t.Fatal(err)
	}
	err = bobSwapService.Start()
	if err != nil {
		t.Fatal(err)
	}
	aliceSwap, err := aliceSwapService.SwapOut(peer, "l-btc", channelId, initiator, amount)
	if err != nil {
		t.Fatalf(" error swapping oput %v: ", err)
	}

	bobReceivedMsg := <-bobMsgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPOUTREQUEST, bobReceivedMsg)
	bobSwap := bobSwapService.activeSwaps[aliceSwap.Id]

	aliceReceivedMsg := <-aliceMsgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPOUTAGREEMENT, aliceReceivedMsg)
	assert.Equal(t, State_SwapOutSender_AwaitTxBroadcastedMessage, aliceSwap.Current)
	assert.Equal(t, State_SwapOutReceiver_AwaitFeeInvoicePayment, bobSwap.Current)
	bobSwapService.swapServices.lightning.(*dummyLightningClient).TriggerPayment(&glightning.Payment{
		Label: "fee_" + bobSwap.Id,
	})
	assert.Equal(t, State_SwapOutReceiver_AwaitClaimInvoicePayment, bobSwap.Current)

	aliceReceivedMsg = <-aliceMsgChan
	assert.Equal(t, messages.MESSAGETYPE_OPENINGTXBROADCASTED, aliceReceivedMsg)

	// trigger openingtx confirmed
	err = aliceSwapService.swapServices.liquidTxWatcher.(*dummyChain).txConfirmedFunc(aliceSwap.Id, aliceSwap.Data.OpeningTxHex)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedPreimage, aliceSwap.Current)

	// trigger bob payment received
	bobSwapService.swapServices.lightning.(*dummyLightningClient).TriggerPayment(&glightning.Payment{
		Label: "claim_" + bobSwap.Id,
	})
	assert.Equal(t, State_ClaimedPreimage, bobSwap.Current)
}
func Test_FeePaymentFailed(t *testing.T) {
	channelId := "chanId"
	amount := uint64(100)
	peer := "bob"
	initiator := "alice"

	aliceSwapService := getTestSetup("alice")
	bobSwapService := getTestSetup("bob")

	// set lightning to fail
	aliceSwapService.swapServices.lightning.(*dummyLightningClient).failpayment = true

	aliceSwapService.swapServices.messenger.(*ConnectedMessenger).other = bobSwapService.swapServices.messenger.(*ConnectedMessenger)
	bobSwapService.swapServices.messenger.(*ConnectedMessenger).other = aliceSwapService.swapServices.messenger.(*ConnectedMessenger)

	aliceSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan = make(chan messages.MessageType)
	bobSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan = make(chan messages.MessageType)

	aliceMsgChan := aliceSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan
	bobMsgChan := bobSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan

	err := aliceSwapService.Start()
	if err != nil {
		t.Fatal(err)
	}
	err = bobSwapService.Start()
	if err != nil {
		t.Fatal(err)
	}
	aliceSwap, err := aliceSwapService.SwapOut(peer, "btc", channelId, initiator, amount)
	if err != nil {
		t.Fatalf(" error swapping oput %v: ", err)
	}
	bobReceivedMsg := <-bobMsgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPOUTREQUEST, bobReceivedMsg)
	bobSwap, err := bobSwapService.GetActiveSwap(aliceSwap.SwapId.String())
	assert.NoError(t, err)

	aliceReceivedMsg := <-aliceMsgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPOUTAGREEMENT, aliceReceivedMsg)

	assert.Equal(t, State_SwapCanceled, aliceSwap.Current)

	bobReceivedMsg = <-bobMsgChan
	assert.Equal(t, messages.MESSAGETYPE_CANCELED, bobReceivedMsg)
	assert.Equal(t, State_SwapCanceled, bobSwap.Current)
}
func Test_ClaimPaymentFailedCoopClose(t *testing.T) {
	channelId := "chanId"
	amount := uint64(100)
	peer := "bob"
	initiator := "alice"

	aliceSwapService := getTestSetup("alice")
	bobSwapService := getTestSetup("bob")
	aliceSwapService.swapServices.messenger.(*ConnectedMessenger).other = bobSwapService.swapServices.messenger.(*ConnectedMessenger)
	bobSwapService.swapServices.messenger.(*ConnectedMessenger).other = aliceSwapService.swapServices.messenger.(*ConnectedMessenger)

	aliceSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan = make(chan messages.MessageType)
	bobSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan = make(chan messages.MessageType)

	aliceMsgChan := aliceSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan
	bobMsgChan := bobSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan

	err := aliceSwapService.Start()
	if err != nil {
		t.Fatal(err)
	}
	err = bobSwapService.Start()
	if err != nil {
		t.Fatal(err)
	}
	aliceSwap, err := aliceSwapService.SwapOut(peer, "btc", channelId, initiator, amount)
	if err != nil {
		t.Fatalf(" error swapping oput %v: ", err)
	}
	bobReceivedMsg := <-bobMsgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPOUTREQUEST, bobReceivedMsg)
	bobSwap := bobSwapService.activeSwaps[aliceSwap.Id]

	aliceReceivedMsg := <-aliceMsgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPOUTAGREEMENT, aliceReceivedMsg)

	assert.Equal(t, State_SwapOutSender_AwaitTxBroadcastedMessage, aliceSwap.Current)
	assert.Equal(t, State_SwapOutReceiver_AwaitFeeInvoicePayment, bobSwap.Current)

	bobSwapService.swapServices.lightning.(*dummyLightningClient).TriggerPayment(&glightning.Payment{
		Label: "fee_" + bobSwap.Id,
	})
	assert.Equal(t, State_SwapOutReceiver_AwaitClaimInvoicePayment, bobSwap.Current)

	aliceReceivedMsg = <-aliceMsgChan
	assert.Equal(t, messages.MESSAGETYPE_OPENINGTXBROADCASTED, aliceReceivedMsg)

	// trigger openingtx confirmed
	aliceSwapService.swapServices.lightning.(*dummyLightningClient).failpayment = true
	err = aliceSwapService.swapServices.liquidTxWatcher.(*dummyChain).txConfirmedFunc(aliceSwap.Id, aliceSwap.Data.OpeningTxHex)
	if err != nil {
		t.Fatal(err)
	}
	// wants to await the csv claim before it goes to a
	// finish state, such that the channel is still
	// locked for furhter peerswap requests.
	assert.Equal(t, State_ClaimedCoop, aliceSwap.Current)

	// trigger bob payment received

	bobReceivedMsg = <-bobMsgChan
	assert.Equal(t, messages.MESSAGETYPE_COOPCLOSE, bobReceivedMsg)
	assert.Equal(t, State_ClaimedCoop, bobSwap.Current)
}

func Test_OnlyOneActiveSwapPerChannel(t *testing.T) {
	service := getTestSetup("alice")
	service.AddActiveSwap("swapid", &SwapStateMachine{
		Id: "swapid",
		Data: &SwapData{
			FSMState:         "",
			Role:             0,
			CreatedAt:        0,
			InitiatorNodeId:  "",
			PeerNodeId:       "",
			PrivkeyBytes:     []byte{},
			ClaimPreimage:    "",
			ClaimPaymentHash: "",
			FeePreimage:      "",
			OpeningTxFee:     0,
			OpeningTxHex:     "",
			ClaimTxId:        "",
			CancelMessage:    "",
			LastErr:          nil,
			LastErrString:    "",
			SwapInRequest:    &SwapInRequestMessage{Scid: "channelID"},
		},
		Type:     0,
		Role:     0,
		Previous: "",
		Current:  "",
		States: map[StateType]State{
			"": {
				Action: nil,
				Events: map[EventType]StateType{
					"": "",
				},
			},
		},
		swapServices: &SwapServices{
			swapStore:        nil,
			lightning:        nil,
			messenger:        nil,
			policy:           nil,
			bitcoinTxWatcher: nil,
			liquidTxWatcher:  nil,
		},
		retries:  0,
		failures: 0,
	})

	_, err := service.SwapOut("peer", "l-btc", "channelID", "alice", uint64(200))
	if assert.Error(t, err, "expected error") {
		assert.Equal(t, "already has an active swap on channel", err.Error())
	}

	_, err = service.SwapIn("peer", "l-btc", "channelID", "alice", uint64(200))
	if assert.Error(t, err, "expected error") {
		assert.Equal(t, "already has an active swap on channel", err.Error())
	}
}

func TestMessageFromUnexpectedPeer(t *testing.T) {
	channelId := "chanId"
	amount := uint64(100)
	peer := "bob"
	initiator := "alice"

	aliceSwapService := getTestSetup("alice")
	bobSwapService := getTestSetup("bob")
	aliceSwapService.swapServices.messenger.(*ConnectedMessenger).other = bobSwapService.swapServices.messenger.(*ConnectedMessenger)
	bobSwapService.swapServices.messenger.(*ConnectedMessenger).other = aliceSwapService.swapServices.messenger.(*ConnectedMessenger)

	aliceSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan = make(chan messages.MessageType)
	bobSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan = make(chan messages.MessageType)

	aliceMsgChan := aliceSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan
	bobMsgChan := bobSwapService.swapServices.messenger.(*ConnectedMessenger).msgReceivedChan

	err := aliceSwapService.Start()
	if err != nil {
		t.Fatal(err)
	}
	err = bobSwapService.Start()
	if err != nil {
		t.Fatal(err)
	}
	aliceSwap, err := aliceSwapService.SwapOut(peer, "btc", channelId, initiator, amount)
	if err != nil {
		t.Fatalf(" error swapping oput %v: ", err)
	}
	bobReceivedMsg := <-bobMsgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPOUTREQUEST, bobReceivedMsg)
	bobSwap := bobSwapService.activeSwaps[aliceSwap.Id]

	aliceReceivedMsg := <-aliceMsgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPOUTAGREEMENT, aliceReceivedMsg)

	assert.Equal(t, State_SwapOutSender_AwaitTxBroadcastedMessage, aliceSwap.Current)
	assert.Equal(t, State_SwapOutReceiver_AwaitFeeInvoicePayment, bobSwap.Current)

	bobSwapService.swapServices.lightning.(*dummyLightningClient).TriggerPayment(&glightning.Payment{
		Label: "fee_" + bobSwap.Id,
	})
	assert.Equal(t, State_SwapOutReceiver_AwaitClaimInvoicePayment, bobSwap.Current)

	aliceReceivedMsg = <-aliceMsgChan
	assert.Equal(t, messages.MESSAGETYPE_OPENINGTXBROADCASTED, aliceReceivedMsg)

	// Setup done.
	// Sending messages from unexpected peer.
	charlieMessenger := &ConnectedMessenger{
		thisPeerId:      "charlie",
		other:           aliceSwapService.swapServices.messenger.(*ConnectedMessenger),
		msgReceivedChan: make(chan messages.MessageType),
	}

	type test struct {
		name        string
		message     PeerMessage
		assertError bool
	}

	tests := []test{
		{name: "swap in agreement message", message: &SwapInAgreementMessage{SwapId: aliceSwap.SwapId}, assertError: true},
		{name: "swap out agreement message", message: &SwapOutAgreementMessage{SwapId: aliceSwap.SwapId}, assertError: true},
		{name: "opening tx broadcasted message", message: &OpeningTxBroadcastedMessage{SwapId: aliceSwap.SwapId}, assertError: true},
		{name: "coop close message", message: &CoopCloseMessage{SwapId: aliceSwap.SwapId}, assertError: true},
		{name: "cancel message", message: &CancelMessage{SwapId: aliceSwap.SwapId}, assertError: true},
		{name: "swap in request message", message: &SwapInRequestMessage{SwapId: NewSwapId()}, assertError: false},
		{name: "swap out request message", message: &SwapOutRequestMessage{SwapId: NewSwapId()}, assertError: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset error for clean test.
			aliceMessenger := aliceSwap.swapServices.messenger.(*ConnectedMessenger)
			aliceMessenger.lastErr = nil
			require.NoError(t, aliceMessenger.lastErr)

			msgBytes, msgType, err := MarshalPeerswapMessage(tc.message)
			require.NoError(t, err)

			charlieMessenger.SendMessage("alice", msgBytes, msgType)
			<-aliceMsgChan

			if tc.assertError {
				require.Error(t, aliceMessenger.lastErr)
				assert.Equal(t, ErrReceivedMessageFromUnexpectedPeer(charlieMessenger.thisPeerId, aliceSwap.SwapId).Error(), aliceSwapService.swapServices.messenger.(*ConnectedMessenger).lastErr.Error())
			} else {
				require.NoError(t, aliceMessenger.lastErr)
			}
		})
	}
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	sws := getTestSetup("alice")
	sws.swapServices.messenger = &noopMessenger{}
	sws.Start()

	fsm := newSwapInSenderFSM(sws.swapServices, "alice", "bob")
	sws.AddActiveSwap(fsm.Id, fsm)

	fsm.Current = State_SwapInSender_AwaitAgreement
	sws.swapServices.toService.addNewTimeOut(context.Background(), 10*time.Millisecond, fsm.Id)

	tm := time.NewTimer(1 * time.Second)

	for {
		select {
		case <-tm.C:
			t.Errorf("expected state to change to State_SwapCanceled")
			return
		default:
			fsm.mutex.Lock()
			if fsm.Current == State_SwapCanceled {
				fsm.mutex.Unlock()
				tm.Stop()
				return
			}
			fsm.mutex.Unlock()
		}
	}
}

func getTestSetup(name string) *SwapService {
	store := &dummyStore{dataMap: map[string]*SwapStateMachine{}}
	reqSwapsStore := &requestedSwapsStoreMock{data: map[string][]RequestedSwap{}}
	messenger := &ConnectedMessenger{
		thisPeerId: name,
	}
	mmgr := &MessengerManagerStub{}
	lc := &dummyLightningClient{preimage: ""}
	policy := &dummyPolicy{}
	chain := &dummyChain{}
	swapServices := NewSwapServices(store, reqSwapsStore, lc, messenger, mmgr, policy, true, chain, chain, chain, true, chain, chain, chain)
	swapService := NewSwapService(swapServices)
	return swapService
}

type ConnectedMessenger struct {
	sync.Mutex
	thisPeerId      string
	OnMessage       func(peerId string, msgType string, msgBytes []byte) error
	other           *ConnectedMessenger
	msgReceivedChan chan messages.MessageType
	lastErr         error
}

func (c *ConnectedMessenger) SendMessage(peerId string, msg []byte, msgType int) error {
	go func() {
		time.Sleep(time.Millisecond * 10)
		msgString := messages.MessageTypeToHexString(messages.MessageType(msgType))
		err := c.other.OnMessage(c.thisPeerId, msgString, msg)
		if err != nil {
			log.Printf("error on message send %v", err)
			c.other.Lock()
			c.other.lastErr = err
			c.other.Unlock()
		}
		if c.other.msgReceivedChan != nil {
			c.other.msgReceivedChan <- messages.MessageType(msgType)
		}
	}()

	return nil
}

func (c *ConnectedMessenger) AddMessageHandler(f func(peerId string, msgType string, msgBytes []byte) error) {
	c.OnMessage = f
}

type MessengerManagerStub struct {
	sync.Mutex
	called  int
	added   int
	removed int
}

func (s *MessengerManagerStub) AddSender(id string, messenger messages.StoppableMessenger) error {
	s.Lock()
	defer s.Unlock()
	s.called++
	s.added++
	return nil
}

func (s *MessengerManagerStub) RemoveSender(id string) {
	s.Lock()
	defer s.Unlock()
	s.called++
	s.removed++
}

type noopMessenger struct {
}

func (m *noopMessenger) SendMessage(peerId string, msg []byte, msgType int) error {
	return nil
}

func (m *noopMessenger) AddMessageHandler(f func(peerId string, msgType string, msgBytes []byte) error) {
}
