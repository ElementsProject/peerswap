package swap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/elementsproject/peerswap/lightning"
)

type SwapType int

func (s SwapType) String() string {
	switch s {
	case SWAPTYPE_OUT:
		return "swap-out"
	case SWAPTYPE_IN:
		return "swap-in"
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
	SWAPTYPE_IN SwapType = iota + 1
	SWAPTYPE_OUT
)
const (
	SWAPROLE_SENDER SwapRole = iota + 1
	SWAPROLE_RECEIVER
)
const (
	CLAIMTYPE_PREIMAGE = iota + 1
	CLAIMTYPE_CSV
)

type InvoiceType int

const (
	INVOICE_CLAIM InvoiceType = iota + 1
	INVOICE_FEE
)

func (i InvoiceType) String() string {
	switch i {
	case INVOICE_CLAIM:
		return "claim"
	case INVOICE_FEE:
		return "fee"
	}
	return ""
}

// SwapData holds all the data needed for a swap
type SwapData struct {
	// Swap In
	SwapInRequest   *SwapInRequestMessage   `json:"swap_in_request"`
	SwapInAgreement *SwapInAgreementMessage `json:"swap_in_agreement"`

	// Swap Out
	SwapOutRequest   *SwapOutRequestMessage   `json:"swap_out_request"`
	SwapOutAgreement *SwapOutAgreementMessage `json:"swap_out_agreement"`

	// TxOpened
	OpeningTxBroadcasted *OpeningTxBroadcastedMessage `json:"opening_tx_broadcasted"`

	// CoopClose
	CoopClose *CoopCloseMessage `json:"coop_close_message"`

	// Cancel
	Cancel *CancelMessage `json:"cancel_message_obj"`

	// cancel message
	CancelMessage string `json:"cancel_message"`

	PeerNodeId          string    `json:"peer_nod_id"`
	InitiatorNodeId     string    `json:"initiator_node_id"`
	CreatedAt           int64     `json:"created_at"`
	Role                SwapRole  `json:"role"`
	FSMState            StateType `json:"fsm_state"`
	PrivkeyBytes        []byte
	FeePreimage         string `json:"fee_preimage"`
	OpeningTxFee        uint64 `json:"opening_tx_fee"`
	OpeningTxHex        string `json:"opening_tx-hex"`
	StartingBlockHeight uint32 `json:"opening_block_height"`
	ClaimTxId           string `json:"claim_tx_id"`
	ClaimPaymentHash    string `json:"claim_payment_hash"`
	ClaimPreimage       string `json:"claim_preimage"`

	BlindingKeyHex string `json:"blinding_key"`

	LastMessage EventContext `json:"last_message"`

	NextMessage     []byte
	NextMessageType int

	LastErr       error  `json:"-"`
	LastErrString string `json:"last_err,omitempty"`

	// TimeOut cancel func. If set and called cancels the timout context so that
	// the TimeOut callback does not get called after cancel.
	toCancel context.CancelFunc
}

func (s *SwapData) GetId() *SwapId {
	if s.SwapInRequest != nil {
		return s.SwapInRequest.SwapId
	}
	if s.SwapOutRequest != nil {
		return s.SwapOutRequest.SwapId
	}
	if s.SwapInAgreement != nil {
		return s.SwapInAgreement.SwapId
	}
	if s.SwapOutAgreement != nil {
		return s.SwapOutAgreement.SwapId
	}
	return nil
}

func (s *SwapData) GetProtocolVersion() uint8 {
	if s.SwapInRequest != nil {
		return s.SwapInRequest.ProtocolVersion
	}
	if s.SwapOutRequest != nil {
		return s.SwapOutRequest.ProtocolVersion
	}
	if s.SwapInAgreement != nil {
		return s.SwapInAgreement.ProtocolVersion
	}
	if s.SwapOutAgreement != nil {
		return s.SwapOutAgreement.ProtocolVersion
	}
	return 0
}

func (s *SwapData) GetType() SwapType {
	if s.SwapInRequest != nil {
		return SWAPTYPE_IN
	}
	if s.SwapOutRequest != nil {
		return SWAPTYPE_OUT
	}
	return 0
}

func (s *SwapData) GetScid() string {
	if s.SwapInRequest != nil {
		return s.SwapInRequest.Scid
	}
	if s.SwapOutRequest != nil {
		return s.SwapOutRequest.Scid
	}
	return ""
}

func (s *SwapData) GetScidInBoltFormat() string {
	if s.SwapInRequest != nil {
		return strings.ReplaceAll(s.SwapInRequest.Scid, ":", "x")
	}
	if s.SwapOutRequest != nil {
		return strings.ReplaceAll(s.SwapOutRequest.Scid, ":", "x")
	}
	return ""
}

func (s *SwapData) GetAmount() uint64 {
	if s.SwapInRequest != nil {
		return s.SwapInRequest.Amount
	}
	if s.SwapOutRequest != nil {
		return s.SwapOutRequest.Amount
	}
	return 0
}

func (s *SwapData) GetAsset() string {
	if s.SwapInRequest != nil {
		return s.SwapInRequest.Asset
	}
	if s.SwapOutRequest != nil {
		return s.SwapOutRequest.Asset
	}
	return ""
}

func (s *SwapData) GetInvoiceExpiry() uint64 {
	var expiry uint64
	switch s.GetChain() {
	case btc_chain:
		expiry = 3600 * 24
	case l_btc_chain:
		expiry = 3600
	default:
		expiry = 0
	}
	return expiry
}

func (s *SwapData) GetNetwork() string {
	if s.SwapInRequest != nil {
		return s.SwapInRequest.Network
	}
	if s.SwapOutRequest != nil {
		return s.SwapOutRequest.Network
	}
	return ""
}

func (s *SwapData) GetChain() string {
	if s.GetAsset() != "" && s.GetNetwork() == "" {
		return l_btc_chain
	} else if s.GetAsset() == "" && s.GetNetwork() != "" {
		return btc_chain
	} else {
		return ""
	}

}

func (s *SwapData) GetMakerPubkey() string {
	if s.SwapInRequest != nil {
		return s.SwapInRequest.Pubkey
	}
	if s.SwapOutAgreement != nil {
		return s.SwapOutAgreement.Pubkey
	}
	return ""
}

func (s *SwapData) GetTakerPubkey() string {
	if s.SwapOutRequest != nil {
		return s.SwapOutRequest.Pubkey
	}
	if s.SwapInAgreement != nil {
		return s.SwapInAgreement.Pubkey
	}
	return ""
}

func (s *SwapData) GetPreimage() string {
	return s.ClaimPreimage
}

func (s *SwapData) GetPaymentHash() string {
	if s.ClaimPaymentHash != "" {
		return s.ClaimPaymentHash
	}
	if s.ClaimPreimage != "" {
		preimage, _ := lightning.MakePreimageFromStr(s.ClaimPreimage)
		return preimage.Hash().String()
	}
	return ""
}

func (s *SwapData) SetState(stateType StateType) {
	s.FSMState = stateType
}
func (s *SwapData) GetCurrentState() StateType {
	return s.FSMState
}

func (s *SwapData) GetRequest() PeerMessage {
	if s.SwapInRequest != nil {
		return s.SwapInRequest
	}
	if s.SwapOutRequest != nil {
		return s.SwapOutRequest
	}
	return nil
}

func (s *SwapData) GetOpeningParams() *OpeningParams {
	var blindingKey *btcec.PrivateKey
	if s.OpeningTxBroadcasted != nil && s.OpeningTxBroadcasted.BlindingKey != "" {
		blindingKeyBytes, _ := hex.DecodeString(s.OpeningTxBroadcasted.BlindingKey)
		blindingKey, _ = btcec.PrivKeyFromBytes(btcec.S256(), blindingKeyBytes)
	} else if s.BlindingKeyHex != "" {
		blindingKeyBytes, _ := hex.DecodeString(s.BlindingKeyHex)
		blindingKey, _ = btcec.PrivKeyFromBytes(btcec.S256(), blindingKeyBytes)
	}

	return &OpeningParams{
		TakerPubkey:      s.GetTakerPubkey(),
		MakerPubkey:      s.GetMakerPubkey(),
		ClaimPaymentHash: s.GetPaymentHash(),
		Amount:           s.GetAmount(),
		BlindingKey:      blindingKey,
	}
}

func (s *SwapData) GetClaimParams() *ClaimParams {
	key, _ := btcec.PrivKeyFromBytes(btcec.S256(), s.PrivkeyBytes)

	claimParams := &ClaimParams{
		Preimage:     s.ClaimPreimage,
		Signer:       key,
		OpeningTxHex: s.OpeningTxHex,
	}

	return claimParams
}

func (s *SwapData) GetOpeningTxId() string {
	if s.OpeningTxBroadcasted != nil {
		return s.OpeningTxBroadcasted.TxId
	}
	return ""
}

func (s *SwapData) GetCancelMessage() string {
	if s.Cancel != nil {
		return s.Cancel.Message
	}

	if s.LastErr != nil {
		return s.LastErr.Error()
	}

	if s.CancelMessage != "" {
		return s.CancelMessage
	}

	return ""
}

func (s *SwapData) cancelTimeout() {
	if s.toCancel != nil {
		s.toCancel()
	}
}

type PrettyPrintSwapData struct {
	Id              string `json:"id"`
	Asset           string `json:"asset"`
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

	return &PrettyPrintSwapData{
		Id:              s.GetId().String(),
		Asset:           s.GetChain(),
		Type:            s.GetType().String(),
		Role:            s.Role.String(),
		State:           string(s.FSMState),
		InitiatorNodeId: s.InitiatorNodeId,
		PeerNodeId:      s.PeerNodeId,
		Amount:          s.GetAmount(),
		ShortChannelId:  s.GetScid(),
		OpeningTxId:     s.GetOpeningTxId(),
		ClaimTxId:       s.ClaimTxId,
		CreatedAt:       timeStamp.String(),
		CancelMessage:   s.GetCancelMessage(),
	}
}

func (s *SwapData) GetPrivkey() *btcec.PrivateKey {
	privkey, _ := btcec.PrivKeyFromBytes(btcec.S256(), s.PrivkeyBytes)
	return privkey
}

// NewSwapData returns a new swap with a random hex id and the given arguments
func NewSwapData(swapId *SwapId, initiatorNodeId string, peerNodeId string) *SwapData {
	return &SwapData{
		PeerNodeId:      peerNodeId,
		InitiatorNodeId: initiatorNodeId,
		PrivkeyBytes:    getRandomPrivkey().Serialize(),
		CreatedAt:       time.Now().Unix(),
		Role:            SWAPROLE_SENDER,
	}
}

// NewSwapDataFromRequest returns a new swap created from a swap request
func NewSwapDataFromRequest(swapId *SwapId, senderNodeId string) *SwapData {
	return &SwapData{
		PeerNodeId:      senderNodeId,
		InitiatorNodeId: senderNodeId,
		CreatedAt:       time.Now().Unix(),
		PrivkeyBytes:    getRandomPrivkey().Serialize(),
		Role:            SWAPROLE_RECEIVER,
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
	if s == nil || len(s) == 0 {
		return ""
	}
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

type SwapErrorContext struct {
	Err      error
	SendPeer bool
}

func (s SwapErrorContext) ApplyToSwapData(data *SwapData) error {
	if s.Err != nil {
		data.LastErr = s.Err
		data.LastErrString = s.Err.Error()
		if s.SendPeer {
			data.CancelMessage = s.Err.Error()
		}
	}
	return nil
}

func (s *SwapErrorContext) Validate(data *SwapData) error {
	return nil
}
