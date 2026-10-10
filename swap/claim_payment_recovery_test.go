package swap

import (
	"errors"
	"testing"
)

const (
	bitcoinTestOpeningHex = "opening"
	bitcoinTestInvoice    = "claim"
	bitcoinTestPreimage   = "preimage"
)

type paymentWindowWatcher struct {
	TxWatcher
	height  uint32
	watches int
}

func (w *paymentWindowWatcher) GetBlockHeight() (uint32, error) { return w.height, nil }

func (w *paymentWindowWatcher) AddWaitForConfirmationTx(string, string, uint32, uint32, uint32, []byte) {
	w.watches++
}

func bitcoinPaymentFixture(t *testing.T, height uint32, cltv int64) (*SwapServices, *SwapData, *paymentWindowWatcher) {
	t.Helper()
	services := getSwapServices(t, nil)
	w := &paymentWindowWatcher{TxWatcher: services.bitcoinTxWatcher, height: height}
	services.bitcoinTxWatcher = w
	lc := services.lightning.(*dummyLightningClient)
	lc.decodeAmount = 100_000_000
	lc.decodeFinalCLTV = cltv
	data := swapDataForPolicy(btc_chain, PEERSWAP_PROTOCOL_VERSION)
	data.SwapInRequest.SwapId = NewSwapId()
	data.SwapInRequest.Amount = 100_000
	data.StartingBlockHeight = 800_000
	data.StartingBlockHeightSet = true
	data.OpeningTxBroadcasted = &OpeningTxBroadcastedMessage{Payreq: bitcoinTestInvoice}
	return services, data, w
}

func TestBitcoinRecoveryFollowsExistingPaymentBeforeNewPaymentLimits(t *testing.T) {
	for _, confirmedState := range []bool{false, true} {
		services, data, _ := bitcoinPaymentFixture(t, 802_000, 504)
		data.StartingBlockHeightSet = false
		data.OpeningTxHex = bitcoinTestOpeningHex
		lc := services.lightning.(*dummyLightningClient)
		lc.recoverPaymentPreimage = bitcoinTestPreimage
		fsm := newSwapInReceiverFSM(data.GetId(), services, "peer")
		fsm.Data = data
		fsm.Current = State_SwapInReceiver_AwaitTxConfirmation
		if confirmedState {
			fsm.Current = State_SwapInReceiver_ValidateTxAndPayClaimInvoice
		}
		done, err := fsm.Recover()
		assertNoError(t, err)
		assertEqual(t, true, done)
		assertEqual(t, State_ClaimedPreimage, fsm.Current)
		assertEqual(t, 0, lc.rebalanceCalled)
		assertEqual(t, bitcoinTestPreimage, data.ClaimPreimage)
	}
}

func TestBitcoinUnresolvedPaymentRetainsKeyAndResumes(t *testing.T) {
	for _, state := range []StateType{
		State_SwapInReceiver_ValidateTxAndPayClaimInvoice,
		State_SwapInReceiver_SendPrivkey,
		State_SwapInReceiver_SendCoopClose,
	} {
		t.Run(string(state), func(t *testing.T) {
			services, data, watcher := bitcoinPaymentFixture(t, 802_000, 504)
			data.OpeningTxHex = bitcoinTestOpeningHex
			messenger := &suspensionMessenger{}
			services.messenger = messenger
			if state == State_SwapInReceiver_SendCoopClose {
				data.NextMessage = []byte("previously prepared cooperative close")
			}
			lc := services.lightning.(*dummyLightningClient)
			lc.recoverPaymentError = errors.New("RPC disconnected while HTLC is held")
			fsm := newSwapInReceiverFSM(data.GetId(), services, "peer")
			fsm.Data, fsm.Current = data, state
			done, err := fsm.Recover()
			assertNoError(t, err)
			assertEqual(t, false, done)
			assertEqual(t, state, fsm.Current)
			assertEqual(t, 0, len(messenger.payload))
			assertEqual(t, 1, services.toService.(*timeOutDummy).getCalled())
			assertEqual(t, 0, lc.rebalanceCalled)

			// Another block arrives while the RPC is down. A later success must claim, not refund.
			watcher.height++
			lc.recoverPaymentError = nil
			lc.recoverPaymentPreimage = bitcoinTestPreimage
			done, err = fsm.SendEvent(Event_OnTimeout, nil)
			assertNoError(t, err)
			assertEqual(t, true, done)
			assertEqual(t, State_ClaimedPreimage, fsm.Current)
			assertEqual(t, 0, len(messenger.payload))
			assertEqual(t, 0, lc.rebalanceCalled)
		})
	}
}

func TestBitcoinTimeoutCannotRefundPaidSwap(t *testing.T) {
	services, data, _ := bitcoinPaymentFixture(t, 800_003, 503)
	data.ClaimPreimage = bitcoinTestPreimage
	fsm := newSwapOutSenderFSM(services, "local", "peer")
	fsm.SwapId, fsm.Data = data.GetId(), data
	fsm.Current = State_SwapOutSender_ClaimSwap
	done, err := fsm.SendEvent(Event_OnTimeout, nil)
	if !errors.Is(err, ErrEventRejected) {
		t.Fatalf("expected rejected timeout, got %v", err)
	}
	assertEqual(t, false, done)
	assertEqual(t, 0, len(data.NextMessage))
}

func TestBitcoinCoopCloseChecksPaymentBeforeConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		preimage string
		err      error
		event    EventType
		sent     bool
	}{
		{"paid", bitcoinTestPreimage, nil, Event_OnClaimInvoicePaid, false},
		{"pending", "", errors.New("payment pending"), NoOp, false},
		{"RPC error", "", errors.New("RPC disconnected"), NoOp, false},
		{"missing", "", ErrClaimPaymentNotFound, Event_ActionSucceeded, true},
		{"failed", "", ErrClaimPaymentFailed, Event_ActionSucceeded, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			services, data, _ := bitcoinPaymentFixture(t, 800_003, 503)
			assertEqual(t, "", data.OpeningTxHex)
			// Recovery may resume a previously prepared refund message.
			data.NextMessage = []byte("saved cooperative close")
			messenger := &suspensionMessenger{}
			services.messenger = messenger
			lc := services.lightning.(*dummyLightningClient)
			lc.recoverPaymentPreimage = tc.preimage
			lc.recoverPaymentError = tc.err

			event := (&TakerSendCoopCloseAction{}).Execute(services, data)
			assertEqual(t, tc.event, event)
			assertEqual(t, 1, lc.recoverPaymentCalled)
			assertEqual(t, tc.sent, len(messenger.payload) > 0)
			assertEqual(t, tc.preimage, data.ClaimPreimage)
			assertEqual(t, 0, lc.rebalanceCalled)
		})
	}
}

func TestConfirmationErrorCannotStartClaimPayment(t *testing.T) {
	services, data, _ := bitcoinPaymentFixture(t, 800_003, 503)
	fsm := newSwapInReceiverFSM(data.GetId(), services, "peer")
	fsm.Data, fsm.Current = data, State_SwapInReceiver_AwaitTxConfirmation
	service := NewSwapService(services)
	service.activeSwaps[data.GetId().String()] = fsm
	watchErr := errors.New("exceeded safe payment window")
	assertNoError(t, service.OnTxConfirmed(data.GetId().String(), bitcoinTestOpeningHex, watchErr))
	if !errors.Is(data.LastErr, watchErr) {
		t.Fatalf("watcher error lost: %v", data.LastErr)
	}
	assertEqual(t, "", data.OpeningTxHex)
	assertEqual(t, 0, services.lightning.(*dummyLightningClient).rebalanceCalled)
}
