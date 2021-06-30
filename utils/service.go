package utils

import (
	"encoding/hex"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/transaction"
)

type Utility struct {}

func (u *Utility) CreateOpeningTransaction(redeemScript []byte, asset []byte, amount uint64) (*transaction.Transaction, error) {
	return CreateOpeningTransaction(redeemScript,asset,amount)
}

func (u *Utility) VoutFromTxHex(txHex string, redeemScript []byte) (uint32, error) {
	return VoutFromTxHex(txHex, redeemScript)
}

func (u *Utility) Blech32ToScript(blech32Addr string, network *network.Network) ([]byte, error) {
	return Blech32ToScript(blech32Addr, network)
}

func (u *Utility) CreateSpendingTransaction(openingTxHex string, swapAmount, feeAmount, currentBlock uint64, asset, redeemScript, outputScript []byte) (tx *transaction.Transaction, sigHash [32]byte, err error) {
	return CreateSpendingTransaction(openingTxHex,swapAmount,feeAmount,currentBlock, asset, redeemScript,outputScript)
}

func (u *Utility) GetSwapScript(takerPubkeyHash, makerPubkeyHash, paymentHash string, cltv int64) ([]byte, error) {
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
	script, err := GetOpeningTxScript(takerPubkeyHashBytes, makerPubkeyHashBytes, pHashBytes, cltv)
	if err != nil {
		return nil, err
	}
	return script, nil
}

func (u *Utility) GetPreimageWitness(signature, preimage, redeemScript []byte) [][]byte {
	return GetPreimageWitness(signature,preimage,redeemScript)
}

func (u *Utility) GetCltvWitness(signature, redeemScript []byte) [][]byte {
	return GetCltvWitness(signature,redeemScript)
}

