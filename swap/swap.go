package swap

import (
	"crypto/rand"
	"encoding/hex"
)

type SwapType byte

type SwapState int


// Swap in means the initiator wants to pay lbtc to rebalance the channel to his side
// swap out means the initiator wants to pay an invoice to rebalance the the channel to his peer
const (
	SWAPTYPE_IN SwapType = iota
	SWAPTYPE_OUT

	SWAPSTATE_CREATED SwapState = iota
	SWAPSTATE_INITIATED
	SWAPSTATE_OPENING_TX_BROADCASTED
	SWAPSTATE_CLAIMED_PREIMAGE
	SWAPSTATE_CLAIMED_TIMELOCK
	SWAPSTATE_CANCELED
)

// Swap defines a Swapprocess
type Swap struct {
	Id string
	Type SwapType
	State SwapState
	MakerNodeId string
	TakerNodeId string
	Amount uint64

	Invoice string

	// Script
	SpendingScript []byte

	OpeningTxId string
	OpeningTxHex string

	ClaimTxId string
	ClaimTxHex string
}

// NewSwap returns a new swap with a random hex id and the given arguments
func NewSwap(swapType SwapType, maker string, taker string, amount uint64) (*Swap, error) {
	swapId, err := newSwapId()
	if err != nil {
		return nil, err
	}
	return &Swap {
		Id: swapId,
		Type: swapType,
		State: SWAPSTATE_CREATED,
		MakerNodeId: maker,
		TakerNodeId: taker,
		Amount: amount,
	}, nil
}

// newSwapId returns a random 32 byte hex string
func newSwapId() (string, error) {
	idBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(idBytes), nil
}



