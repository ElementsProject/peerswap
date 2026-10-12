package swap

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const (
	coopRecovery            = "coop recovery"
	keyRecovery             = "key recovery"
	receiveInvoice          = "receive"
	confirmationRecovery    = "confirmation recovery"
	undecodableClaimInvoice = "err"
)

type invoiceRecoveryStore struct {
	Store
	fsm *SwapStateMachine
}

func (s *invoiceRecoveryStore) ListAll() ([]*SwapStateMachine, error) {
	return []*SwapStateMachine{s.fsm}, nil
}

func TestRejectedClaimInvoiceReleasesChannel(t *testing.T) {
	for _, direction := range []SwapType{SWAPTYPE_IN, SWAPTYPE_OUT} {
		for _, invalid := range []string{"decode", "amount", "cltv"} {
			for _, path := range []string{receiveInvoice, confirmationRecovery, keyRecovery, coopRecovery} {
				t.Run(direction.String()+"/"+invalid+"/"+path, func(t *testing.T) {
					testRejectedClaimInvoiceReleasesChannel(t, direction, invalid, path)
				})
			}
		}
	}
}

func testRejectedClaimInvoiceReleasesChannel(t *testing.T, direction SwapType, invalid, path string) {
	t.Helper()
	services, data, watcher := bitcoinPaymentFixture(t, 800_003, 503)
	data.ClaimPaymentHash = "" // The invoice has never passed validation.
	data.SwapInRequest.Scid = "1x2x3"
	lc := services.lightning.(*dummyLightningClient)
	switch invalid {
	case "decode":
		data.OpeningTxBroadcasted.Payreq = undecodableClaimInvoice
	case "amount":
		lc.decodeAmount--
	case "cltv":
		lc.decodeFinalCLTV++
	}
	// Backends cannot decode a malformed invoice. An RPC outage
	// also must not strand an invoice rejected for amount or CLTV.
	lc.recoverPaymentError = errors.New("invoice lookup unavailable")
	messenger := &suspensionMessenger{}
	services.messenger = messenger
	fsm := newSwapInReceiverFSM(data.GetId(), services, "peer")
	states := map[string]StateType{
		receiveInvoice:       State_SwapInReceiver_AwaitTxBroadcastedMessage,
		confirmationRecovery: State_SwapInReceiver_AwaitTxConfirmation,
		keyRecovery:          State_SwapInReceiver_SendPrivkey,
		coopRecovery:         State_SwapInReceiver_SendCoopClose,
	}
	if direction == SWAPTYPE_OUT {
		request := data.SwapInRequest
		data.SwapOutRequest = &SwapOutRequestMessage{
			SwapId: request.SwapId, Amount: request.Amount, Scid: request.Scid,
			Network: request.Network, ProtocolVersion: request.ProtocolVersion,
		}
		data.SwapOutAgreement = &SwapOutAgreementMessage{ProtocolVersion: PEERSWAP_PROTOCOL_VERSION}
		data.SwapInRequest, data.SwapInAgreement = nil, nil
		fsm = newSwapOutSenderFSM(services, "local", "peer")
		states = map[string]StateType{
			receiveInvoice:       State_SwapOutSender_AwaitTxBroadcastedMessage,
			confirmationRecovery: State_SwapOutSender_AwaitTxConfirmation,
			keyRecovery:          State_SwapOutSender_SendPrivkey,
			coopRecovery:         State_SwapOutSender_SendCoopClose,
		}
	}
	fsm.SwapId, fsm.Data, fsm.Current = data.GetId(), data, states[path]
	service := NewSwapService(services)
	timeouts := &timeOutDummy{}
	services.toService = timeouts
	if path == receiveInvoice {
		message := data.OpeningTxBroadcasted
		message.SwapId, message.TxId = data.GetId(), strings.Repeat("01", 32)
		data.OpeningTxBroadcasted = nil
		assertNoError(t, service.lockSwap(data.GetId().String(), data.GetScid(), fsm))
		assertNoError(t, service.OnTxOpenedMessage(message))
	} else {
		// Exercise the service's restart and active-channel cleanup path.
		// #nosec G117 -- Only synthetic test invoice and key data are serialized.
		encoded, err := json.Marshal(fsm)
		assertNoError(t, err)
		var restored SwapStateMachine
		assertNoError(t, json.Unmarshal(encoded, &restored))
		fsm = &restored
		services.swapStore = &invoiceRecoveryStore{Store: services.swapStore, fsm: fsm}
		assertNoError(t, service.RecoverSwaps())
	}
	assertEqual(t, State_ClaimedCoop, fsm.Current)
	assertEqual(t, "", fsm.Data.ClaimPaymentHash)
	assertEqual(t, 0, lc.recoverPaymentCalled)
	assertEqual(t, 0, lc.rebalanceCalled)
	assertEqual(t, 0, timeouts.getCalled())
	assertEqual(t, 0, watcher.watches)
	assertEqual(t, true, len(messenger.payload) > 0)
	_, err := service.GetActiveSwap(data.GetId().String())
	if !errors.Is(err, ErrSwapDoesNotExist) {
		t.Fatalf("rejected invoice still occupies channel: %v", err)
	}
	assertNoError(t, service.lockSwap(NewSwapId().String(), data.GetScid(), fsm))
}

func TestClaimPaymentRequiresValidatedHash(t *testing.T) {
	for _, hash := range []string{"", "different"} {
		t.Run("hash="+hash, func(t *testing.T) {
			services, data, _ := bitcoinPaymentFixture(t, 800_003, 503)
			data.OpeningTxHex = bitcoinTestOpeningHex
			data.ClaimPaymentHash = hash
			assertEqual(t, Event_ActionFailed, (&ValidateTxAndPayClaimInvoiceAction{}).Execute(services, data))
			if data.LastErr == nil || !strings.Contains(data.LastErr.Error(), "payment hash") {
				t.Fatalf("expected payment hash validation error, got %v", data.LastErr)
			}
			assertEqual(t, 0, services.lightning.(*dummyLightningClient).rebalanceCalled)
		})
	}
}

func TestClaimPaymentRecoveryPrecedesHashValidation(t *testing.T) {
	for _, hash := range []string{"", "different"} {
		for _, outcome := range []string{"settled", "in flight", "unavailable", "saved preimage"} {
			t.Run("hash="+hash+"/"+outcome, func(t *testing.T) {
				services, data, _ := bitcoinPaymentFixture(t, 802_000, 504)
				data.OpeningTxHex, data.ClaimPaymentHash = bitcoinTestOpeningHex, hash
				data.StartingBlockHeightSet = false
				lc := services.lightning.(*dummyLightningClient)
				want := NoOp
				switch outcome {
				case "settled":
					lc.recoverPaymentPreimage, want = bitcoinTestPreimage, Event_ActionSucceeded
				case "in flight":
					lc.recoverPaymentError = errors.New("payment pending")
				case "unavailable":
					lc.recoverPaymentError = errors.New("RPC disconnected")
				case "saved preimage":
					data.ClaimPreimage, want = bitcoinTestPreimage, Event_ActionSucceeded
				}
				assertEqual(t, want, (&ValidateTxAndPayClaimInvoiceAction{}).Execute(services, data))
				assertEqual(t, 0, lc.rebalanceCalled)
				if want == NoOp {
					assertEqual(t, 1, services.toService.(*timeOutDummy).getCalled())
				} else {
					assertEqual(t, bitcoinTestPreimage, data.ClaimPreimage)
				}
			})
		}
	}
}

func TestSavedClaimPreimagePreventsKeyReleaseWithoutHash(t *testing.T) {
	services, data, _ := bitcoinPaymentFixture(t, 800_003, 503)
	data.ClaimPaymentHash, data.ClaimPreimage = "", bitcoinTestPreimage
	assertEqual(t, Event_OnClaimInvoicePaid, (&TakerSendPrivkeyAction{}).Execute(services, data))
	assertEqual(t, 0, services.lightning.(*dummyLightningClient).recoverPaymentCalled)
	assertEqual(t, 0, len(data.NextMessage))
}
