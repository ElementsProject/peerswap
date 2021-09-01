package swap

import (
	"github.com/btcsuite/btcd/btcec"
)

// CreateOpeningTransaction creates the opening transaction from a swap
func CreateOpeningTransaction(services *SwapServices, swap *SwapData) error {
	// Create the opening transaction
	txHex, _, fee, cltv, vout, err := services.onchain.CreateOpeningTransaction(&OpeningParams{
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
	key, _ := btcec.PrivKeyFromBytes(btcec.S256(), swap.PrivkeyBytes)
	openingParams := &OpeningParams{
		TakerPubkeyHash:  swap.TakerPubkeyHash,
		MakerPubkeyHash:  swap.MakerPubkeyHash,
		ClaimPaymentHash: swap.ClaimPaymentHash,
		Amount:           swap.Amount,
	}
	spendParams := &ClaimParams{
		Preimage: swap.ClaimPreimage,
		Signer:   key,
		Cltv:     swap.Cltv,
	}
	txId, _, err := services.onchain.CreatePreimageSpendingTransaction(openingParams, spendParams, swap.OpeningTxId)
	if err != nil {
		return err
	}
	swap.ClaimTxId = txId

	return nil
}

// CreateCltvSpendingTransaction creates the spending transaction from a swap when spending the cltv passed branch
func CreateCltvSpendingTransaction(services *SwapServices, swap *SwapData) error {
	key, _ := btcec.PrivKeyFromBytes(btcec.S256(), swap.PrivkeyBytes)
	openingParams := &OpeningParams{
		TakerPubkeyHash:  swap.TakerPubkeyHash,
		MakerPubkeyHash:  swap.MakerPubkeyHash,
		ClaimPaymentHash: swap.ClaimPaymentHash,
		Amount:           swap.Amount,
	}
	spendParams := &ClaimParams{
		Signer: key,
		Cltv:   swap.Cltv,
	}
	txId, _, err := services.onchain.CreateCltvSpendingTransaction(openingParams, spendParams, swap.OpeningTxHex, swap.OpeningTxVout)
	if err != nil {
		return err
	}
	swap.ClaimTxId = txId

	return nil
}
