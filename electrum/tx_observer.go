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
	Callback(context.Context, BlocKHeight) (bool, error)
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
}

var _ TXObserver = (*observeOpeningTX)(nil)

func NewObserveOpeningTX(
	swapID swap.SwapId,
	txID *chainhash.Hash,
	scriptPubkey scriptPubKey,
	electrumClient RPC,
	cb confirmationCallback) observeOpeningTX {
	return observeOpeningTX{
		swapID:         swapID,
		txID:           txID,
		scriptPubkey:   scriptPubkey,
		electrumClient: electrumClient,
		cb:             cb,
	}
}

func (o *observeOpeningTX) GetSwapID() swap.SwapId {
	return o.swapID
}

func getHeight(hs []*electrum.GetMempoolResult, txID *chainhash.Hash) BlocKHeight {
	for _, h := range hs {
		hh, err := chainhash.NewHashFromStr(h.Hash)
		if err != nil {
			continue
		}
		if hh.IsEqual(txID) {
			return BlocKHeight(h.Height)
		}
	}
	return 0
}

func (o *observeOpeningTX) Callback(ctx context.Context, currentHeight BlocKHeight) (bool, error) {
	hs, err := o.electrumClient.GetHistory(ctx, o.scriptPubkey.scriptHash())
	if err != nil {
		return false, fmt.Errorf("failed to get history: %w", err)
	}
	if !(getHeight(hs, o.txID).Confirmed()) {
		return false, fmt.Errorf("the transaction is unconfirmed")
	}
	rawTx, err := o.electrumClient.GetRawTransaction(ctx, o.txID.String())
	if err != nil {
		log.Debugf("failed to get raw transaction: %s", o.txID.String())
		return false, nil
	}
	if !(currentHeight.Height() >= getHeight(hs, o.txID).Height()+uint32(onchain.LiquidConfs)-1) {
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
}

var _ TXObserver = (*observeCSVTX)(nil)

func NewobserveCSVTX(
	swapID swap.SwapId,
	txID *chainhash.Hash,
	scriptPubkey scriptPubKey,
	electrumClient RPC,
	cb csvCallback) observeCSVTX {
	return observeCSVTX{
		swapID:         swapID,
		txID:           txID,
		scriptPubkey:   scriptPubkey,
		electrumClient: electrumClient,
		cb:             cb,
	}
}

func (o *observeCSVTX) GetSwapID() swap.SwapId {
	return o.swapID
}

func (o *observeCSVTX) Callback(ctx context.Context, currentHeight BlocKHeight) (bool, error) {
	hs, err := o.electrumClient.GetHistory(ctx, o.scriptPubkey.scriptHash())
	if err != nil {
		return false, fmt.Errorf("failed to get history: %w", err)
	}
	if !(getHeight(hs, o.txID).Confirmed()) {
		log.Debugf("the transaction is unconfirmed. txhash: %s", o.txID.String())
		return false, nil
	}
	if !(currentHeight.Height() >= getHeight(hs, o.txID).Height()+uint32(onchain.LiquidCsv-1)) {
		return false, nil
	}
	return true, o.cb(o.swapID.String())
}
