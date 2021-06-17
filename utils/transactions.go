package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/txscript"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/transaction"
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

func CreatePreimageSpendingTransaction(signer TxSigner, openingTxHex string, swapAmount, feeAmount, currentBlock uint64, asset, outputScript, redeemScript, preimage []byte) (string, error) {
	spendingTx, sigHash, err := createSpendingTransaction(openingTxHex, swapAmount, feeAmount, currentBlock, asset, redeemScript, outputScript)
	if err != nil {
		return "", err
	}

	sig, err := signer.Sign(sigHash[:])
	if err != nil {
		return "", err
	}

	spendingTx.Inputs[0].Witness = getPreimageWitness(sig.Serialize(), preimage, redeemScript)

	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		return "", err
	}
	return spendingTxHex, nil
}

func CreateCltvSpendingTransaction(signer TxSigner, openingTxHex string, swapAmount, feeAmount, currentBlock uint64, asset, outputScript, redeemScript []byte) (string, error) {
	spendingTx, sigHash, err := createSpendingTransaction(openingTxHex, swapAmount, feeAmount, currentBlock, asset, redeemScript, outputScript)
	if err != nil {
		return "", err
	}

	sig, err := signer.Sign(sigHash[:])
	if err != nil {
		return "", err
	}

	spendingTx.Inputs[0].Witness = getCtlvWitness(sig.Serialize(), redeemScript)

	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		return "", err
	}
	return spendingTxHex, nil
}

func createSpendingTransaction(openingTxHex string, swapAmount, feeAmount, currentBlock uint64, asset, redeemScript, outputScript []byte) (tx *transaction.Transaction, sigHash [32]byte, err error) {
	firstTx, err := transaction.NewTxFromHex(openingTxHex)
	if err != nil {
		return nil, [32]byte{}, err
	}

	swapInValue, _ := elementsutil.SatoshiToElementsValue(swapAmount)
	var vout uint32
	for i, v := range firstTx.Outputs {
		if bytes.Compare(v.Value, swapInValue) == 0 {
			vout = uint32(i)
		}
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
