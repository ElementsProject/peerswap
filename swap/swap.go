package swap

import (
	"crypto/rand"
	"encoding/hex"
)

type SwapType byte

type SwapState int

type SwapRole byte

// SwapType in means the initiator wants to pay lbtc to rebalance the channel to his side
// swap out means the initiator wants to pay an invoice to rebalance the the channel to his peer
const (
	SWAPTYPE_IN SwapType = iota
	SWAPTYPE_OUT

	SWAPSTATE_CREATED SwapState = iota
	SWAPSTATE_REQUEST_SENT
	SWAPSTATE_REQUEST_RECEIVED
	SWAPSTATE_OPENING_TX_PREPARED
	SWAPSTATE_CLAIMED_PREIMAGE
	SWAPSTATE_CLAIMED_TIMELOCK
	SWAPSTATE_CANCELED
)

// Swap defines a swap process
type Swap struct {
	Id         string
	Type       SwapType
	State      SwapState
	PeerNodeId string
	Amount     uint64
	ChannelId  string

	Payreq string
	PHash  []byte

	// Script
	MakerPubkeyHash string
	TakerPubkeyHash string

	OpeningTxId  string
	OpeningTxHex string

	ClaimTxId  string
	ClaimTxHex string
}

// NewSwap returns a new swap with a random hex id and the given arguments
func NewSwap(swapType SwapType, amount uint64, peerNodeId string, channelId string) *Swap {
	return &Swap{
		Id:         newSwapId(),
		Type:       swapType,
		State:      SWAPSTATE_CREATED,
		PeerNodeId: peerNodeId,
		ChannelId:  channelId,
		Amount:     amount,
	}
}

// newSwapId returns a random 32 byte hex string
func newSwapId() string {
	idBytes := make([]byte, 32)
	_, _ = rand.Read(idBytes[:])
	return hex.EncodeToString(idBytes)
}

// SwapRequest gets send when a peer wants to start a new swap.
type SwapRequest struct {
	SwapId          string
	ChannelId       string
	Amount          uint64
	Type            SwapType
	TakerPubkeyHash string
}

// MakerResponse is the response if the requester wants to swap out.
type MakerResponse struct {
	SwapId          string
	MakerPubkeyHash string
	Invoice         string
	TxId            string
}

// TakerResponse is the response if the requester wants to swap in
type TakerResponse struct {
	SwapId          string
	TakerPubkeyHash string
}
