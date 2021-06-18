package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/txscript"
	address2 "github.com/vulpemventures/go-elements/address"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/transaction"
	"log"
)

type TxSigner interface {
	Sign(hash []byte) (*btcec.Signature, error)
}

func getAsset(network *network.Network) []byte {
	return append(
		[]byte{0x01},
		elementsutil.ReverseBytes(h2b(network.AssetID))...,
	)
}
func GetFeeOutput(fee uint64, network *network.Network) (*transaction.TxOutput, error) {
	feeValue, err := elementsutil.SatoshiToElementsValue(fee)
	if err != nil {
		return nil, err
	}
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(getAsset(network), feeValue, feeScript)
	return feeOutput, nil
}

// GetOpeningTxScript returns the script for the opening transaction of a swap,
// where the taker is the peer paying the invoice and the maker the peer providing the lbtc
func GetOpeningTxScript(takerPubkeyHash []byte, makerPubkeyHash []byte, pHash []byte, locktimeHeight int64) ([]byte, error) {
	script := txscript.NewScriptBuilder().
		AddData(takerPubkeyHash).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_NOTIF).
		AddData(makerPubkeyHash).
		AddOp(txscript.OP_CHECKSIGVERIFY).
		AddInt64(locktimeHeight).
		AddOp(txscript.OP_CHECKLOCKTIMEVERIFY).
		AddOp(txscript.OP_ELSE).
		AddOp(txscript.OP_SIZE).
		AddData(h2b("20")).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_SHA256).
		AddData(pHash[:]).
		AddOp(txscript.OP_EQUAL).
		AddOp(txscript.OP_ENDIF)
	return script.Script()
}

func CreateOpeningAddress(redeemScript []byte) (string, error) {
	scriptPubKey := []byte{0x00, 0x20}
	witnessProgram := sha256.Sum256(redeemScript)
	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)

	redeemPayment, err := payment.FromScript(scriptPubKey, &network.Regtest, nil)
	if err != nil {
		return "", err
	}
	addr, err := redeemPayment.WitnessScriptHash()
	if err != nil {
		return "", err
	}
	return addr, nil
}

type SpendingParams struct {
	Signer       TxSigner
	OpeningTxHex string
	SwapAmount   uint64
	FeeAmount    uint64
	CurrentBlock uint64
	Asset        []byte
	OutputScript []byte
	RedeemScript []byte
}

func CreatePreimageSpendingTransaction(params *SpendingParams, preimage []byte) (string, error) {
	spendingTx, sigHash, err := createSpendingTransaction(params.OpeningTxHex, params.SwapAmount, params.FeeAmount, params.CurrentBlock, params.Asset, params.RedeemScript, params.OutputScript)
	if err != nil {
		return "", err
	}

	sig, err := params.Signer.Sign(sigHash[:])
	if err != nil {
		return "", err
	}

	spendingTx.Inputs[0].Witness = getPreimageWitness(sig.Serialize(), preimage, params.RedeemScript)

	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		return "", err
	}
	return spendingTxHex, nil
}

func CreateCltvSpendingTransaction(params *SpendingParams) (string, error) {
	paramBytes, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	log.Printf("params: %s", string(paramBytes))
	spendingTx, sigHash, err := createSpendingTransaction(params.OpeningTxHex, params.SwapAmount, params.FeeAmount, params.CurrentBlock, params.Asset, params.RedeemScript, params.OutputScript)
	if err != nil {
		return "", err
	}

	sig, err := params.Signer.Sign(sigHash[:])
	if err != nil {
		return "", err
	}

	spendingTx.Inputs[0].Witness = getCtlvWitness(sig.Serialize(), params.RedeemScript)

	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		return "", err
	}
	return spendingTxHex, nil
}

func VoutFromTxHex(txHex string, redeemScript []byte) (uint32, error) {

	tx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		return 0, err
	}
	vout, err := findVout(tx.Outputs, redeemScript)
	if err != nil {
		return 0, err
	}
	return vout, nil
}

func findVout(outputs []*transaction.TxOutput, redeemScript []byte) (uint32, error) {
	wantAddr, err := CreateOpeningAddress(redeemScript)
	if err != nil {
		return 0, err
	}
	wantBytes, err := address2.ToOutputScript(wantAddr)
	if err != nil {
		return 0, err
	}
	for i, v := range outputs {
		if bytes.Compare(v.Script, wantBytes) == 0 {
			return uint32(i), nil
		}
	}
	return 0, errors.New("vout not found")
}

func createSpendingTransaction(openingTxHex string, swapAmount, feeAmount, currentBlock uint64, asset, redeemScript, outputScript []byte) (tx *transaction.Transaction, sigHash [32]byte, err error) {
	firstTx, err := transaction.NewTxFromHex(openingTxHex)
	if err != nil {
		return nil, [32]byte{}, err
	}

	swapInValue, err := elementsutil.SatoshiToElementsValue(swapAmount)
	if err != nil {
		return nil, [32]byte{}, err
	}
	vout, err := findVout(firstTx.Outputs, redeemScript)
	if err != nil {
		return nil, [32]byte{}, err
	}

	txHash := firstTx.TxHash()
	spendingInput := transaction.NewTxInput(txHash[:], vout)
	spendingInput.Sequence = 0
	spendingSatsBytes, _ := elementsutil.SatoshiToElementsValue(swapAmount - feeAmount)

	spendingOutput := transaction.NewTxOutput(asset, spendingSatsBytes[:], outputScript)

	feeValue, _ := elementsutil.SatoshiToElementsValue(feeAmount)
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(asset, feeValue, feeScript)

	spendingTx := &transaction.Transaction{
		Version:  2,
		Flag:     0,
		Locktime: uint32(currentBlock),
		Inputs:   []*transaction.TxInput{spendingInput},
		Outputs:  []*transaction.TxOutput{spendingOutput, feeOutput},
	}

	sigHash = spendingTx.HashForWitnessV0(
		0,
		redeemScript[:],
		swapInValue,
		txscript.SigHashAll,
	)
	return spendingTx, sigHash, nil
}

func getPreimageWitness(signature, preimage, redeemScript []byte) [][]byte {
	sigWithHashType := append(signature, byte(txscript.SigHashAll))
	witness := make([][]byte, 0)
	witness = append(witness, preimage[:])
	witness = append(witness, sigWithHashType)
	witness = append(witness, redeemScript)
	return witness
}

func getCtlvWitness(signature, redeemScript []byte) [][]byte {
	sigWithHashType := append(signature, byte(txscript.SigHashAll))
	witness := make([][]byte, 0)
	witness = append(witness, sigWithHashType)
	witness = append(witness, []byte{})
	witness = append(witness, redeemScript)
	return witness
}

func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}
