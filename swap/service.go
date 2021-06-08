package swap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/txscript"
	"github.com/sputn1ck/liquid-loop/lightning"
	"github.com/sputn1ck/liquid-loop/liquid"
	"github.com/sputn1ck/liquid-loop/wallet"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/pset"
	"github.com/vulpemventures/go-elements/transaction"
)

const (
	FIXED_FEE = 500
	LOCKTIME  = 100
)

type Swapper interface {
	PostOpenFundingTx(ctx context.Context, amt int64, fee int64) (txId string, err error)
}

type TxBuilder interface {
}

type Wallet interface {
	GetBalance() (uint64, error)
	GetPubkey() (*btcec.PublicKey, error)
	GetPrivKey() (*btcec.PrivateKey, error)
	GetUtxos(amount uint64) ([]*transaction.TxInput, uint64, error)
}

type SwapStore interface {
	Create(context.Context, *Swap) error
	Update(context.Context, *Swap) error
	DeleteById(context.Context, string) error
	GetById(context.Context, string) (*Swap, error)
	ListAll(context.Context) ([]*Swap, error)
}

type Service struct {
	store      SwapStore
	wallet     Wallet
	pc         lightning.PeerCommunicator
	blockchain wallet.BlockchainService
	lightning  LightningClient
	network    *network.Network
	asset      []byte

	ctx context.Context
}

func (s *Service) StartSwap(swapType SwapType, peerNodeId string, channelId string, amount uint64) error {
	swap := NewSwap(swapType, amount, peerNodeId, channelId)
	err := s.store.Create(s.ctx, swap)
	if err != nil {
		return err
	}
	pubkey, err := s.wallet.GetPubkey()
	if err != nil {
		return err
	}
	request := &SwapRequest{
		SwapId:          swap.Id,
		ChannelId:       channelId,
		Amount:          amount,
		Type:            swapType,
		TakerPubkeyHash: hex.EncodeToString(pubkey.SerializeCompressed()),
	}
	req, err := json.Marshal(request)
	if err != nil {
		return err
	}
	err = s.pc.SendMessage(peerNodeId, req)
	if err != nil {
		return err
	}
	swap.State = SWAPSTATE_REQUEST_SENT
	err = s.store.Update(s.ctx, swap)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) OnSwapRequest(senderNodeId string, request SwapRequest) error {
	swap := &Swap{
		Id:         request.SwapId,
		Type:       request.Type,
		State:      SWAPSTATE_REQUEST_RECEIVED,
		PeerNodeId: senderNodeId,
		Amount:     request.Amount,
		ChannelId:  request.ChannelId,
	}

	err := s.store.Create(s.ctx, swap)
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
		var preimage lightning.Preimage

		if _, err = rand.Read(preimage[:]); err != nil {
			return err
		}
		pHash := preimage.Hash()

		payreq, err := s.lightning.GetPayreq(request.Amount+FIXED_FEE, preimage.String(), pHash.String())
		if err != nil {
			return err
		}

		swap.Payreq = payreq
		swap.PHash = pHash[:]
		swap.State = SWAPSTATE_OPENING_TX_PREPARED
		err = s.store.Update(s.ctx, swap)
		if err != nil {
			return err
		}

	} else if request.Type == SWAPTYPE_IN {

	}
	return nil
}

// CreateOpeningTransaction creates and broadcasts the opening Transaction,
// the two peers are the taker(pays the invoice) and the maker
func (s *Service) CreateOpeningTransaction(ctx context.Context, swap *Swap) (string, error) {

	// get the maker pubkey and privkey
	makerPubkey, err := s.wallet.GetPubkey()
	if err != nil {
		return "", err
	}
	makerPrivkey, err := s.wallet.GetPrivKey()
	if err != nil {
		return "", err
	}

	// Get the Inputs
	txInputs, change, err := s.wallet.GetUtxos(swap.Amount + FIXED_FEE)
	if err != nil {
		return "", err
	}

	// Outputs
	// Fees
	feeOutput, err := liquid.GetFeeOutput(FIXED_FEE)
	if err != nil {
		return "", err
	}

	// Change
	p2pkh := payment.FromPublicKey(makerPubkey, &network.Regtest, nil)
	changeScript := p2pkh.Script
	changeValue, err := elementsutil.SatoshiToElementsValue(change)
	if err != nil {
		return "", err
	}
	changeOutput := transaction.NewTxOutput(s.asset, changeValue[:], changeScript)

	// Swap
	// calc cltv

	blockHeight, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return "", err
	}
	spendingBlockHeight := int64(blockHeight + LOCKTIME)

	takerPubkeyHashBytes, err := hex.DecodeString(swap.TakerPubkeyHash)
	if err != nil {
		return "", err
	}
	makerPubkeyHashBytes, err := hex.DecodeString(swap.MakerPubkeyHash)
	if err != nil {
		return "", err
	}
	redeemScript, err := liquid.GetOpeningTxScript(takerPubkeyHashBytes, makerPubkeyHashBytes, swap.PHash[:], spendingBlockHeight)
	if err != nil {
		return "", err
	}
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

	redeemOutput := transaction.NewTxOutput(s.asset, swapInValue, redeemPayment.WitnessScript)

	// Create a new pset
	inputs := txInputs
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

	bobspendingTxHash, err := s.blockchain.FetchTxHex(b2h(inputs[0].Hash))
	if err != nil {
		return "", err
	}
	bobFaucetTx, _ := transaction.NewTxFromHex(bobspendingTxHash)

	err = updater.AddInNonWitnessUtxo(bobFaucetTx, 0)
	if err != nil {
		return "", err
	}

	prvKeys := []*btcec.PrivateKey{makerPrivkey}
	scripts := [][]byte{p2pkh.Script}
	if err := liquid.SignTransaction(p, prvKeys, scripts, false, nil); err != nil {
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

func (s *Service) ClaimTxWithPreimage(ctx context.Context, preImage, redeemScript []byte, txHex string) error {
	finalTx, err := transaction.NewTxFromHex(txHex)
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
	p2pkh := payment.FromPublicKey(pubkey, &network.Regtest, nil)

	// second transaction
	firstTxHash := finalTx.WitnessHash()
	spendingInput := transaction.NewTxInput(firstTxHash[:], 0)
	firstTxSats, err := elementsutil.ElementsToSatoshiValue(finalTx.Outputs[0].Value)
	if err != nil {
		return err
	}
	spendingSatsBytes, err := elementsutil.SatoshiToElementsValue(firstTxSats - FIXED_FEE)
	if err != nil {
		return err
	}
	spendingOutput := transaction.NewTxOutput(s.asset, spendingSatsBytes[:], p2pkh.Script)

	feeOutput, err := liquid.GetFeeOutput(FIXED_FEE)
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
		redeemScript[:],
		finalTx.Outputs[0].Value,
		txscript.SigHashAll,
	)

	sig, err := privkey.Sign(sigHash[:])
	if err != nil {
		return err
	}
	sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))
	witness := make([][]byte, 0)

	witness = append(witness, sigWithHashType[:])
	witness = append(witness, []byte{})
	witness = append(witness, redeemScript)
	spendingTx.Inputs[0].Witness = witness

	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		return err
	}

	_, err = s.blockchain.BroadcastTransaction(spendingTxHex)
	if err != nil {
		return err
	}
	return nil
}
func b2h(buf []byte) string {
	return hex.EncodeToString(buf)
}
