package swap

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/elementsproject/peerswap/log"

	"github.com/btcsuite/btcd/btcec"
	"github.com/elementsproject/peerswap/isdev"
	"github.com/elementsproject/peerswap/lightning"
	"github.com/elementsproject/peerswap/messages"
)

type CheckRequestWrapperAction struct {
	next Action
}

func (a CheckRequestWrapperAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.GetChain() == l_btc_chain && !services.liquidEnabled {
		swap.LastErr = errors.New("lbtc swaps are not supported")
		swap.CancelMessage = "lbtc swaps are not supported"
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.GetChain(),
			AmountSat:       swap.GetAmount(),
			Type:            swap.GetType(),
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}

	if swap.GetChain() == btc_chain && !services.bitcoinEnabled {
		swap.LastErr = errors.New("btc swaps are not supported")
		swap.CancelMessage = "btc swaps are not supported"
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.GetChain(),
			AmountSat:       swap.GetAmount(),
			Type:            swap.GetType(),
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}

	if swap.GetProtocolVersion() != PEERSWAP_PROTOCOL_VERSION {
		swap.CancelMessage = "incompatible peerswap version"
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.GetChain(),
			AmountSat:       swap.GetAmount(),
			Type:            swap.GetType(),
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}

	if swap.GetAmount() < MINIMUM_SWAP_SIZE {
		swap.CancelMessage = fmt.Sprintf("requiring minimum swap size of %v", MINIMUM_SWAP_SIZE)
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.GetChain(),
			AmountSat:       swap.GetAmount(),
			Type:            swap.GetType(),
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}

	_, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	if swap.GetAsset() != "" && swap.GetAsset() != wallet.GetAsset() {
		swap.CancelMessage = fmt.Sprintf("invalid liquid asset %s", swap.GetAsset())
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.GetChain(),
			AmountSat:       swap.GetAmount(),
			Type:            swap.GetType(),
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}

	if swap.GetNetwork() != "" && swap.GetNetwork() != wallet.GetNetwork() {
		swap.CancelMessage = fmt.Sprintf("invalid bitcoin network %s", swap.GetNetwork())
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.GetChain(),
			AmountSat:       swap.GetAmount(),
			Type:            swap.GetType(),
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}

	if !services.policy.IsPeerAllowed(swap.PeerNodeId) {
		swap.CancelMessage = fmt.Sprintf("peer %s not allowed to request swaps", swap.PeerNodeId)
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.GetChain(),
			AmountSat:       swap.GetAmount(),
			Type:            swap.GetType(),
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}

	// Call next Action
	return a.next.Execute(services, swap)
}

// todo check for policy / balance
// SwapInReceiverInitAction creates the swap-in process
type SwapInReceiverInitAction struct{}

func (s *SwapInReceiverInitAction) Execute(services *SwapServices, swap *SwapData) EventType {
	agreementMessage := &SwapInAgreementMessage{
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swap.GetId(),
		Pubkey:          hex.EncodeToString(swap.GetPrivkey().PubKey().SerializeCompressed()),
		// todo: set premium
		Premium: 0,
	}
	swap.SwapInAgreement = agreementMessage

	nextMessage, nextMessageType, err := MarshalPeerswapMessage(agreementMessage)
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	toCtx, cancel := context.WithCancel(context.Background())
	swap.toCancel = cancel
	services.toService.addNewTimeOut(toCtx, 10*time.Minute, swap.GetId().String())

	return Event_ActionSucceeded
}

func (s *SwapData) HandleError(err error) EventType {
	s.LastErr = err
	if err != nil {
		s.LastErrString = err.Error()
	}
	if s.CancelMessage == "" {
		s.CancelMessage = s.LastErrString
	}
	return Event_ActionFailed
}

// ClaimSwapTransactionWithPreimageAction spends the opening transaction to the nodes liquid wallet
type ClaimSwapTransactionWithPreimageAction struct{}

// todo this is very critical
func (s *ClaimSwapTransactionWithPreimageAction) Execute(services *SwapServices, swap *SwapData) EventType {
	_, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	if swap.ClaimTxId == "" {
		txId, _, err := wallet.CreatePreimageSpendingTransaction(swap.GetOpeningParams(), swap.GetClaimParams())
		if err != nil {
			log.Infof("Error claiming tx with preimage %v", err)
			return Event_OnRetry
		}
		swap.ClaimTxId = txId
	}

	return Event_ActionSucceeded
}

type CreateAndBroadcastOpeningTransaction struct{}

func (c *CreateAndBroadcastOpeningTransaction) Execute(services *SwapServices, swap *SwapData) EventType {
	txWatcher, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	if swap.OpeningTxBroadcasted != nil {
		return Event_ActionSucceeded
	}

	// Generate Preimage
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return swap.HandleError(err)
	}
	swap.ClaimPreimage = hex.EncodeToString(preimage[:])

	// Construct memo
	memo := fmt.Sprintf("peerswap %s %s %s %s", swap.GetChain(), INVOICE_CLAIM, swap.GetScidInBoltFormat(), swap.GetId())
	payreq, err := services.lightning.GetPayreq((swap.GetAmount())*1000, preimage.String(), swap.GetId().String(), memo, INVOICE_CLAIM, swap.GetInvoiceExpiry())
	if err != nil {
		return swap.HandleError(err)
	}

	var blindingKey *btcec.PrivateKey
	var blindingKeyHex string
	if swap.GetChain() == l_btc_chain {
		blindingKey = swap.GetOpeningParams().BlindingKey
		blindingKeyHex = hex.EncodeToString(blindingKey.Serialize())
	}

	// Create the opening transaction
	txHex, _, vout, err := wallet.CreateOpeningTransaction(&OpeningParams{
		TakerPubkey:      swap.GetTakerPubkey(),
		MakerPubkey:      swap.GetMakerPubkey(),
		ClaimPaymentHash: preimage.Hash().String(),
		Amount:           swap.GetAmount(),
		BlindingKey:      blindingKey,
	})
	if err != nil {
		return swap.HandleError(err)
	}

	txId, txHex, err := wallet.BroadcastOpeningTx(txHex)
	if err != nil {
		// todo: idempotent states
		return swap.HandleError(err)
	}
	startingHeight, err := txWatcher.GetBlockHeight()
	if err != nil {
		return swap.HandleError(err)
	}
	swap.StartingBlockHeight = startingHeight

	swap.OpeningTxHex = txHex

	message := &OpeningTxBroadcastedMessage{
		SwapId:      swap.GetId(),
		Payreq:      payreq,
		TxId:        txId,
		ScriptOut:   vout,
		BlindingKey: blindingKeyHex,
	}

	swap.OpeningTxBroadcasted = message

	nextMessage, nextMessageType, err := MarshalPeerswapMessage(message)
	if err != nil {
		return swap.HandleError(err)
	}

	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

type StopSendMessageWithRetryWrapperAction struct {
	next Action
}

func (a StopSendMessageWithRetryWrapperAction) Execute(services *SwapServices, swap *SwapData) EventType {
	// Stop sending repeated messages
	services.messengerManager.RemoveSender(swap.GetId().String())

	// Call next Action
	return a.next.Execute(services, swap)
}

// AwaitPaymentOrCsvAction checks if the invoice has been paid
type AwaitPaymentOrCsvAction struct{}

//todo this will never throw an error
func (w *AwaitPaymentOrCsvAction) Execute(services *SwapServices, swap *SwapData) EventType {
	onchain, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	// invoice payment part
	services.lightning.AddPaymentNotifier(swap.GetId().String(), swap.OpeningTxBroadcasted.Payreq, INVOICE_CLAIM)

	// csv part
	wantScript, err := wallet.GetOutputScript(swap.GetOpeningParams())
	if err != nil {
		return swap.HandleError(err)
	}

	onchain.AddWaitForCsvTx(swap.GetId().String(), swap.OpeningTxBroadcasted.TxId, swap.OpeningTxBroadcasted.ScriptOut, swap.StartingBlockHeight, wantScript)
	return NoOp
}

// AwaitFeeInvoicePayment adds the opening tx to the txwatcher
type AwaitFeeInvoicePayment struct{}

func (w *AwaitFeeInvoicePayment) Execute(services *SwapServices, swap *SwapData) EventType {
	// invoice payment part
	services.lightning.AddPaymentNotifier(swap.GetId().String(), swap.SwapOutAgreement.Payreq, INVOICE_FEE)
	return NoOp
}

// AwaitCsvAction adds the opening tx to the txwatcher
type AwaitCsvAction struct{}

//todo this will never throw an error
func (w *AwaitCsvAction) Execute(services *SwapServices, swap *SwapData) EventType {
	onchain, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	wantScript, err := wallet.GetOutputScript(swap.GetOpeningParams())
	if err != nil {
		return swap.HandleError(err)
	}

	onchain.AddWaitForCsvTx(swap.GetId().String(), swap.OpeningTxBroadcasted.TxId, swap.OpeningTxBroadcasted.ScriptOut, swap.StartingBlockHeight, wantScript)
	return NoOp
}

type SetBlindingKeyActionWrapper struct {
	next Action
}

func (a *SetBlindingKeyActionWrapper) Execute(services *SwapServices, swap *SwapData) EventType {
	// Set the blinding key for opening transaction if we do a liquid swap
	if swap.GetChain() == l_btc_chain {
		blindingKey, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			return swap.HandleError(err)
		}
		swap.BlindingKeyHex = hex.EncodeToString(blindingKey.Serialize())
	}
	return a.next.Execute(services, swap)
}

// CreateSwapOutFromRequestAction creates the swap-out process and prepares the opening transaction
type CreateSwapOutFromRequestAction struct{}

func (c *CreateSwapOutFromRequestAction) Execute(services *SwapServices, swap *SwapData) EventType {
	_, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	// todo replace with premium estimation https://github.com/elementsproject/peerswap/issues/109
	openingFee, err := wallet.GetFlatSwapOutFee()
	if err != nil {
		swap.LastErr = err
		return swap.HandleError(err)
	}

	// Check if onchain balance is sufficient for swap + fees + some safety net
	walletBalance, err := wallet.GetOnchainBalance()
	if err != nil {
		return swap.HandleError(err)
	}

	// TODO: this should be looked at in the future
	safetynet := uint64(20000)

	if walletBalance < swap.GetAmount()+openingFee+safetynet {
		return swap.HandleError(errors.New("insufficient walletbalance"))
	}

	// Construct memo
	memo := fmt.Sprintf("peerswap %s %s %s %s", swap.GetChain(), INVOICE_FEE, swap.GetScidInBoltFormat(), swap.GetId())

	// Generate Preimage
	feepreimage, err := lightning.GetPreimage()
	if err != nil {
		return swap.HandleError(err)
	}
	feeInvoice, err := services.lightning.GetPayreq(openingFee*1000, feepreimage.String(), swap.GetId().String(), memo, INVOICE_FEE, 600)
	if err != nil {
		return swap.HandleError(err)
	}

	message := &SwapOutAgreementMessage{
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swap.GetId(),
		Pubkey:          hex.EncodeToString(swap.GetPrivkey().PubKey().SerializeCompressed()),
		Payreq:          feeInvoice,
	}
	swap.SwapOutAgreement = message

	nextMessage, nextMessageType, err := MarshalPeerswapMessage(message)
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	toCtx, cancel := context.WithCancel(context.Background())
	swap.toCancel = cancel
	services.toService.addNewTimeOut(toCtx, 10*time.Minute, swap.GetId().String())

	return Event_ActionSucceeded
}

// ClaimSwapTransactionWithCsv spends the opening transaction with a signature
type ClaimSwapTransactionWithCsv struct{}

func (c *ClaimSwapTransactionWithCsv) Execute(services *SwapServices, swap *SwapData) EventType {
	_, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		swap.HandleError(err)
		return Event_OnRetry
	}

	if swap.ClaimTxId == "" {
		txId, _, err := wallet.CreateCsvSpendingTransaction(swap.GetOpeningParams(), swap.GetClaimParams())
		if err != nil {
			swap.HandleError(err)
			return Event_OnRetry
		}
		swap.ClaimTxId = txId
	}

	return Event_ActionSucceeded
}

// ClaimSwapTransactionWithCsv spends the opening transaction with maker and taker Signatures
type ClaimSwapTransactionCoop struct{}

func (c *ClaimSwapTransactionCoop) Execute(services *SwapServices, swap *SwapData) EventType {
	_, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	takerKeyBytes, err := hex.DecodeString(swap.CoopClose.Privkey)
	if err != nil {
		return swap.HandleError(err)
	}
	takerKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), takerKeyBytes)

	if swap.ClaimTxId == "" {
		txId, _, err := wallet.CreateCoopSpendingTransaction(swap.GetOpeningParams(), swap.GetClaimParams(), takerKey)
		if err != nil {
			return swap.HandleError(err)
		}
		swap.ClaimTxId = txId
	}

	return Event_ActionSucceeded
}

// SendCancelAction sends a cancel message to the swap peer
type SendCancelAction struct{}

func (s *SendCancelAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.LastErr != nil {
		log.Debugf("[FSM] Canceling because of %s", swap.LastErr.Error())
	}
	messenger := services.messenger

	msgBytes, msgType, err := MarshalPeerswapMessage(&CancelMessage{
		SwapId:  swap.GetId(),
		Message: swap.CancelMessage,
	})
	if err != nil {
		return swap.HandleError(err)
	}

	err = messenger.SendMessage(swap.PeerNodeId, msgBytes, msgType)
	if err != nil {
		return swap.HandleError(err)
	}
	return Event_ActionSucceeded
}

// TakerSendPrivkeyAction builds the sighash to send the maker for cooperatively closing the swap
type TakerSendPrivkeyAction struct{}

func (s *TakerSendPrivkeyAction) Execute(services *SwapServices, swap *SwapData) EventType {
	privkeystring := hex.EncodeToString(swap.PrivkeyBytes)
	nextMessage, nextMessageType, err := MarshalPeerswapMessage(&CoopCloseMessage{
		SwapId:  swap.GetId(),
		Message: swap.CancelMessage,
		Privkey: privkeystring,
	})
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

// SwapInSenderCreateSwapAction creates the swap data
type CreateSwapRequestAction struct{}

//todo validate data
func (a *CreateSwapRequestAction) Execute(services *SwapServices, swap *SwapData) EventType {
	nextMessage, nextMessageType, err := MarshalPeerswapMessage(swap.GetRequest())
	if err != nil {
		return swap.HandleError(err)
	}

	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	toCtx, cancel := context.WithCancel(context.Background())
	swap.toCancel = cancel
	services.toService.addNewTimeOut(toCtx, 10*time.Minute, swap.GetId().String())

	return Event_ActionSucceeded
}

type SendMessageAction struct{}

func (s *SendMessageAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.NextMessage == nil {
		return swap.HandleError(errors.New("swap.NextMessage is nil"))
	}

	err := services.messenger.SendMessage(swap.PeerNodeId, swap.NextMessage, swap.NextMessageType)
	if err != nil {
		return swap.HandleError(err)
	}
	return Event_ActionSucceeded
}

type SendMessageWithRetryAction struct{}

func (s *SendMessageWithRetryAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.NextMessage == nil {
		return swap.HandleError(errors.New("swap.NextMessage is nil"))
	}

	// Send message repeated as we really want the message to be received at some point!
	rm := messages.NewRedundantMessenger(services.messenger, 10*time.Second)
	err := services.messengerManager.AddSender(swap.GetId().String(), rm)
	if err != nil {
		return swap.HandleError(err)
	}
	rm.SendMessage(swap.PeerNodeId, swap.NextMessage, swap.NextMessageType)

	return Event_ActionSucceeded
}

// PayFeeInvoiceAction checks the feeinvoice and pays it
type PayFeeInvoiceAction struct{}

func (r *PayFeeInvoiceAction) Execute(services *SwapServices, swap *SwapData) EventType {
	_, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	ll := services.lightning
	// policy := services.policy
	_, msatAmt, err := ll.DecodePayreq(swap.SwapOutAgreement.Payreq)
	if err != nil {
		return swap.HandleError(err)
	}
	swap.OpeningTxFee = msatAmt / 1000

	expectedFee, err := wallet.GetFlatSwapOutFee()
	if err != nil {
		swap.LastErr = err
		return swap.HandleError(err)
	}

	maxExpected := uint64((float64(expectedFee) * 3))

	// if the fee invoice is larger than what we would expect, don't pay
	if swap.OpeningTxFee > maxExpected {
		return swap.HandleError(errors.New(fmt.Sprintf("Fee is too damn high. Max expected: %v Received %v", maxExpected, swap.OpeningTxFee)))
	}

	preimage, err := ll.PayInvoice(swap.SwapOutAgreement.Payreq)
	if err != nil {
		return swap.HandleError(err)
	}
	swap.FeePreimage = preimage
	return Event_ActionSucceeded
}

// AwaitTxConfirmationAction  checks the claim invoice and adds the transaction
// to the txwatcher.
type AwaitTxConfirmationAction struct{}

//todo this will not ever throw an error
func (t *AwaitTxConfirmationAction) Execute(services *SwapServices, swap *SwapData) EventType {
	txWatcher, wallet, validator, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	// Check if we are outside of our csv safety limit. This can happen on
	// restart. We do NOT want to continue if we fail here.
	if swap.StartingBlockHeight > 0 {
		now, err := txWatcher.GetBlockHeight()
		if err != nil {
			return swap.HandleError(err)
		}

		if now >= swap.StartingBlockHeight+(validator.GetCSVHeight()/2) {
			err := fmt.Errorf("exceeded csv limit")
			return swap.HandleError(err)
		}
	}

	phash, msatAmount, err := services.lightning.DecodePayreq(swap.OpeningTxBroadcasted.Payreq)
	if err != nil {
		return swap.HandleError(err)
	}

	if msatAmount != swap.GetAmount()*1000 {
		return swap.HandleError(fmt.Errorf("invoice amount does not equal swap amount, invoice: %v, swap %v", swap.OpeningTxBroadcasted.Payreq, swap.GetAmount()))
	}

	swap.ClaimPaymentHash = phash

	wantScript, err := wallet.GetOutputScript(swap.GetOpeningParams())
	if err != nil {
		return swap.HandleError(err)
	}

	txWatcher.AddWaitForConfirmationTx(swap.GetId().String(), swap.OpeningTxBroadcasted.TxId, swap.OpeningTxBroadcasted.ScriptOut, swap.StartingBlockHeight, wantScript)
	return NoOp
}

// ValidateTxAndPayClaimInvoiceAction pays the claim invoice
type ValidateTxAndPayClaimInvoiceAction struct{}

func (p *ValidateTxAndPayClaimInvoiceAction) Execute(services *SwapServices, swap *SwapData) EventType {
	lc := services.lightning
	_, _, validator, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	// todo get opening tx hex
	ok, err := validator.ValidateTx(swap.GetOpeningParams(), swap.OpeningTxHex)
	if err != nil {
		return swap.HandleError(err)
	}
	if !ok {
		return swap.HandleError(errors.New("tx is not valid"))
	}

	var retryTime time.Duration = 120 * time.Second
	var interval time.Duration = 10 * time.Second

	if isdev.FastTests() {
		// Retry time should be in [s].
		if prtStr := os.Getenv("PAYMENT_RETRY_TIME"); prtStr != "" {
			prtInt, err := strconv.Atoi(prtStr)
			if err != nil {
				log.Debugf("could not read from PAYMENT_RETRY_TIME")
			} else {
				retryTime = time.Duration(prtInt) * time.Second
			}
		}
		interval = 1 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), retryTime)
	defer cancel()

	var preimage string
	for {
		select {
		case <-ctx.Done():
			return swap.HandleError(fmt.Errorf("could not pay invoice, last err %w", err))
		case <-ticker.C:
			preimage, err = lc.RebalancePayment(swap.OpeningTxBroadcasted.Payreq, swap.GetScid())
			if err != nil {
				log.Infof("error trying to pay invoice: %v, retry...", err)
				// Another round!
				continue
			}

			swap.ClaimPreimage = preimage
			return Event_ActionSucceeded
		}
	}
}

type SetStartingBlockHeightAction struct{}

func (s *SetStartingBlockHeightAction) Execute(services *SwapServices, swap *SwapData) EventType {
	onchain, _, validator, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		swap.LastErr = err
		return Event_ActionFailed
	}

	now, err := onchain.GetBlockHeight()
	if err != nil {
		swap.LastErr = err
		return Event_ActionFailed
	}

	// Check if we already set a Blockheight and set if not. This check will
	// leave the starting block height untouched in case of a restart. In the
	// case of a restart we check if we already exceeded the csv limit.
	if swap.StartingBlockHeight == 0 {
		swap.StartingBlockHeight = now
	} else if now >= swap.StartingBlockHeight+(validator.GetCSVHeight()/2) {
		swap.LastErr = fmt.Errorf("too close to csv")
		swap.CancelMessage = swap.LastErr.Error()
		return Event_ActionFailed
	}

	return NoOp
}

type NoOpAction struct{}

func (n *NoOpAction) Execute(services *SwapServices, swap *SwapData) EventType {
	return NoOp
}

type NoOpDoneAction struct{}

func (a *NoOpDoneAction) Execute(services *SwapServices, swap *SwapData) EventType {
	// Remove possible message sender
	services.messengerManager.RemoveSender(swap.GetId().String())

	return Event_Done
}

type CancelAction struct{}

func (c *CancelAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.LastErr != nil {
		swap.LastErrString = swap.LastErr.Error()
	}

	return Event_Done
}
