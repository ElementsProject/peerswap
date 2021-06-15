package swap

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/txscript"
	"github.com/sputn1ck/sugarmama/lightning"
	"github.com/sputn1ck/sugarmama/liquid"
	"github.com/sputn1ck/sugarmama/wallet"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/pset"
	"github.com/vulpemventures/go-elements/transaction"
	"log"
	"time"
)

const (
	FIXED_FEE = 100
	LOCKTIME  = 100
)

type TxBuilder interface {
}

type Wallet interface {
	GetBalance() (uint64, error)
	GetPubkey() (*btcec.PublicKey, error)
	GetPrivKey() (*btcec.PrivateKey, error)
	GetUtxos(amount uint64) ([]*wallet.Utxo, uint64, error)
}

type SwapStore interface {
	Create(*Swap) error
	Update(*Swap) error
	DeleteById(string) error
	GetById(string) (*Swap, error)
	ListAll() ([]*Swap, error)
}

func (s *Service) getAsset() []byte {
	return append(
		[]byte{0x01},
		elementsutil.ReverseBytes(h2b(s.network.AssetID))...,
	)
}

type Service struct {
	store      SwapStore
	wallet     Wallet
	pc         lightning.PeerCommunicator
	blockchain wallet.BlockchainService
	lightning  LightningClient
	network    *network.Network
	txWatcher  *txWatcher

	ctx context.Context
}

func NewService(ctx context.Context, store SwapStore, wallet Wallet, pc lightning.PeerCommunicator, blockchain wallet.BlockchainService, lightning LightningClient, network *network.Network) *Service {
	service := &Service{
		store:      store,
		wallet:     wallet,
		pc:         pc,
		blockchain: blockchain,
		lightning:  lightning,
		network:    network,
		ctx:        ctx}
	watchList := newTxWatcher(ctx, blockchain, service.swapCallback)
	service.txWatcher = watchList
	return service
}

func (s *Service) ListSwaps() ([]*Swap, error) {
	return s.store.ListAll()
}

func (s *Service) StartSwapOut(peerNodeId string, channelId string, amount uint64) error {

	swap := NewSwap(SWAPTYPE_OUT, amount, s.lightning.GetNodeId(), peerNodeId, channelId)
	err := s.store.Create(swap)
	if err != nil {
		return err
	}
	pubkey, err := s.wallet.GetPubkey()
	if err != nil {
		return err
	}
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	request := &SwapRequest{
		SwapId:          swap.Id,
		ChannelId:       channelId,
		Amount:          amount,
		Type:            SWAPTYPE_OUT,
		TakerPubkeyHash: swap.TakerPubkeyHash,
	}
	err = s.pc.SendMessage(peerNodeId, request)
	if err != nil {
		return err
	}
	swap.State = SWAPSTATE_REQUEST_SENT
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	return nil
}
func (s *Service) StartSwapIn(peerNodeId string, channelId string, amount uint64) error {
	swap := NewSwap(SWAPTYPE_IN, amount, s.lightning.GetNodeId(), peerNodeId, channelId)
	err := s.store.Create(swap)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	request := &SwapRequest{
		SwapId:          swap.Id,
		ChannelId:       channelId,
		Amount:          amount,
		Type:            SWAPTYPE_IN,
		TakerPubkeyHash: "",
	}
	err = s.pc.SendMessage(peerNodeId, request)
	if err != nil {
		return err
	}
	swap.State = SWAPSTATE_REQUEST_SENT
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	return nil
}

// todo implement swap in
func (s *Service) OnSwapRequest(senderNodeId string, request SwapRequest) error {
	ctx := context.Background()
	swap := &Swap{
		Id:              request.SwapId,
		Type:            request.Type,
		State:           SWAPSTATE_REQUEST_RECEIVED,
		PeerNodeId:      senderNodeId,
		InitiatorNodeId: senderNodeId,
		Amount:          request.Amount,
		ChannelId:       request.ChannelId,
	}

	err := s.store.Create(swap)
	if err != nil {
		return err
	}

	pubkey, err := s.wallet.GetPubkey()
	if err != nil {
		return err
	}

	// requester wants to swap out, meaning responder is the maker
	if request.Type == SWAPTYPE_OUT {
		swap.TakerPubkeyHash = request.TakerPubkeyHash
		swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
		// Generate Preimage
		preimage, err := s.lightning.GetPreimage()
		if err != nil {
			return err
		}
		pHash := preimage.Hash()
		log.Printf("maker preimage: %s ", preimage.String())
		payreq, err := s.lightning.GetPayreq((request.Amount+FIXED_FEE)*1000, preimage.String(), swap.Id)
		if err != nil {
			return err
		}

		swap.Payreq = payreq
		swap.PreImage = preimage.String()
		swap.PHash = pHash.String()
		swap.State = SWAPSTATE_OPENING_TX_PREPARED
		err = s.store.Update(swap)
		if err != nil {
			return err
		}
		txId, err := s.CreateOpeningTransaction(ctx, swap)
		if err != nil {
			return err
		}
		swap.OpeningTxId = txId
		swap.State = SWAPSTATE_OPENING_TX_BROADCASTED
		err = s.store.Update(swap)
		if err != nil {
			return err
		}
		response := &MakerResponse{
			SwapId:          swap.Id,
			MakerPubkeyHash: swap.MakerPubkeyHash,
			Invoice:         payreq,
			TxId:            swap.OpeningTxId,
			Cltv:            swap.Cltv,
		}
		err = s.pc.SendMessage(swap.PeerNodeId, response)
		if err != nil {
			return err
		}
	} else if request.Type == SWAPTYPE_IN {
		swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
		err = s.store.Update(swap)
		if err != nil {
			return err
		}
		response := &TakerResponse{
			SwapId:          swap.Id,
			TakerPubkeyHash: hex.EncodeToString(pubkey.SerializeCompressed()),
		}
		err = s.pc.SendMessage(swap.PeerNodeId, response)
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateOpeningTransaction creates and broadcasts the opening Transaction,
// the two peers are the taker(pays the invoice) and the maker
func (s *Service) CreateOpeningTransaction(ctx context.Context, swap *Swap) (string, error) {
	time.Sleep(5 * time.Second)
	// get the maker privkey
	makerPrivkey, err := s.wallet.GetPrivKey()
	if err != nil {
		return "", err
	}
	makerPubkey := makerPrivkey.PubKey()
	p2pkh := payment.FromPublicKey(makerPubkey, s.network, nil)

	// Get the Inputs
	txInputs, change, err := s.wallet.GetUtxos(swap.Amount + FIXED_FEE)
	if err != nil {
		return "", err
	}
	// Outputs
	// Fees
	feeOutput, err := liquid.GetFeeOutput(FIXED_FEE, s.network)
	if err != nil {
		return "", err
	}

	// Change
	changeScript := p2pkh.Script
	changeValue, err := elementsutil.SatoshiToElementsValue(change)
	if err != nil {
		return "", err
	}
	changeOutput := transaction.NewTxOutput(s.getAsset(), changeValue[:], changeScript)

	// Swap
	blockHeight, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return "", err
	}
	spendingBlocktimeHeight := int64(blockHeight + LOCKTIME)
	swap.Cltv = spendingBlocktimeHeight
	redeemScript, err := s.getSwapScript(swap)
	redeemPayment, err := payment.FromPayment(&payment.Payment{
		Script:  redeemScript,
		Network: s.network,
	})
	if err != nil {
		return "", err
	}

	swapInValue, err := elementsutil.SatoshiToElementsValue(swap.Amount)
	if err != nil {
		return "", err
	}

	log.Printf("inputs: %v change: %v swapAmount: %v", txInputs, change, swap.Amount)
	redeemOutput := transaction.NewTxOutput(s.getAsset(), swapInValue, redeemPayment.WitnessScript)

	// Create a new pset
	inputs, err := s.blockchain.WalletUtxosToTxInputs(txInputs)
	if err != nil {
		return "", err
	}
	outputs := []*transaction.TxOutput{redeemOutput, changeOutput, feeOutput}
	p, err := pset.New(inputs, outputs, 2, 0)
	if err != nil {
		return "", err
	}
	// Add sighash type and witness utxo to the partial input.
	updater, err := pset.NewUpdater(p)
	if err != nil {
		return "", err
	}

	bobspendingTxHash, err := s.blockchain.FetchTxHex(b2h(elementsutil.ReverseBytes(inputs[0].Hash[:])))
	if err != nil {
		return "", err
	}

	bobFaucetTx, err := transaction.NewTxFromHex(bobspendingTxHash)
	if err != nil {
		return "", err
	}

	err = updater.AddInNonWitnessUtxo(bobFaucetTx, 0)
	if err != nil {
		return "", err
	}

	prvKeys := []*btcec.PrivateKey{makerPrivkey}
	scripts := [][]byte{p2pkh.Script}

	if err := liquid.SignTransaction(p, prvKeys, scripts[:], false, nil); err != nil {
		return "", err
	}

	// Finalize the partial transaction.
	if err := pset.FinalizeAll(p); err != nil {
		return "", err
	}

	// Extract the final signed transaction from the Pset wrapper.

	finalTx, err := pset.Extract(p)
	if err != nil {
		return "", err
	}
	// Serialize the transaction and try to broadcast.
	txHex, err := finalTx.ToHex()
	if err != nil {
		return "", err
	}

	txId, err := s.blockchain.BroadcastTransaction(txHex)
	if err != nil {
		return "", err
	}

	return txId, nil
}
func (s *Service) OnTakerResponse(senderNodeId string, request TakerResponse) error {
	swap, err := s.store.GetById(request.SwapId)
	if err != nil {
		return err
	}
	if swap.PeerNodeId != senderNodeId {
		return errors.New("peer has changed, aborting")
	}

	pubkey, err := s.wallet.GetPubkey()
	if err != nil {
		return err
	}

	swap.TakerPubkeyHash = request.TakerPubkeyHash
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	preimage, err := s.lightning.GetPreimage()
	if err != nil {
		return err
	}
	pHash := preimage.Hash()
	log.Printf("maker preimage: %s ", preimage.String())
	payreq, err := s.lightning.GetPayreq((swap.Amount+FIXED_FEE)*1000, preimage.String(), swap.Id)
	if err != nil {
		return err
	}

	swap.Payreq = payreq
	swap.PreImage = preimage.String()
	swap.PHash = pHash.String()
	swap.State = SWAPSTATE_OPENING_TX_PREPARED
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	txId, err := s.CreateOpeningTransaction(context.Background(), swap)
	if err != nil {
		return err
	}
	swap.OpeningTxId = txId
	swap.State = SWAPSTATE_OPENING_TX_BROADCASTED
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	response := &MakerResponse{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         payreq,
		TxId:            swap.OpeningTxId,
		Cltv:            swap.Cltv,
	}
	err = s.pc.SendMessage(swap.PeerNodeId, response)
	if err != nil {
		return err
	}
	return nil
}
func (s *Service) OnMakerResponse(senderNodeId string, request MakerResponse) error {
	swap, err := s.store.GetById(request.SwapId)
	if err != nil {
		return err
	}
	if swap.PeerNodeId != senderNodeId {
		return errors.New("peer has changed, aborting")
	}
	swap.State = SWAPSTATE_WAITING_FOR_TX
	swap.MakerPubkeyHash = request.MakerPubkeyHash
	swap.Payreq = request.Invoice
	swap.OpeningTxId = request.TxId
	swap.Cltv = request.Cltv

	invoice, err := s.lightning.DecodePayreq(swap.Payreq)
	if err != nil {
		return err
	}

	swap.PHash = invoice.PHash

	if invoice.Amount > (swap.Amount+FIXED_FEE)*1000 {
		return errors.New(fmt.Sprintf("invoice amount is to high, got: %v, expected %v", swap.Amount+FIXED_FEE, invoice.Amount))
	}

	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	s.txWatcher.AddTx(swap.Id, swap.OpeningTxId)
	return nil
}

func (s *Service) OnClaimedResponse(senderNodeId string, request ClaimedMessage) error {
	swap, err := s.store.GetById(request.SwapId)
	if err != nil {
		return err
	}
	swap.State = SwapState(int(SWAPSTATE_CLAIMED_PREIMAGE) + int(request.ClaimType))
	swap.ClaimTxId = request.ClaimTxId
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) StartWatchingTxs() error {
	swaps, err := s.store.ListAll()
	if err != nil {
		return err
	}
	err = s.txWatcher.StartWatchingTxs(swaps)
	if err != nil {
		return err
	}
	return nil
}
func (s *Service) ClaimTxWithPreimage(ctx context.Context, swap *Swap, tx *transaction.Transaction) error {
	err := s.CheckTransaction(ctx, swap, tx)
	if err != nil {
		return err
	}
	if swap.PreImage == "" {
		preimageString, err := s.lightning.PayInvoice(swap.Payreq)
		if err != nil {
			return err
		}
		swap.PreImage = preimageString
		err = s.store.Update(swap)
		if err != nil {
			return err
		}
	}
	preimage, err := lightning.MakePreimageFromStr(swap.PreImage)
	if err != nil {
		return err
	}
	script, err := s.getSwapScript(swap)
	if err != nil {
		return err
	}

	// get the maker pubkey and privkey
	pubkey, err := s.wallet.GetPubkey()
	if err != nil {
		return err
	}
	privkey, err := s.wallet.GetPrivKey()
	if err != nil {
		return err
	}

	// Change
	p2pkh := payment.FromPublicKey(pubkey, s.network, nil)

	// second transaction
	firstTxHash := tx.WitnessHash()
	log.Printf("taker opening txhash %s", tx.WitnessHash().String())
	spendingInput := transaction.NewTxInput(firstTxHash[:], 0)
	firstTxSats, err := elementsutil.ElementsToSatoshiValue(tx.Outputs[0].Value)
	if err != nil {
		return err
	}
	log.Printf("first sats: %v", firstTxSats)
	spendingSatsBytes, err := elementsutil.SatoshiToElementsValue(firstTxSats - FIXED_FEE)
	if err != nil {
		return err
	}
	spendingOutput := transaction.NewTxOutput(s.getAsset(), spendingSatsBytes, p2pkh.Script)

	feeOutput, err := liquid.GetFeeOutput(FIXED_FEE, s.network)
	if err != nil {
		return err
	}

	spendingTx := &transaction.Transaction{
		Version:  2,
		Flag:     0,
		Locktime: 0,
		Inputs:   []*transaction.TxInput{spendingInput},
		Outputs:  []*transaction.TxOutput{spendingOutput, feeOutput},
	}

	var sigHash [32]byte

	sigHash = spendingTx.HashForWitnessV0(
		0,
		script[:],
		tx.Outputs[0].Value,
		txscript.SigHashAll,
	)
	log.Printf("taker sighash: %s", hex.EncodeToString(sigHash[:]))
	sig, err := privkey.Sign(sigHash[:])
	if err != nil {
		return err
	}
	sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))

	log.Printf("taker preimage %s", swap.PreImage)

	witness := make([][]byte, 0)
	witness = append(witness, preimage[:])
	witness = append(witness, sigWithHashType[:])
	witness = append(witness, script[:])
	spendingTx.Inputs[0].Witness = witness

	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		return err
	}

	claimId, err := s.blockchain.BroadcastTransaction(spendingTxHex)
	if err != nil {
		return err
	}
	swap.ClaimTxId = claimId
	swap.State = SWAPSTATE_CLAIMED_PREIMAGE

	log.Printf("taker claimid %s", claimId)
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) swapCallback(swapId string, tx *transaction.Transaction) error {
	swap, err := s.store.GetById(swapId)
	if err != nil {
		return err
	}
	err = s.ClaimTxWithPreimage(s.ctx, swap, tx)
	if err != nil {
		return err
	}
	swap.State = SWAPSTATE_CLAIMED_PREIMAGE
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	claimedMessage := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_PREIMAGE,
		ClaimTxId: swap.ClaimTxId,
	}
	err = s.pc.SendMessage(swap.PeerNodeId, claimedMessage)
	if err != nil {
		return err
	}
	return nil
}

// CheckTransaction checks if the opening transaction is according to the takers terms
func (s *Service) CheckTransaction(ctx context.Context, swap *Swap, tx *transaction.Transaction) error {
	// check value
	value, err := elementsutil.SatoshiToElementsValue(swap.Amount)
	if err != nil {
		return err
	}
	if bytes.Compare(tx.Outputs[0].Value, value) != 0 {
		return errors.New("tx value does not match contract")
	}
	return nil
}

func (s *Service) getSwapScript(swap *Swap) ([]byte, error) {
	// check script
	takerPubkeyHashBytes, err := hex.DecodeString(swap.TakerPubkeyHash)
	if err != nil {
		return nil, err
	}
	makerPubkeyHashBytes, err := hex.DecodeString(swap.MakerPubkeyHash)
	if err != nil {
		return nil, err
	}
	pHashBytes, err := hex.DecodeString(swap.PHash)
	if err != nil {
		return nil, err
	}
	script, err := liquid.GetOpeningTxScript(takerPubkeyHashBytes, makerPubkeyHashBytes, pHashBytes, swap.Cltv)
	if err != nil {
		return nil, err
	}
	log.Printf("\n scriptvals: %s %s %s %v \nscripthex: %s", swap.TakerPubkeyHash, swap.MakerPubkeyHash, swap.PHash, swap.Cltv, hex.EncodeToString(script))
	return script, nil
}
func b2h(buf []byte) string {
	return hex.EncodeToString(buf)
}
func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}
