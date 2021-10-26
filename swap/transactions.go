package swap

import (
	"github.com/btcsuite/btcd/btcec"
)

// CreateOpeningTransaction creates the opening transaction from a swap
func CreateOpeningTransaction(services *SwapServices, swap *SwapData) error {
	onchain, err := services.getOnchainAsset(swap.Asset)
	if err != nil {
		return err
	}

	// Create the opening transaction
	txHex, _, fee, _, vout, err := onchain.CreateOpeningTransaction(&OpeningParams{
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

	return nil
}

func SetRefundAddress(services *SwapServices, swap *SwapData) error {
	onchain, err := services.getOnchainAsset(swap.Asset)
	if err != nil {
		return err
	}

	refundAddr, err := onchain.CreateRefundAddress()
	if err != nil {
		return err
	}
	swap.MakerRefundAddr = refundAddr
	return nil
}

// CreatePreimageSpendingTransaction creates the spending transaction from a swap when spending the preimage branch
func CreatePreimageSpendingTransaction(services *SwapServices, swap *SwapData) error {
	onchain, err := services.getOnchainAsset(swap.Asset)
	if err != nil {
		return err
	}
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
	}
	txId, _, err := onchain.CreatePreimageSpendingTransaction(openingParams, spendParams, swap.OpeningTxId)
	if err != nil {
		return err
	}
	swap.ClaimTxId = txId

	return nil
}

// CreateCsvSpendingTransaction creates the spending transaction from a swap when spending the csv passed branch
func CreateCsvSpendingTransaction(services *SwapServices, swap *SwapData) error {
	onchain, err := services.getOnchainAsset(swap.Asset)
	if err != nil {
		return err
	}
	key, _ := btcec.PrivKeyFromBytes(btcec.S256(), swap.PrivkeyBytes)
	openingParams := &OpeningParams{
		TakerPubkeyHash:  swap.TakerPubkeyHash,
		MakerPubkeyHash:  swap.MakerPubkeyHash,
		ClaimPaymentHash: swap.ClaimPaymentHash,
		Amount:           swap.Amount,
	}
	spendParams := &ClaimParams{
		Signer: key,
	}
	txId, _, err := onchain.CreateCsvSpendingTransaction(openingParams, spendParams, swap.OpeningTxHex, swap.OpeningTxVout)
	if err != nil {
		return err
	}
	swap.ClaimTxId = txId

	return nil
}
