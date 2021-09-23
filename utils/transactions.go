package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

// GetOpeningTxScript returns the script for the opening transaction of a swap,
// where the taker is the peer paying the invoice and the maker the peer providing the lbtc
func GetOpeningTxScript(takerPubkeyHash []byte, makerPubkeyHash []byte, pHash []byte, locktimeHeight int64) ([]byte, error) {
	script := txscript.NewScriptBuilder().
		AddData(makerPubkeyHash).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_NOTIF).
		AddData(makerPubkeyHash).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_NOTIF).
		AddOp(txscript.OP_SIZE).
		AddData(h2b("20")).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_SHA256).
		AddData(pHash[:]).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_ENDIF).
		AddData(takerPubkeyHash).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_ELSE).
		AddInt64(locktimeHeight).
		AddOp(txscript.OP_CHECKLOCKTIMEVERIFY).
		AddOp(txscript.OP_ENDIF)
	return script.Script()
}

// CreatOpeningAddress returns the address for the opening tx
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

func CreateOpeningTransaction(redeemScript []byte, asset []byte, amount uint64) (*transaction.Transaction, error) {
	scriptPubKey := []byte{0x00, 0x20}
	witnessProgram := sha256.Sum256(redeemScript)
	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)

	redeemPayment, _ := payment.FromScript(scriptPubKey, &network.Regtest, nil)
	sats, _ := elementsutil.SatoshiToElementsValue(amount)
	output := transaction.NewTxOutput(asset, sats, redeemPayment.WitnessScript)
	tx := transaction.NewTx(2)
	tx.Outputs = append(tx.Outputs, output)
	return tx, nil
}

func VoutFromTxHex(txHex string, redeemScript []byte) (uint32, error) {
	tx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		return 0, err
	}
	vout, err := FindVout(tx.Outputs, redeemScript)
	if err != nil {
		return 0, err
	}
	return vout, nil
}

func FindVout(outputs []*transaction.TxOutput, redeemScript []byte) (uint32, error) {
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

// CreateSpendingTransaction returns the spendningTransaction for the swap
func CreateSpendingTransaction(openingTxHex string, swapAmount, feeAmount, currentBlock uint64, asset, redeemScript, outputScript []byte) (tx *transaction.Transaction, sigHash [32]byte, err error) {
	firstTx, err := transaction.NewTxFromHex(openingTxHex)
	if err != nil {
		log.Printf("error creating first tx %s", openingTxHex)
		return nil, [32]byte{}, err
	}

	swapInValue, err := elementsutil.SatoshiToElementsValue(swapAmount)
	if err != nil {
		log.Printf("error getting swapin value")
		return nil, [32]byte{}, err
	}
	vout, err := FindVout(firstTx.Outputs, redeemScript)
	if err != nil {
		log.Printf("error finding vour")
		return nil, [32]byte{}, err
	}

	txHash := firstTx.TxHash()
	spendingInput := transaction.NewTxInput(txHash[:], vout)
	spendingInput.Sequence = 0
	spendingSatsBytes, _ := elementsutil.SatoshiToElementsValue(swapAmount - feeAmount)

	var txOutputs = []*transaction.TxOutput{}

	spendingOutput := transaction.NewTxOutput(asset, spendingSatsBytes[:], outputScript)
	txOutputs = append(txOutputs, spendingOutput)

	if feeAmount > 0 {
		feeValue, _ := elementsutil.SatoshiToElementsValue(feeAmount)
		feeScript := []byte{}
		feeOutput := transaction.NewTxOutput(asset, feeValue, feeScript)
		txOutputs = append(txOutputs, feeOutput)
	}

	spendingTx := &transaction.Transaction{
		Version:  2,
		Flag:     0,
		Locktime: uint32(currentBlock),
		Inputs:   []*transaction.TxInput{spendingInput},
		Outputs:  txOutputs,
	}

	sigHash = spendingTx.HashForWitnessV0(
		0,
		redeemScript[:],
		swapInValue,
		txscript.SigHashAll,
	)
	return spendingTx, sigHash, nil
}

// GetPreimageWitness returns the witness for spending the transaction with the preimage
func GetPreimageWitness(signature, preimage, redeemScript []byte) [][]byte {
	sigWithHashType := append(signature, byte(txscript.SigHashAll))
	witness := make([][]byte, 0)
	witness = append(witness, []byte{})
	witness = append(witness, []byte{})
	witness = append(witness, preimage[:])
	witness = append(witness, sigWithHashType)
	witness = append(witness, redeemScript)
	return witness
}

// GetCltvWitness returns the witness for spending the transaction with a passed cltv
func GetCltvWitness(signature, redeemScript []byte) [][]byte {
	sigWithHashType := append(signature, byte(txscript.SigHashAll))
	witness := make([][]byte, 0)
	witness = append(witness, sigWithHashType)

	witness = append(witness, redeemScript)
	return witness
}

func GetCooperativeWitness(takerSig, makerSig, redeemScript []byte) [][]byte {
	witness := make([][]byte, 0)
	witness = append(witness, []byte{})
	witness = append(witness, append(makerSig, byte(txscript.SigHashAll)))
	witness = append(witness, append(takerSig, byte(txscript.SigHashAll)))
	witness = append(witness, redeemScript)
	return witness
}

func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}
