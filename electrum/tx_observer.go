package electrum

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/checksum0/go-electrum/electrum"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/swap"
)

type TXObserver interface {
	GetSwapID() swap.SwapId
	// Callback calls the callback function if the condition is match.
	// Returns true if the callback function is called.
	Callback(context.Context, BlockHeight) (bool, error)
}

type scriptPubKey struct {
	txscript.PkScript
}

func NewScriptPubKey(script []byte) (scriptPubKey, error) {
	s, err := txscript.ParsePkScript(script)
	if err != nil {
		return scriptPubKey{s}, fmt.Errorf("failed to parse script: %w", err)
	}
	return scriptPubKey{s}, nil
}
func (s *scriptPubKey) scriptHash() string {
	hash := sha256.Sum256(s.Script())
	reversedHash := make([]byte, len(hash))
	for i, b := range hash {
		reversedHash[len(hash)-1-i] = b
	}
	return fmt.Sprintf("%X", reversedHash)
}

type confirmationCallback = func(swapId string, txHex string, err error) error

type observeOpeningTX struct {
	swapID         swap.SwapId
	txID           *chainhash.Hash
	scriptPubkey   scriptPubKey
	electrumClient RPC
	cb             confirmationCallback
	startingHeight uint32
	paymentWindow  uint32
}

var _ TXObserver = (*observeOpeningTX)(nil)

func NewObserveOpeningTX(
	swapID swap.SwapId,
	txID *chainhash.Hash,
	scriptPubkey scriptPubKey,
	electrumClient RPC,
	cb confirmationCallback,
	startingHeight,
	paymentWindow uint32,
) observeOpeningTX {
	return observeOpeningTX{
		swapID:         swapID,
		txID:           txID,
		scriptPubkey:   scriptPubkey,
		electrumClient: electrumClient,
		cb:             cb,
		startingHeight: startingHeight,
		paymentWindow:  paymentWindow,
	}
}

func (o *observeOpeningTX) GetSwapID() swap.SwapId {
	return o.swapID
}

func getHeight(hs []*electrum.GetMempoolResult, txID *chainhash.Hash) (BlockHeight, bool) {
	for _, h := range hs {
		if h == nil {
			continue
		}
		hh, err := chainhash.NewHashFromStr(h.Hash)
		if err != nil {
			continue
		}
		if hh.IsEqual(txID) {
			return BlockHeight(h.Height), true
		}
	}
	return 0, false
}

func hasConfirmations(txHeight, tipHeight BlockHeight, required uint32) (bool, error) {
	if tipHeight <= 0 {
		return false, fmt.Errorf("invalid electrum tip height: %d", tipHeight)
	}
	if txHeight <= 0 {
		return false, nil
	}
	if txHeight > tipHeight {
		return false, fmt.Errorf(
			"electrum transaction height %d is above tip height %d",
			txHeight, tipHeight,
		)
	}

	confirmations := tipHeight - txHeight + 1
	return confirmations >= BlockHeight(required), nil
}

func (o *observeOpeningTX) Callback(ctx context.Context, currentHeight BlockHeight) (bool, error) {
	if currentHeight <= 0 {
		return false, fmt.Errorf("invalid electrum tip height: %d", currentHeight)
	}
	deadline := BlockHeight(o.startingHeight) + BlockHeight(o.paymentWindow)
	if currentHeight < BlockHeight(o.startingHeight) || currentHeight >= deadline {
		err := fmt.Errorf(
			"claim payment deadline exceeded: current height %d, deadline %d",
			currentHeight,
			deadline,
		)
		return true, o.cb(o.swapID.String(), "", err)
	}

	hs, err := o.electrumClient.GetHistory(ctx, o.scriptPubkey.scriptHash())
	if err != nil {
		return false, fmt.Errorf("failed to get history: %w", err)
	}
	txHeight, found := getHeight(hs, o.txID)
	if !found {
		return false, nil
	}
	confirmed, err := hasConfirmations(
		txHeight, currentHeight, uint32(onchain.LiquidConfs),
	)
	if err != nil {
		return false, err
	}
	if !confirmed {
		return false, nil
	}
	rawTx, err := o.electrumClient.GetRawTransaction(ctx, o.txID.String())
	if err != nil {
		log.Debugf("failed to get raw transaction: %s", o.txID.String())
		return false, nil
	}
	return true, o.cb(o.swapID.String(), rawTx, nil)
}

type csvCallback = func(swapId string) error

type observeCSVTX struct {
	swapID         swap.SwapId
	txID           *chainhash.Hash
	scriptPubkey   scriptPubKey
	electrumClient RPC
	cb             csvCallback
	csv            uint32
}

var _ TXObserver = (*observeCSVTX)(nil)

func NewobserveCSVTX(
	swapID swap.SwapId,
	txID *chainhash.Hash,
	scriptPubkey scriptPubKey,
	electrumClient RPC,
	cb csvCallback,
	csv uint32,
) observeCSVTX {
	return observeCSVTX{
		swapID:         swapID,
		txID:           txID,
		scriptPubkey:   scriptPubkey,
		electrumClient: electrumClient,
		cb:             cb,
		csv:            csv,
	}
}

func (o *observeCSVTX) GetSwapID() swap.SwapId {
	return o.swapID
}

func (o *observeCSVTX) Callback(ctx context.Context, currentHeight BlockHeight) (bool, error) {
	hs, err := o.electrumClient.GetHistory(ctx, o.scriptPubkey.scriptHash())
	if err != nil {
		return false, fmt.Errorf("failed to get history: %w", err)
	}
	txHeight, found := getHeight(hs, o.txID)
	if !found {
		return false, nil
	}
	mature, err := hasConfirmations(
		txHeight, currentHeight, o.csv,
	)
	if err != nil {
		return false, err
	}
	if !mature {
		log.Debugf("the transaction is unconfirmed. txhash: %s", o.txID.String())
		return false, nil
	}
	return true, o.cb(o.swapID.String())
}
