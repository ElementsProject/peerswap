package swap

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/btcsuite/btcd/btcec"
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
	CLAIMTYPE_PREIMAGE = iota
	CLAIMTYPE_CSV
)

// SwapData holds all the data needed for a swap
type SwapData struct {
	Id              string
	Asset           string
	ProtocolVersion uint64
	Type            SwapType
	FSMState        StateType
	Role            SwapRole
	CreatedAt       int64
	InitiatorNodeId string
	PeerNodeId      string
	Amount          uint64
	ChannelId       string

	PrivkeyBytes []byte

	ClaimInvoice     string
	ClaimPreimage    string
	ClaimPaymentHash string

	// Script
	MakerPubkeyHash string
	TakerPubkeyHash string

	MakerRefundAddr    string
	TakerRefundSigHash string

	Csv uint32

	FeeInvoice  string
	FeePreimage string

	OpeningTxId            string
	OpeningTxUnpreparedHex string
	OpeningTxVout          uint32
	OpeningTxFee           uint64
	OpeningTxHex           string

	ClaimTxId string

	NextMessage     []byte
	NextMessageType int

	CancelMessage string

	LastErr       error  `json:"-"`
	LastErrString string `json:"last_err,omitempty"`
}

func (s *SwapData) GetId() string {
	return s.Id
}

func (s *SwapData) SetState(stateType StateType) {
	s.FSMState = stateType
}
func (s *SwapData) GetCurrentState() StateType {
	return s.FSMState
}

func (s *SwapData) GetOpeningParams() *OpeningParams {
	return &OpeningParams{
		TakerPubkeyHash:  s.TakerPubkeyHash,
		MakerPubkeyHash:  s.MakerPubkeyHash,
		ClaimPaymentHash: s.ClaimPaymentHash,
		Amount:           s.Amount,
	}
}

type PrettyPrintSwapData struct {
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

	Csv uint32 `json:",omitempty"`

	CancelMessage string `json:",omitempty"`
}

func (s *SwapData) ToPrettyPrint() *PrettyPrintSwapData {
	timeStamp := time.Unix(s.CreatedAt, 0)
	if s.LastErr != nil {
		s.LastErrString = s.LastErr.Error()
	}
	return &PrettyPrintSwapData{
		Id:              s.Id,
		Type:            s.Type.String(),
		Role:            s.Role.String(),
		State:           string(s.FSMState),
		InitiatorNodeId: s.InitiatorNodeId,
		PeerNodeId:      s.PeerNodeId,
		Amount:          s.Amount,
		ShortChannelId:  s.ChannelId,
		OpeningTxId:     s.OpeningTxId,
		ClaimTxId:       s.ClaimTxId,
		Csv:             s.Csv,
		CreatedAt:       timeStamp.String(),
		CancelMessage:   s.LastErrString,
	}
}

func (s *SwapData) GetPrivkey() *btcec.PrivateKey {
	privkey, _ := btcec.PrivKeyFromBytes(btcec.S256(), s.PrivkeyBytes)
	return privkey
}

// NewSwap returns a new swap with a random hex id and the given arguments
func NewSwap(swapId string, asset string, swapType SwapType, swapRole SwapRole, amount uint64, initiatorNodeId string, peerNodeId string, channelId string, protocolVersion uint64) *SwapData {
	return &SwapData{
		Id:              swapId,
		Asset:           asset,
		Role:            swapRole,
		Type:            swapType,
		PeerNodeId:      peerNodeId,
		InitiatorNodeId: initiatorNodeId,
		ChannelId:       channelId,
		Amount:          amount,
		PrivkeyBytes:    getRandomPrivkey().Serialize(),
		CreatedAt:       time.Now().Unix(),
		ProtocolVersion: protocolVersion,
	}
}

// NewSwapFromRequest returns a new swap created from a swap request
func NewSwapFromRequest(senderNodeId string, asset string, swapId string, amount uint64, channelId string, swapType SwapType, protocolVersion uint64) *SwapData {
	return &SwapData{
		Id:              swapId,
		Asset:           asset,
		Type:            swapType,
		PeerNodeId:      senderNodeId,
		InitiatorNodeId: senderNodeId,
		Amount:          amount,
		ChannelId:       channelId,
		CreatedAt:       time.Now().Unix(),
		PrivkeyBytes:    getRandomPrivkey().Serialize(),
		ProtocolVersion: protocolVersion,
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
