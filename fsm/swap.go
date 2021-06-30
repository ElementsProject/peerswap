package fsm

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"time"
)

type SwapType int

func (s SwapType) String() string {
	switch s {
	case SWAPTYPE_OUT:
		return "swap out"
	case SWAPTYPE_IN:
		return "swap in"
	}
	return ""
}

type SwapState int

func (s SwapState) String() string {
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
	case SWAPSTATE_WAITING_FOR_TX_CONFS:
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

func (s SwapRole) String() string {
	switch s {
	case SWAPROLE_SENDER:
		return "sender"
	case SWAPROLE_RECEIVER:
		return "receiver"
	}
	return ""
}

type ClaimType int

// SwapType in means the initiator wants to pay lbtc to rebalance the channel to his side
// swap out means the initiator wants to pay an invoice to rebalance the the channel to his peer
const (
	SWAPTYPE_IN SwapType = iota
	SWAPTYPE_OUT
)
const (
	SWAPROLE_SENDER SwapRole = iota
	SWAPROLE_RECEIVER
)
const (
	SWAPSTATE_CREATED SwapState = iota
	SWAPSTATE_REQUEST_SENT
	SWAPSTATE_REQUEST_RECEIVED
	SWAPSTATE_OPENING_TX_PREPARED
	SWAPSTATE_FEE_INVOICE_PAID
	SWAPSTATE_OPENING_TX_BROADCASTED
	SWAPSTATE_WAITING_FOR_TX_CONFS
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
	FSMState        StateType
	Role            SwapRole
	CreatedAt       int64
	InitiatorNodeId string
	PeerNodeId      string
	Amount          uint64
	ChannelId       string

	PrivkeyBytes []byte

	Payreq   string
	PreImage string
	PHash    string

	// Script
	MakerPubkeyHash string
	TakerPubkeyHash string

	Cltv int64

	FeeInvoice  string
	FeePreimage string

	OpeningTxId            string
	OpeningTxUnpreparedHex string
	OpeningTxVout          uint32
	OpeningTxFee           uint64
	OpeningTxHex           string

	ClaimTxId string

	CancelMessage string
}

func (s *Swap) GetId() string {
	return s.Id
}

func (s *Swap) SetState(stateType StateType) {
	s.FSMState = stateType
}
func (s *Swap) GetCurrentState() StateType {
	return s.FSMState
}

type PrettyPrintSwap struct {
	Id              string
	CreatedAt       string
	Type            string
	Role            string
	State           string
	InitiatorNodeId string
	PeerNodeId      string
	Amount          uint64
	ShortChannelId  string

	OpeningTxId string `json:",omitempty"`

	ClaimTxId string `json:",omitempty"`

	CltvHeight int64 `json:",omitempty"`

	CancelMessage string `json:",omitempty"`
}

func (s *Swap) ToPrettyPrint() *PrettyPrintSwap {
	timeStamp := time.Unix(s.CreatedAt, 0)
	return &PrettyPrintSwap{
		Id:              s.Id,
		Type:            fmt.Sprintf("%s", s.Type),
		Role:            s.Role.String(),
		State:           s.State.String(),
		InitiatorNodeId: s.InitiatorNodeId,
		PeerNodeId:      s.PeerNodeId,
		Amount:          s.Amount,
		ShortChannelId:  s.ChannelId,
		OpeningTxId:     s.OpeningTxId,
		ClaimTxId:       s.ClaimTxId,
		CltvHeight:      s.Cltv,
		CreatedAt:       timeStamp.String(),
		CancelMessage:   s.CancelMessage,
	}
}

func (s *Swap) GetPrivkey() *btcec.PrivateKey {
	privkey, _ := btcec.PrivKeyFromBytes(btcec.S256(), s.PrivkeyBytes)
	return privkey
}

// NewSwap returns a new swap with a random hex id and the given arguments
func NewSwap(swapId string, swapType SwapType, swapRole SwapRole, amount uint64, initiatorNodeId string, peerNodeId string, channelId string) *Swap {
	return &Swap{
		Id:              swapId,
		Role:            swapRole,
		Type:            swapType,
		State:           SWAPSTATE_CREATED,
		PeerNodeId:      peerNodeId,
		InitiatorNodeId: initiatorNodeId,
		ChannelId:       channelId,
		Amount:          amount,
		PrivkeyBytes:    getRandomPrivkey().Serialize(),
		CreatedAt:       time.Now().Unix(),
	}
}

func NewSwapFromRequest(senderNodeId string, swapId string, amount uint64, channelId string, swapType SwapType) *Swap {
	return &Swap{
		Id:              swapId,
		Type:            swapType,
		State:           SWAPSTATE_REQUEST_RECEIVED,
		PeerNodeId:      senderNodeId,
		InitiatorNodeId: senderNodeId,
		Amount:          amount,
		ChannelId:       channelId,
		CreatedAt:       time.Now().Unix(),
		PrivkeyBytes:    getRandomPrivkey().Serialize(),
	}
}

// newSwapId returns a random 32 byte hex string
func newSwapId() string {
	idBytes := make([]byte, 32)
	_, _ = rand.Read(idBytes[:])
	return hex.EncodeToString(idBytes)
}

// getRandomPrivkey returns a random private key for the swap
func getRandomPrivkey() *btcec.PrivateKey {
	privkey, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		return nil
	}
	return privkey
}
