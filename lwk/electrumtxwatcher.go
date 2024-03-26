package lwk

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/checksum0/go-electrum/electrum"
	"github.com/elementsproject/peerswap/onchain"
)

type electrumTxWatcher struct {
	electrumClient      ElectrumRPC
	blockheight         *electrum.SubscribeHeadersResult
	watchTicker         *time.Ticker
	blockheightChan     <-chan *electrum.SubscribeHeadersResult
	waitingConfirmation []*waitingElectrumTX
	cb                  waitingTXCallBacker
}

func NewElectrumTxWatcher(electrumClient ElectrumRPC) (*electrumTxWatcher, error) {
	r := &electrumTxWatcher{
		electrumClient: electrumClient,
		watchTicker:    time.NewTicker(2 * time.Second),
		cb:             waitingTXCallBacker{},
	}
	headerSubscription, err := r.electrumClient.SubscribeHeaders(context.Background())
	if err != nil {
		return nil, err
	}
	r.blockheightChan = headerSubscription
	return r, nil
}

func (r *electrumTxWatcher) StartWatchingTxs() error {
	ctx := context.Background()
	go func() {
		for {
			select {
			case <-r.watchTicker.C:
				fmt.Println("Checking for confirmations")
				for _, w := range r.waitingConfirmation {
					fmt.Println("Checking for confirmation of tx:", w.txID, "vout:", w.vout,
						"startingHeight:", w.height, "scriptHash:", w.scriptHash())
					if w.isConfirmed() {
						w := w // Assign loop variable to a new variable
						err := r.cb.callBack(r.blockheight.Height, w)
						if err != nil {
							fmt.Println("callback failed:", err)
						}
					} else {
						hs, err := r.electrumClient.GetHistory(ctx, w.scriptHash())
						if err != nil {
							fmt.Println("failed to get history:", err)
							continue
						}
						w.setHeight(hs)
						rawTx, err := r.electrumClient.GetRawTransaction(ctx, w.txID)
						if err != nil {
							fmt.Println("failed to get raw transaction:", err)
							continue
						}
						w.rawTX = rawTx
					}
				}
			case blockHeader := <-r.blockheightChan:
				r.blockheight = blockHeader
				fmt.Println("New block header received:", blockHeader)
			case <-ctx.Done():
				fmt.Println("Context canceled, stopping loop")
				return
			}
		}
	}()
	return nil
}

func (r *electrumTxWatcher) AddWaitForConfirmationTx(swapID, txID string, vout, startingHeight uint32, scriptpubkey []byte) {
	r.waitingConfirmation = append(r.waitingConfirmation, &waitingElectrumTX{
		swapID:       swapID,
		txID:         txID,
		kind:         confirmation,
		vout:         vout,
		scriptPubkey: scriptpubkey,
	})
}

func (r *electrumTxWatcher) AddConfirmationCallback(f func(swapId string, txHex string, err error) error) {
	r.cb.confirmationCallback = f
}
func (r *electrumTxWatcher) AddCsvCallback(f func(swapId string) error) {
	r.cb.csvCallback = f
}

func (r *electrumTxWatcher) GetBlockHeight() (uint32, error) {
	if r.blockheight == nil {
		return 0, fmt.Errorf("blockheight not available")
	}
	return uint32(r.blockheight.Height), nil
}

func (r *electrumTxWatcher) AddWaitForCsvTx(swapID, txID string, vout, startingHeight uint32, scriptpubkey []byte) {
	r.waitingConfirmation = append(r.waitingConfirmation, &waitingElectrumTX{
		swapID:       swapID,
		txID:         txID,
		kind:         csv,
		vout:         vout,
		scriptPubkey: scriptpubkey,
	})
}

type kind string

const (
	confirmation kind = "confirmation"
	csv          kind = "csv"
)

type waitingElectrumTX struct {
	swapID       string
	txID         string
	kind         kind
	height       uint32
	vout         uint32
	scriptPubkey []byte
	rawTX        string
}

// https://electrumx.readthedocs.io/en/latest/protocol-basics.html#script-hashes
func (tx *waitingElectrumTX) scriptHash() string {
	hash := sha256.Sum256(tx.scriptPubkey)
	reversedHash := make([]byte, len(hash))
	for i, b := range hash {
		reversedHash[len(hash)-1-i] = b
	}
	return fmt.Sprintf("%X", reversedHash)
}

func (tx *waitingElectrumTX) isConfirmed() bool {
	// If the transaction is unconfirmed,
	// 0 if all inputs are confirmed, and -1 otherwise.
	return tx.height > 0 && tx.rawTX != ""
}

func (tx *waitingElectrumTX) setHeight(hs []*electrum.GetMempoolResult) {
	for _, h := range hs {
		if h.Hash == tx.txID {
			tx.height = uint32(h.Height)
		}
	}
}

type waitingTXCallBacker struct {
	confirmationCallback func(swapId string, txHex string, err error) error
	csvCallback          func(swapId string) error
}

func (c *waitingTXCallBacker) callBack(currentHeight int32, w *waitingElectrumTX) error {
	if w.kind == confirmation {
		if currentHeight > int32(w.height)+onchain.LiquidConfs {
			return c.confirmationCallback(w.swapID, w.rawTX, nil)
		}
	} else if w.kind == csv {
		if currentHeight > int32(w.height)+onchain.LiquidCsv {
			return c.csvCallback(w.swapID)
		}
	}
	return nil
}
