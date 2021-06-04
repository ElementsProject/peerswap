package swap

import "context"

type SwapType int
const (
	SWAP_IN SwapType = iota
	SWAP_OUT
)
type InitializeSwapRequest struct {
	SwapId string
	Type SwapType
	Amount uint64
}

type FeeInvoice struct {
	SwapId string
	Invoice string
}

type OpenTransactionMessage struct {
	SwapId string
	TxHex string
}

type AbortMessage struct {
	SwapId string
	TxId string
}

type ClaimedMessage struct {
	SwapId string
	TxId string
}

type Swapper interface {
	PostOpenFundingTx(ctx context.Context, amt int64, fee int64) (txId string, err error)

}
