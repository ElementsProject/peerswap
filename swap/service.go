package swap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	LOCKTIME = 100
)
type InitializeSwapRequest struct {
	SwapId string
	Type SwapType
	Amount uint64
}

type FeeInvoice struct {
	SwapId string
	Invoice string
}

type OpenTransactionMessage struct {
	SwapId string
	TxHex string
}

type AbortMessage struct {
	SwapId string
	TxId string
}

type ClaimedMessage struct {
	SwapId string
	TxId string
}

type Swapper interface {
	PostOpenFundingTx(ctx context.Context, amt int64, fee int64) (txId string, err error)

}

type TxBuilder interface {
}



type Wallet interface {
	GetBalance() (uint64, error)
	GetPubkey() (*btcec.PublicKey, error)
	GetPrivKey() (*btcec.PrivateKey, error)
	GetUtxos(amount uint64) ([]*transaction.TxInput,uint64, error)
}

type SwapStore interface {
	Create(context.Context, *Swap) error
	Update(context.Context, *Swap) error
	DeleteById(context.Context, string) error
	GetById(context.Context, string)  (*Swap, error)
	ListAll(context.Context) ([]*Swap, error)
}

type Service struct {
	store SwapStore
	wallet Wallet
	blockchain wallet.BlockchainService

	network *network.Network
	asset []byte
}

// CreateOpeningTransaction creates and broadcasts the opening Transaction,
// the two peers are the taker(pays the invoice) and the maker
func (s *Service) CreateOpeningTransaction(ctx context.Context, takerPubkeyString string, amount uint64) error{
	// Create the preimage for claiming the swap
	var preimage lightning.Preimage
	if _, err := rand.Read(preimage[:]); err != nil {
		return err
	}
	pHash := preimage.Hash()


	// get the maker pubkey and privkey
	makerPubkey, err := s.wallet.GetPubkey()
	if err != nil {
		return err
	}
	makerPrivkey, err := s.wallet.GetPrivKey()
	if err != nil {
		return err
	}

	// get the taker pubkey
	takerPubkey, err := btcec.ParsePubKey([]byte(takerPubkeyString), btcec.S256())
	if err != nil {
		return err
	}

	// Get the Inputs
	txInputs, change, err := s.wallet.GetUtxos(amount + FIXED_FEE)
	if err != nil {
		return err
	}

	// Outputs
	// Fees
	feeOutput,err := liquid.GetFeeOutput(FIXED_FEE)
	if err != nil {
		return err
	}

	// Change
	p2pkh := payment.FromPublicKey(makerPubkey, &network.Regtest, nil)
	changeScript := p2pkh.Script
	changeValue, err := elementsutil.SatoshiToElementsValue(change)
	if err != nil {
		return err
	}
	changeOutput := transaction.NewTxOutput(s.asset, changeValue[:], changeScript)

	// Swap
	// calc cltv

	blockHeight, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return err
	}
	spendingBlockHeight := int64(blockHeight + LOCKTIME)

	redeemScript, err := liquid.GetOpeningTxScript(takerPubkey,makerPubkey, pHash[:], spendingBlockHeight)
	if err != nil {
		return err
	}
	redeemPayment, err := payment.FromPayment(&payment.Payment{
		Script:  redeemScript,
		Network: s.network,
	})
	if err != nil {
		return err
	}

	loopInValue, err := elementsutil.SatoshiToElementsValue(amount)
	if err != nil {
		return err
	}

	redeemOutput := transaction.NewTxOutput(s.asset, loopInValue, redeemPayment.WitnessScript)

	// Create a new pset
	inputs := txInputs
	outputs := []*transaction.TxOutput{redeemOutput, changeOutput, feeOutput}
	p, err := pset.New(inputs, outputs, 2, 0)
	if err != nil {
		return err
	}

	// Add sighash type and witness utxo to the partial input.
	updater, err := pset.NewUpdater(p)
	if err != nil {
		return err
	}

	bobspendingTxHash, err := s.blockchain.FetchTxHex(b2h(inputs[0].Hash))
	if err != nil {
		return err
	}
	bobFaucetTx, _ := transaction.NewTxFromHex(bobspendingTxHash)

	err = updater.AddInNonWitnessUtxo(bobFaucetTx, 0)
	if err != nil {
		return err
	}

	prvKeys := []*btcec.PrivateKey{makerPrivkey}
	scripts := [][]byte{p2pkh.Script}
	if err := liquid.SignTransaction(p, prvKeys, scripts, false, nil); err != nil {
		return err
	}

	// Finalize the partial transaction.
	if err := pset.FinalizeAll(p); err != nil {
		return err
	}
	// Extract the final signed transaction from the Pset wrapper.
	finalTx, err := pset.Extract(p)
	if err != nil {
		return err
	}
	// Serialize the transaction and try to broadcast.
	txHex, err := finalTx.ToHex()
	if err != nil {
		return err
	}
	_, err = s.blockchain.BroadcastTransaction(txHex)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) ClaimTxWithPreimage(ctx context.Context, preImage, redeemScript []byte, txHex string) error{
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

	feeOutput,err := liquid.GetFeeOutput(FIXED_FEE)
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
	spendingTx.Inputs[0].Witness =  witness


	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		return err
	}

	_, err = s.blockchain.BroadcastTransaction(spendingTxHex)
	if err != nil  {
		return err
	}
	return nil
}
func b2h(buf []byte) string {
	return hex.EncodeToString(buf)
}
