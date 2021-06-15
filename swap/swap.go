package swap

import (
	"crypto/rand"
	"encoding/hex"
)

type SwapType int

func (s SwapType) ToString() string {
	switch s {
	case SWAPTYPE_OUT:
		return "swap out"
	case SWAPTYPE_IN:
		return "swap in"
	}
	return ""
}

type SwapState int

func (s SwapState) ToString() string {
	switch s {
	case SWAPSTATE_CREATED:
		return "created"
	case SWAPSTATE_REQUEST_SENT:
		return "request sent"
	case SWAPSTATE_REQUEST_RECEIVED:
		return "request received"
	case SWAPSTATE_OPENING_TX_PREPARED:
		return "opening tx prepared"
	case SWAPSTATE_OPENING_TX_BROADCASTED:
		return "opening tx broadcasted"
	case SWAPSTATE_WAITING_FOR_TX:
		return "waiting for opening tx"
	case SWAPSTATE_CLAIMED_PREIMAGE:
		return "claimed with preimage"
	case SWAPSTATE_CLAIMED_TIMELOCK:
		return "claimed with cltv"
	case SWAPSTATE_CANCELED:
		return "canceled"
	}
	return ""
}

type SwapRole int

type ClaimType int

// SwapType in means the initiator wants to pay lbtc to rebalance the channel to his side
// swap out means the initiator wants to pay an invoice to rebalance the the channel to his peer
const (
	SWAPTYPE_IN SwapType = iota
	SWAPTYPE_OUT



	MESSAGETYPE_SWAPREQUEST   = "a455"
	MESSAGETYPE_MAKERRESPONSE = "a457"
	MESSAGETYPE_TAKERRESPONSE = "a459"
	MESSAGETYPE_ERRORRESPONSE = "a461"
	MESSAGETYPE_CLAIMED       = "a463"

)
const (
	SWAPSTATE_CREATED SwapState = iota
	SWAPSTATE_REQUEST_SENT
	SWAPSTATE_REQUEST_RECEIVED
	SWAPSTATE_OPENING_TX_PREPARED
	SWAPSTATE_OPENING_TX_BROADCASTED
	SWAPSTATE_WAITING_FOR_TX
	SWAPSTATE_CLAIMED_PREIMAGE
	SWAPSTATE_CLAIMED_TIMELOCK
	SWAPSTATE_CANCELED
)
const (
	CLAIMTYPE_PREIMAGE = iota
	CLAIMTYPE_CLTV
)

// Swap defines a swap process
type Swap struct {
	Id              string
	Type            SwapType
	State           SwapState
	InitiatorNodeId string
	PeerNodeId      string
	Amount          uint64
	ChannelId       string

	Payreq   string
	PreImage string
	PHash    string

	// Script
	MakerPubkeyHash string
	TakerPubkeyHash string

	Cltv int64

	OpeningTxId string

	ClaimTxId string
}

type PrettyPrintSwap struct {
	Id              string
	Type            string
	State           string
	InitiatorNodeId string
	PeerNodeId      string
	Amount          uint64
	ShortChannelId  string

	OpeningTxId string
	ClaimTxId   string

	CltvHeight int64
}

func (s *Swap) ToPrettyPrint() *PrettyPrintSwap {
	return &PrettyPrintSwap{
		Id:              s.Id,
		Type:            s.Type.ToString(),
		State:           s.State.ToString(),
		InitiatorNodeId: s.InitiatorNodeId,
		PeerNodeId:      s.PeerNodeId,
		Amount:          s.Amount,
		ShortChannelId:  s.ChannelId,
		OpeningTxId:     s.OpeningTxId,
		ClaimTxId:       s.ClaimTxId,
		CltvHeight:      s.Cltv,
	}
}

// NewSwap returns a new swap with a random hex id and the given arguments
func NewSwap(swapType SwapType, amount uint64, initiatorNodeId string, peerNodeId string, channelId string) *Swap {
	return &Swap{
		Id:              newSwapId(),
		Type:            swapType,
		State:           SWAPSTATE_CREATED,
		PeerNodeId:      peerNodeId,
		InitiatorNodeId: initiatorNodeId,
		ChannelId:       channelId,
		Amount:          amount,
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

func (s *SwapRequest) MessageType() string {
	return MESSAGETYPE_SWAPREQUEST
}

// MakerResponse is the response if the requester wants to swap out.
type MakerResponse struct {
	SwapId          string
	MakerPubkeyHash string
	Invoice         string
	TxId            string
	Cltv            int64
}

func (m *MakerResponse) MessageType() string {
	return MESSAGETYPE_MAKERRESPONSE
}

// TakerResponse is the response if the requester wants to swap in
type TakerResponse struct {
	SwapId          string
	TakerPubkeyHash string
}

func (t *TakerResponse) MessageType() string {
	return MESSAGETYPE_TAKERRESPONSE
}

type ClaimedMessage struct {
	SwapId    string
	ClaimType ClaimType
	ClaimTxId string
}

func (s *ClaimedMessage) MessageType() string {
	return MESSAGETYPE_CLAIMED
}

type ErrorResponse struct {
	SwapId string
	Error  string
}

func (e *ErrorResponse) MessageType() string {
	return MESSAGETYPE_ERRORRESPONSE
}
