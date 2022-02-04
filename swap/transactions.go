package swap

import (
	"encoding/hex"

	"github.com/btcsuite/btcd/btcec"
)

type CreateOpeningTxResponse struct {
	UnpreparedHex string
	Fee           uint64
	Vout          uint32
	BlindingKey   string
}

// CreateOpeningTransaction creates the opening transaction from a swap
func CreateOpeningTransaction(services *SwapServices, chain, takerPubkey, makerPubkey, claimPaymentHash string, amount uint64) (*CreateOpeningTxResponse, error) {
	_, wallet, _, err := services.getOnChainServices(chain)
	if err != nil {
		return nil, err
	}

	var blindingKey *btcec.PrivateKey
	var blindingKeyHex string
	if chain == l_btc_chain && blindingKey == nil {
		blindingKey, err = btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			return nil, err
		}
		blindingKeyHex = hex.EncodeToString(blindingKey.Serialize())
	}

	// Create the opening transaction
	txHex, fee, vout, err := wallet.CreateOpeningTransaction(&OpeningParams{
		TakerPubkeyHash:  takerPubkey,
		MakerPubkeyHash:  makerPubkey,
		ClaimPaymentHash: claimPaymentHash,
		Amount:           amount,
		BlindingKey:      blindingKey,
	})
	if err != nil {
		return nil, err
	}

	res := &CreateOpeningTxResponse{
		UnpreparedHex: txHex,
		Fee:           fee,
		Vout:          vout,
		BlindingKey:   blindingKeyHex,
	}

	return res, nil
}

// CreatePreimageSpendingTransaction creates the spending transaction from a swap when spending the preimage branch
func CreatePreimageSpendingTransaction(services *SwapServices, chain string, openingParams *OpeningParams, claimParams *ClaimParams) (string, error) {
	_, wallet, _, err := services.getOnChainServices(chain)
	if err != nil {
		return "", err
	}

	txId, _, err := wallet.CreatePreimageSpendingTransaction(openingParams, claimParams)
	if err != nil {
		return "", err
	}

	return txId, nil
}

// CreateCsvSpendingTransaction creates the spending transaction from a swap when spending the csv passed branch
func CreateCsvSpendingTransaction(services *SwapServices, chain string, openingParams *OpeningParams, claimParams *ClaimParams) (string, error) {
	_, wallet, _, err := services.getOnChainServices(chain)
	if err != nil {
		return "", err
	}

	txId, _, err := wallet.CreateCsvSpendingTransaction(openingParams, claimParams)
	if err != nil {
		return "", err
	}

	return txId, nil
}
