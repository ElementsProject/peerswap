package lwk_test

import (
	"sync"
	"testing"

	"github.com/checksum0/go-electrum/electrum"
	"github.com/elementsproject/peerswap/lwk"
	mock_txwatcher "github.com/elementsproject/peerswap/lwk/mock"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/swap"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestElectrumTxWatcher_Callback(t *testing.T) {
	t.Parallel()
	t.Run("confirmed opening transaction", func(t *testing.T) {
		t.Parallel()
		var (
			wantSwapID       = swap.NewSwapId().String()
			wantTxID         = "1" // Single digit hash.
			wantTxHex        = "testb"
			wantscriptpubkey = []byte{
				// OP_0
				0x00,
				// OP_DATA_32
				0x20,
				// <32-byte script hash>
				0xec, 0x6f, 0x7a, 0x5a, 0xa8, 0xf2, 0xb1, 0x0c,
				0xa5, 0x15, 0x04, 0x52, 0x3a, 0x60, 0xd4, 0x03,
				0x06, 0xf6, 0x96, 0xcd, 0x06, 0xf6, 0x96, 0xcd,
				0x06, 0xf6, 0x96, 0xcd, 0x06, 0xf6, 0x96, 0xcd,
			}
			callbackChan         = make(chan string)
			targetTXHeight int32 = 100
		)

		electrumRPC := mock_txwatcher.NewMockelectrumRPC(gomock.NewController(t))
		headerResultChan := make(chan *electrum.SubscribeHeadersResult, 1)
		electrumRPC.EXPECT().SubscribeHeaders(gomock.Any()).
			Return(headerResultChan, nil)
		electrumRPC.EXPECT().GetHistory(gomock.Any(), gomock.Any()).Return([]*electrum.GetMempoolResult{
			{
				Hash:   wantTxID,
				Height: targetTXHeight,
			},
		}, nil)
		electrumRPC.EXPECT().GetRawTransaction(gomock.Any(), gomock.Any()).Return(wantTxHex, nil)

		r, err := lwk.NewElectrumTxWatcher(electrumRPC)
		assert.NoError(t, err)
		r.AddConfirmationCallback(
			func(swapId string, txHex string, err error) error {
				assert.Equal(t, wantSwapID, swapId)
				assert.Equal(t, wantTxHex, txHex)
				assert.NoError(t, err)
				callbackChan <- swapId
				return nil
			},
		)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err = r.StartWatchingTxs()
			assert.NoError(t, err)
			wg.Done()
		}()
		r.AddWaitForConfirmationTx(wantSwapID, wantTxID, 0, 0, wantscriptpubkey)
		headerResultChan <- &electrum.SubscribeHeadersResult{
			Height: onchain.LiquidConfs + targetTXHeight + 1,
		}
		wg.Wait()
		assert.Equal(t, <-callbackChan, wantSwapID)
	})

	t.Run("confirmed csv transaction", func(t *testing.T) {
		t.Parallel()
		var (
			wantSwapID       = swap.NewSwapId().String()
			wantTxID         = "1" // Single digit hash.
			wantscriptpubkey = []byte{
				// OP_0
				0x00,
				// OP_DATA_32
				0x20,
				// <32-byte script hash>
				0xec, 0x6f, 0x7a, 0x5a, 0xa8, 0xf2, 0xb1, 0x0c,
				0xa5, 0x15, 0x04, 0x52, 0x3a, 0x60, 0xd4, 0x03,
				0x06, 0xf6, 0x96, 0xcd, 0x06, 0xf6, 0x96, 0xcd,
				0x06, 0xf6, 0x96, 0xcd, 0x06, 0xf6, 0x96, 0xcd,
			}
			callbackChan   = make(chan string)
			targetTXHeight = int32(100)
		)

		electrumRPC := mock_txwatcher.NewMockelectrumRPC(gomock.NewController(t))
		headerResultChan := make(chan *electrum.SubscribeHeadersResult, 1)
		electrumRPC.EXPECT().SubscribeHeaders(gomock.Any()).
			Return(headerResultChan, nil)
		electrumRPC.EXPECT().GetHistory(gomock.Any(), gomock.Any()).Return([]*electrum.GetMempoolResult{
			{
				Hash:   wantTxID,
				Height: targetTXHeight,
			},
		}, nil)

		r, err := lwk.NewElectrumTxWatcher(electrumRPC)
		assert.NoError(t, err)
		r.AddCsvCallback(
			func(swapId string) error {
				assert.Equal(t, wantSwapID, swapId)
				assert.NoError(t, err)
				callbackChan <- swapId
				return nil
			},
		)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err = r.StartWatchingTxs()
			assert.NoError(t, err)
			wg.Done()
		}()
		r.AddWaitForCsvTx(wantSwapID, wantTxID, 0, 0, wantscriptpubkey)
		headerResultChan <- &electrum.SubscribeHeadersResult{
			Height: onchain.LiquidCsv + targetTXHeight + 1,
		}
		wg.Wait()
		assert.Equal(t, <-callbackChan, wantSwapID)
	})

}
