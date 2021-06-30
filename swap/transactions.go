package swap

import (
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/utils"
)

func CreateOpeningTransaction(services *SwapServices, swap *Swap) error {
	// Create the opening transaction
	blockHeight, err := services.blockchain.GetBlockHeight()
	if err != nil {
		return err
	}
	spendingBlocktimeHeight := int64(blockHeight + services.blockchain.GetLocktime())
	swap.Cltv = spendingBlocktimeHeight
	redeemScript, err := services.utils.GetSwapScript(swap.TakerPubkeyHash,swap.MakerPubkeyHash,swap.PHash,swap.Cltv)
	if err != nil {
		return err
	}

	openingTx, err := services.utils.CreateOpeningTransaction(redeemScript, services.blockchain.GetAsset(), swap.Amount)
	if err != nil {
		return err
	}

	txHex, fee, err := services.wallet.CreateFundedTransaction(openingTx)
	if err != nil {
		return err
	}
	vout, err := services.utils.VoutFromTxHex(txHex, redeemScript)
	if err != nil {
		return err
	}

	swap.OpeningTxUnpreparedHex = txHex
	swap.OpeningTxFee = fee
	swap.OpeningTxVout = vout

	return nil
}

func CreatePreimageSpendingTransaction(services *SwapServices, swap *Swap) (string, error) {
	blockchain := services.blockchain
	wallet := services.wallet

	address, err := wallet.GetAddress()
	if err != nil {
		return "", err
	}
	outputScript, err := services.utils.Blech32ToScript(address, blockchain.GetNetwork())
	if err != nil {
		return "", err
	}

	redeemScript, err := services.utils.GetSwapScript(swap.TakerPubkeyHash,swap.MakerPubkeyHash,swap.PHash,swap.Cltv)
	if err != nil {
		return "", err
	}

	blockheight, err := blockchain.GetBlockHeight()
	if err != nil {
		return "", err
	}
	// todo correct fee
	spendingTx, sigHash, err := services.utils.CreateSpendingTransaction(swap.OpeningTxHex, swap.Amount, services.blockchain.GetFee(""), blockheight, blockchain.GetAsset(), redeemScript, outputScript)
	if err != nil {
		return "", err
	}

	sig, err := swap.GetPrivkey().Sign(sigHash[:])
	if err != nil {
		return "", err
	}

	preimage, err := lightning.MakePreimageFromStr(swap.PreImage)
	if err != nil {
		return "", err

	}

	spendingTx.Inputs[0].Witness = services.utils.GetPreimageWitness(sig.Serialize(), preimage[:], redeemScript)

	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		return "", err
	}
	return spendingTxHex, nil
}

func CreateCltvSpendingTransaction(services *SwapServices, swap *Swap) (string, error) {
	blockchain := services.blockchain
	wallet := services.wallet

	address, err := wallet.GetAddress()
	if err != nil {
		return "", err
	}
	outputScript, err := services.utils.Blech32ToScript(address, blockchain.GetNetwork())
	if err != nil {
		return "", err
	}

	redeemScript, err := services.utils.GetSwapScript(swap.TakerPubkeyHash,swap.MakerPubkeyHash,swap.PHash,swap.Cltv)
	if err != nil {
		return "", err
	}

	blockheight, err := blockchain.GetBlockHeight()
	if err != nil {
		return "", err
	}

	spendingTx, sigHash, err := services.utils.CreateSpendingTransaction(swap.OpeningTxHex, swap.Amount, services.blockchain.GetFee(""), blockheight, blockchain.GetAsset(), redeemScript, outputScript)
	if err != nil {
		return "", err
	}

	sig, err := swap.GetPrivkey().Sign(sigHash[:])
	if err != nil {
		return "", err
	}

	spendingTx.Inputs[0].Witness = utils.GetCltvWitness(sig.Serialize(), redeemScript)

	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		return "", err
	}
	return spendingTxHex, nil
}
