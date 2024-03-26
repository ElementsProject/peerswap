package lwk_test

import (
	"testing"

	"github.com/checksum0/go-electrum/electrum"
	"github.com/elementsproject/peerswap/lwk"
	mock_txwatcher "github.com/elementsproject/peerswap/lwk/mock"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestElectrumTxWatcher_Callback(t *testing.T) {
	t.Parallel()
	t.Run("confirmed opening transaction", func(t *testing.T) {
		t.Parallel()
		var (
			wantSwapID             = "swapId"
			wantTxID               = "txId"
			wantTxHex              = "txHex"
			wantscriptpubkey       = []byte{0x01, 0x02, 0x03}
			callbackChan           = make(chan string)
			targetTXHeight   int32 = 100
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
		err = r.StartWatchingTxs()
		assert.NoError(t, err)
		r.AddWaitForConfirmationTx(wantSwapID, wantTxID, 0, 0, wantscriptpubkey)
		headerResultChan <- &electrum.SubscribeHeadersResult{
			Height: onchain.LiquidConfs + targetTXHeight + 1,
		}

		assert.Equal(t, <-callbackChan, wantSwapID)
	})

	t.Run("confirmed csv transaction", func(t *testing.T) {
		t.Parallel()
		var (
			wantSwapID       = "swapId"
			wantTxID         = "txId"
			wantTxHex        = "txHex"
			wantscriptpubkey = []byte{0x01, 0x02, 0x03}
			callbackChan     = make(chan string)
			targetTXHeight   = int32(100)
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
		r.AddCsvCallback(
			func(swapId string) error {
				assert.Equal(t, wantSwapID, swapId)
				assert.NoError(t, err)
				callbackChan <- swapId
				return nil
			},
		)
		err = r.StartWatchingTxs()
		assert.NoError(t, err)
		r.AddWaitForCsvTx(wantSwapID, wantTxID, 0, 0, wantscriptpubkey)
		headerResultChan <- &electrum.SubscribeHeadersResult{
			Height: onchain.LiquidCsv + targetTXHeight + 1,
		}

		assert.Equal(t, <-callbackChan, wantSwapID)
	})

}
