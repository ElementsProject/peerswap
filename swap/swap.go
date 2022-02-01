package swap

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

func (s SwapType) JsonFieldValue() string {
	switch s {
	case SWAPTYPE_OUT:
		return "swap_out"
	case SWAPTYPE_IN:
		return "swap_in"
	default:
		return "unknown_swap_type"
	}
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
	Id              string    `json:"id"`
	Asset           string    `json:"asset"`
	ProtocolVersion uint64    `json:"protocol_version"`
	Type            SwapType  `json:"type"`
	FSMState        StateType `json:"fsm_state"`
	Role            SwapRole  `json:"role"`
	CreatedAt       int64     `json:"created_at"`
	InitiatorNodeId string    `json:"initiator_node_id"`
	PeerNodeId      string    `json:"peer_nod_id"`
	Amount          uint64    `json:"amount"`
	ChannelId       string    `json:"channel_id"`

	PrivkeyBytes []byte

	ClaimInvoice     string `json:"claim_invoice"`
	ClaimPreimage    string `json:"claim_preimage"`
	ClaimPaymentHash string `json:"claim_payment_hash"`

	// Script
	MakerPubkeyHash string `json:"maker_pubkey_hash"`
	TakerPubkeyHash string `json:"taker_pubkey_hash"`

	TakerPrivkey string `json:"taker_priv_key"`

	FeeInvoice  string `json:"fee_invoice"`
	FeePreimage string `json:"fee_preimage"`

	OpeningTxId            string `json:"opening_tx_id"`
	OpeningTxUnpreparedHex string `json:"opening_tx_unprepped_hex"`
	OpeningTxVout          uint32 `json:"opening_tx_vout"`
	OpeningTxFee           uint64 `json:"opening_tx_fee"`
	OpeningTxHex           string `json:"opening_tx-hex"`
	StartingBlockHeight    uint32 `json:"opening_block_height"`

	BlindingKeyHex string `json:"blinding_key_hex,omitempty"`

	ClaimTxId string `json:"claim_tx_id"`

	NextMessage     []byte
	NextMessageType int

	CancelMessage string `json:"cancel_message"`

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
	var blindingKey *btcec.PrivateKey
	if s.BlindingKeyHex != "" {
		blindingKeyBytes, _ := hex.DecodeString(s.BlindingKeyHex)
		blindingKey, _ = btcec.PrivKeyFromBytes(btcec.S256(), blindingKeyBytes)
	}
	return &OpeningParams{
		TakerPubkeyHash:  s.TakerPubkeyHash,
		MakerPubkeyHash:  s.MakerPubkeyHash,
		ClaimPaymentHash: s.ClaimPaymentHash,
		Amount:           s.Amount,
		BlindingKey:      blindingKey,
	}
}

type PrettyPrintSwapData struct {
	Id              string `json:"id"`
	CreatedAt       string `json:"created_at"`
	Type            string `json:"type"`
	Role            string `json:"role"`
	State           string `json:"state"`
	InitiatorNodeId string `json:"initiator_node_id"`
	PeerNodeId      string `json:"peer_node_id"`
	Amount          uint64 `json:"amount"`
	ShortChannelId  string `json:"short_channel_id"`

	OpeningTxId string `json:"opening_tx_id,omitempty"`

	ClaimTxId string `json:"claim_tx_id,omitempty"`

	CancelMessage string `json:"cancel_message,omitempty"`
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

type SwapId [32]byte

func NewSwapId() *SwapId {
	var swapId *SwapId = new(SwapId)
	rand.Read(swapId[:])
	return swapId
}

func (s *SwapId) String() string {
	return hex.EncodeToString(s[:])
}

func (s *SwapId) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *SwapId) UnmarshalJSON(data []byte) error {
	var result string
	err := json.Unmarshal(data, &result)
	if err != nil {
		return err
	}
	return s.FromString(result)
}

func (s *SwapId) FromString(str string) error {
	data, err := hex.DecodeString(str)
	if err != nil {
		return err
	}
	if len(data) != 32 {
		return fmt.Errorf("can not decode string: invalid length")
	}
	copy(s[:], data[:])
	return nil
}

func ParseSwapIdFromString(str string) (*SwapId, error) {
	data, err := hex.DecodeString(str)
	if err != nil {
		return nil, err
	}
	if len(data) != 32 {
		return nil, fmt.Errorf("can not decode string: invalid length")
	}
	var swapId *SwapId = new(SwapId)
	copy(swapId[:], data[:])
	return swapId, err
}
