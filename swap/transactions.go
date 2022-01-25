package swap

import (
	"encoding/hex"

	"github.com/btcsuite/btcd/btcec"
)

// CreateOpeningTransaction creates the opening transaction from a swap
func CreateOpeningTransaction(services *SwapServices, swap *SwapData) error {
	_, wallet, _, err := services.getOnchainAsset(swap.Asset)
	if err != nil {
		return err
	}
	var blindingKey *btcec.PrivateKey
	if swap.Asset == l_btc_asset {
		blindingKey, err = btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			return err
		}
		swap.BlindingKeyHex = hex.EncodeToString(blindingKey.Serialize())
	}
	// Create the opening transaction
	txHex, fee, vout, err := wallet.CreateOpeningTransaction(&OpeningParams{
		TakerPubkeyHash:  swap.TakerPubkeyHash,
		MakerPubkeyHash:  swap.MakerPubkeyHash,
		ClaimPaymentHash: swap.ClaimPaymentHash,
		Amount:           swap.Amount,
		BlindingKey:      blindingKey,
	})
	if err != nil {
		return err
	}
	swap.OpeningTxUnpreparedHex = txHex
	swap.OpeningTxFee = fee
	swap.OpeningTxVout = vout

	return nil
}

// CreatePreimageSpendingTransaction creates the spending transaction from a swap when spending the preimage branch
func CreatePreimageSpendingTransaction(services *SwapServices, swap *SwapData) error {
	_, wallet, _, err := services.getOnchainAsset(swap.Asset)
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
	claimParams := &ClaimParams{
		Preimage:     swap.ClaimPreimage,
		Signer:       key,
		OpeningTxHex: swap.OpeningTxHex,
	}
	if swap.Asset == l_btc_asset {
		err = SetBlindingParams(swap, openingParams)
		if err != nil {
			return err
		}
	}
	txId, _, err := wallet.CreatePreimageSpendingTransaction(openingParams, claimParams)
	if err != nil {
		return err
	}
	swap.ClaimTxId = txId

	return nil
}

func SetBlindingParams(swap *SwapData, openingParams *OpeningParams) error {
	blindingKeyBytes, err := hex.DecodeString(swap.BlindingKeyHex)
	if err != nil {
		return err
	}
	blindingKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), blindingKeyBytes)
	openingParams.BlindingKey = blindingKey
	return nil

}

// CreateCsvSpendingTransaction creates the spending transaction from a swap when spending the csv passed branch
func CreateCsvSpendingTransaction(services *SwapServices, swap *SwapData) error {
	_, wallet, _, err := services.getOnchainAsset(swap.Asset)
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
	claimParams := &ClaimParams{
		Signer:       key,
		OpeningTxHex: swap.OpeningTxHex,
	}
	if swap.Asset == l_btc_asset {
		err = SetBlindingParams(swap, openingParams)
		if err != nil {
			return err
		}
	}
	txId, _, err := wallet.CreateCsvSpendingTransaction(openingParams, claimParams)
	if err != nil {
		return err
	}
	swap.ClaimTxId = txId

	return nil
}
