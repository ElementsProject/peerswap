package utils

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/transaction"
)

type Utility struct{}

// CreateOpeningTransaction returns the opening transaction for the swap
func (u *Utility) CreateOpeningTransaction(redeemScript []byte, asset []byte, amount uint64) (*transaction.Transaction, error) {
	return CreateOpeningTransaction(redeemScript, asset, amount)
}

// VoutFromTxHex returns the swap vout from an opening transaction
func (u *Utility) VoutFromTxHex(txHex string, redeemScript []byte) (uint32, error) {
	return VoutFromTxHex(txHex, redeemScript)
}

// Blech32ToScript returns the script of a bech32 addr
func (u *Utility) Blech32ToScript(blech32Addr string, network *network.Network) ([]byte, error) {
	return Blech32ToScript(blech32Addr, network)
}

// CreateSpendingTransaction returns the spendningTransaction for the swap
func (u *Utility) CreateSpendingTransaction(openingTxHex string, swapAmount, feeAmount, currentBlock uint64, asset, redeemScript, outputScript []byte) (tx *transaction.Transaction, sigHash [32]byte, err error) {
	return CreateSpendingTransaction(openingTxHex, swapAmount, feeAmount, currentBlock, asset, redeemScript, outputScript)
}

func (u *Utility) CheckTransactionValidity(openingTxHex string, swapAmount uint64, redeemScript []byte) error {
	openingTx, err := transaction.NewTxFromHex(openingTxHex)
	if err != nil {
		return err
	}
	swapInValue, err := elementsutil.SatoshiToElementsValue(swapAmount)
	if err != nil {
		return err
	}
	vout, err := FindVout(openingTx.Outputs, redeemScript)
	if err != nil {
		return err
	}
	if bytes.Compare(openingTx.Outputs[vout].Value, swapInValue) != 0 {
		return errors.New("swap value does not match tx value")
	}
	return nil
}

// GetSwapScript returns the swap script
func (u *Utility) GetSwapScript(takerPubkeyHash, makerPubkeyHash, paymentHash string, csv uint32) ([]byte, error) {
	// check script
	takerPubkeyHashBytes, err := hex.DecodeString(takerPubkeyHash)
	if err != nil {
		return nil, err
	}
	makerPubkeyHashBytes, err := hex.DecodeString(makerPubkeyHash)
	if err != nil {
		return nil, err
	}
	pHashBytes, err := hex.DecodeString(paymentHash)
	if err != nil {
		return nil, err
	}
	script, err := GetOpeningTxScript(takerPubkeyHashBytes, makerPubkeyHashBytes, pHashBytes, csv)
	if err != nil {
		return nil, err
	}
	return script, nil
}

// GetPreimageWitness returns the witness for spending the transaction with the preimage
func (u *Utility) GetPreimageWitness(signature, preimage, redeemScript []byte) [][]byte {
	return GetPreimageWitness(signature, preimage, redeemScript)
}

// GetCsvWitness returns the witness for spending the transaction with a passed csv
func (u *Utility) GetCsvWitness(signature, redeemScript []byte) [][]byte {
	return GetCsvWitness(signature, redeemScript)
}
