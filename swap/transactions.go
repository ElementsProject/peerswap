package swap

import (
	"github.com/btcsuite/btcd/btcec"
)

// CreateOpeningTransaction creates the opening transaction from a swap
func CreateOpeningTransaction(services *SwapServices, swap *SwapData) error {
	// Create the opening transaction
	txHex, fee, cltv, vout, err:= services.onchain.CreateOpeningTransaction(&OpeningParams{
		TakerPubkeyHash:  swap.TakerPubkeyHash,
		MakerPubkeyHash:  swap.MakerPubkeyHash,
		ClaimPaymentHash: swap.ClaimPaymentHash,
		Amount:           swap.Amount,
	})
	if err != nil {
		return err
	}
	swap.OpeningTxUnpreparedHex = txHex
	swap.OpeningTxFee = fee
	swap.OpeningTxVout = vout
	swap.Cltv = cltv

	return nil
}


// CreatePreimageSpendingTransaction creates the spending transaction from a swap when spending the preimage branch
func CreatePreimageSpendingTransaction(services *SwapServices, swap *SwapData) error {
	key,_ := btcec.PrivKeyFromBytes(btcec.S256(), swap.PrivkeyBytes)
	openingParams := &OpeningParams{
		TakerPubkeyHash:  swap.TakerPubkeyHash,
		MakerPubkeyHash:  swap.MakerPubkeyHash,
		ClaimPaymentHash: swap.ClaimPaymentHash,
		Amount:           swap.Amount,
	}
	spendParams := &ClaimParams{
		OpeningTxHex: swap.OpeningTxHex,
		Preimage:     swap.ClaimPreimage,
		Signer:       key,
	}
	txId, _, err := services.onchain.CreatePreimageSpendingTransaction(openingParams,spendParams)
	if err != nil {
		return err
	}
	swap.ClaimTxId = txId

	return  nil
}

// CreateCltvSpendingTransaction creates the spending transaction from a swap when spending the cltv passed branch
func CreateCltvSpendingTransaction(services *SwapServices, swap *SwapData) (error) {
	key,_ := btcec.PrivKeyFromBytes(btcec.S256(), swap.PrivkeyBytes)
	openingParams := &OpeningParams{
		TakerPubkeyHash:  swap.TakerPubkeyHash,
		MakerPubkeyHash:  swap.MakerPubkeyHash,
		ClaimPaymentHash: swap.ClaimPaymentHash,
		Amount:           swap.Amount,
	}
	spendParams := &ClaimParams{
		OpeningTxHex: swap.OpeningTxHex,
		Signer:       key,
	}
	txId, _, err := services.onchain.CreatePreimageSpendingTransaction(openingParams,spendParams)
	if err != nil {
		return err
	}
	swap.ClaimTxId = txId

	return  nil
}
