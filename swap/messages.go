package swap

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/elementsproject/peerswap/messages"
)

var (
	AlreadyExistsError     = errors.New("Message already exists")
	InvalidLengthError     = errors.New("Hex string is of invalid length")
	InvalidNetworkError    = errors.New("Invalid network")
	InvalidScidError       = errors.New("Invalid Scid")
	AssetOrNetworkSetError = errors.New("Either asset or network must be set")
)

func NewInvalidLengthError(paramName string, expected, actual int) error {
	return fmt.Errorf("Param %s is of invalid length expected: %v, actual %v", paramName, expected, actual)
}

type SwapInRequestMessage struct {
	// ProtocolVersion is the version of the PeerSwap peer protocol the sending
	// node uses.
	ProtocolVersion uint8 `json:"protocol_version"`
	// SwapId is a randomly generated 32 byte string that must be kept the same
	// through the whole process of a swap and serves as an identifier for a
	// specific swap.
	SwapId *SwapId `json:"swap_id"`
	// Network is the desired on-chain network to use. This can be:
	// Bitcoin: mainnet, testnet, signet, regtest
	// Liquid: The field is left blank as the asset id also defines the bitcoinNetwork.
	Network string `json:"network"`
	// Asset is the desired on-chain asset to use. This can be:
	// Bitcoin: The field is left blank.
	// Liquid: The asset id of the networks Bitcoin asset.
	Asset string `json:"asset"`
	// Scid is the short channel id in human readable format, defined by BOLT#7
	// with x as separator, e.g. 539268x845x1.
	Scid string `json:"scid"`
	// Amount is The amount in Sats that is asked for.
	Amount       uint64 `json:"amount"`
	Pubkey       string `json:"pubkey"`
	PremiumLimit int64  `json:"acceptable_premium"`
}

func (s SwapInRequestMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPINREQUEST
}

func (s SwapInRequestMessage) Validate(swap *SwapData) error {
	err := validateHexString("pubkey", s.Pubkey, 33)
	if err != nil {
		return err
	}
	err = validateAssetAndNetwork(s.Asset, s.Network)
	if err != nil {
		return err
	}
	if s.Asset != "" {
		err = validateHexString("asset", s.Asset, 33)
		if err != nil {
			return err
		}
	}
	if s.Network != "" {
		err = validateNetwork(s.Network)
		if err != nil {
			return err
		}
	}
	err = validateScid(s.Scid)
	if err != nil {
		return err
	}
	return nil
}

func validateScid(scid string) error {
	var prefix string
	if strings.Contains(scid, "x") {
		prefix = "x"
	} else if strings.Contains(scid, ":") {
		prefix = ":"
	} else {
		return InvalidScidError
	}
	parts := strings.Split(scid, prefix)
	if len(parts) != 3 {
		return InvalidScidError
	}
	_, err := strconv.Atoi(parts[0])
	_, err = strconv.Atoi(parts[1])
	_, err = strconv.Atoi(parts[2])
	if err != nil {
		return InvalidScidError
	}
	return nil
}
func validateAssetAndNetwork(asset string, network string) error {
	if (asset == "" && network == "") || (asset != "" && network != "") {
		return AssetOrNetworkSetError
	}
	if asset != "" {
		err := validateHexString("asset", asset, 33)
		if err != nil {
			return err
		}
	}
	if network != "" {
		err := validateNetwork(network)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateNetwork(network string) error {
	switch network {
	case "mainnet":
		fallthrough
	case "testnet":
		fallthrough
	case "testnet3":
		fallthrough
	case "testnet4":
		fallthrough
	case "signet":
		fallthrough
	case "regtest":
		return nil
	}
	return InvalidNetworkError
}
func validateHexString(paramName, hexString string, expectedLength int) error {
	data, err := hex.DecodeString(hexString)
	if err != nil {
		return err
	}
	if len(data) != expectedLength {
		return NewInvalidLengthError(paramName, expectedLength, len(data))
	}
	return nil
}

func (s SwapInRequestMessage) ApplyToSwapData(swap *SwapData) error {
	if swap.SwapInRequest != nil {
		return AlreadyExistsError
	}
	swap.SwapInRequest = &s
	return nil
}

// SwapInAgreementMessage is the response by the swap-in peer if he accepts the
// swap.
type SwapInAgreementMessage struct {
	// ProtocolVersion is the version of the PeerSwap peer protocol the sending
	// node uses.
	ProtocolVersion uint8 `json:"protocol_version"`
	// SwapId is a randomly generated 32 byte string that must be kept the same
	// through the whole process of a swap and serves as an identifier for a
	// specific swap.
	SwapId *SwapId `json:"swap_id"`
	// Pubkey is a 33 byte compressed public key used for the spending paths in
	// the opening_transaction.
	Pubkey string `json:"pubkey"`
	// Premium is a compensation in Sats that the swap partner wants to be paid
	// in order to participate in the swap.
	Premium int64 `json:"premium"`
}

func (s SwapInAgreementMessage) Validate(swap *SwapData) error {
	err := validateHexString("pubkey", s.Pubkey, 33)
	if err != nil {
		return err
	}

	return nil
}

func (s SwapInAgreementMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPINAGREEMENT
}

func (s SwapInAgreementMessage) ApplyToSwapData(swap *SwapData) error {
	if swap.SwapInAgreement != nil {
		return AlreadyExistsError
	}
	swap.SwapInAgreement = &s
	return nil
}

// SwapOutRequestMessage gets send when a peer wants to start a new swap.
type SwapOutRequestMessage struct {
	// ProtocolVersion is the version of the PeerSwap peer protocol the sending
	// node uses.
	ProtocolVersion uint8 `json:"protocol_version"`
	// SwapId is a randomly generated 32 byte string that must be kept the same
	// through the whole process of a swap and serves as an identifier for a
	// specific swap.
	SwapId *SwapId `json:"swap_id"`
	// Asset is the desired on-chain asset to use. This can be:
	// Bitcoin: The field is left blank.
	// Liquid: The asset id of the networks Bitcoin asset.
	Asset string `json:"asset"`
	// Network is the desired on-chain network to use. This can be:
	// Bitcoin: mainnet, testnet, signet, regtest
	// Liquid: The field is left blank as the asset id also defines the bitcoinNetwork.
	Network string `json:"network"`
	// Scid is the short channel id in human readable format, defined by BOLT#7
	// with x as separator, e.g. 539268x845x1.
	Scid string `json:"scid"`
	// Amount is The amount in Sats that is asked for.
	Amount uint64 `json:"amount"`
	// Pubkey is a 33 byte compressed public key used for the spending paths in
	// the opening_transaction.
	Pubkey       string `json:"pubkey"`
	PremiumLimit int64  `json:"acceptable_premium"`
}

func (s SwapOutRequestMessage) Validate(swap *SwapData) error {
	err := validateHexString("pubkey", s.Pubkey, 33)
	if err != nil {
		return err
	}
	err = validateAssetAndNetwork(s.Asset, s.Network)
	if err != nil {
		return err
	}
	err = validateScid(s.Scid)
	if err != nil {
		return err
	}
	return nil
}

func (s SwapOutRequestMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPOUTREQUEST
}

func (s SwapOutRequestMessage) ApplyToSwapData(swap *SwapData) error {
	if swap.SwapOutRequest != nil {
		return AlreadyExistsError
	}
	swap.SwapOutRequest = &s
	return nil
}

// SwapOutAgreementMessage is the response by the swap-out peer if he accepts
// the swap it contains an Invoice that the swap-out initiator must pay.
type SwapOutAgreementMessage struct {
	// ProtocolVersion is the version of the PeerSwap peer protocol the sending
	// node uses.
	ProtocolVersion uint8 `json:"protocol_version"`
	// SwapId is a randomly generated 32 byte string that must be kept the same
	// through the whole process of a swap and serves as an identifier for a
	// specific swap.
	SwapId *SwapId `json:"swap_id"`
	// Pubkey is a 33 byte compressed public key used for the spending paths in
	// the opening_transaction.
	Pubkey string `json:"pubkey"`
	// Payreq is a BOLT#11 invoice with an amount that covers the fee expenses
	// for the on-chain transactions.
	Payreq string
	// Premium is a compensation in Sats that the swap partner wants to be paid
	// in order to participate in the swap.
	Premium int64 `json:"premium"`
}

func (s SwapOutAgreementMessage) Validate(swap *SwapData) error {
	err := validateHexString("pubkey", s.Pubkey, 33)
	if err != nil {
		return err
	}
	return nil
}

func (s SwapOutAgreementMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPOUTAGREEMENT
}

func (s SwapOutAgreementMessage) ApplyToSwapData(swap *SwapData) error {
	if swap.SwapOutAgreement != nil {
		return AlreadyExistsError
	}
	swap.SwapOutAgreement = &s
	return nil
}

// OpeningTxBroadcastedMessage is the message sent by the creator of the opening
// tx.
type OpeningTxBroadcastedMessage struct {
	// SwapId is the unique identifier of the swap.
	SwapId *SwapId `json:"swap_id"`
	// Payreq is the invoice as described in BOLT#11 that the responder is
	// requested to pay.
	Payreq string `json:"payreq"`
	// TxId is the transaction id of the opening_transaction broadcasted by the
	// initiator.
	TxId string `json:"tx_id"`
	// ScriptOut is the transaction output that contains the opening_transaction
	// output script for the swap.
	ScriptOut uint32 `json:"script_out"`
	// BlindingKey:
	// Bitcoin: Blank.
	// Liquid BitcoinNetwork: Is the 32 byte blinding key to un-blind the outputs of
	//the opening_transaction.
	BlindingKey string `json:"blinding_key"`
}

func (s OpeningTxBroadcastedMessage) Validate(swap *SwapData) error {
	if swap.GetChain() == l_btc_chain {
		err := validateHexString("blinding_key", s.BlindingKey, 32)
		if err != nil {
			return err
		}
	}
	err := validateHexString("txId", s.TxId, 32)
	if err != nil {
		return err
	}
	return nil
}

func (t OpeningTxBroadcastedMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_OPENINGTXBROADCASTED
}

func (m OpeningTxBroadcastedMessage) ApplyToSwapData(swap *SwapData) error {
	if swap.OpeningTxBroadcasted != nil {
		return AlreadyExistsError
	}
	swap.OpeningTxBroadcasted = &m
	return nil
}

// CancelMessage is the message sent by a peer if he wants to / has to cancel
// the swap
type CancelMessage struct {
	// SwapId is the unique identifier of the swap.
	SwapId *SwapId `json:"swap_id"`
	// Message is a hint to why the swap was canceled.
	Message string `json:"message"`
}

func (e CancelMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_CANCELED
}

func (s CancelMessage) Validate(swap *SwapData) error {
	return nil
}

func (m CancelMessage) ApplyToSwapData(swap *SwapData) error {
	swap.Cancel = &m
	return nil
}

// CoopCloseMessage is the message sent by the transaction taker if he wants to
// cancel the swap, but allow the maker a quick close
type CoopCloseMessage struct {
	// SwapId is the unique identifier of the swap.
	SwapId *SwapId `json:"swap_id"`
	// Message is a hint to why the swap was canceled.
	Message string `json:"message"`
	// privkey is the private key to the pubkey that is used to build the opening_transaction.
	Privkey string `json:"privkey"`
}

func (c CoopCloseMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_COOPCLOSE
}

func (s CoopCloseMessage) Validate(swap *SwapData) error {
	err := validateHexString("privkey", s.Privkey, 32)
	if err != nil {
		return err
	}
	return nil
}

func (m CoopCloseMessage) ApplyToSwapData(swap *SwapData) error {
	if swap.CoopClose != nil {
		return AlreadyExistsError
	}
	swap.CoopClose = &m
	return nil
}

func MarshalPeerswapMessage(msg PeerMessage) ([]byte, int, error) {
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return nil, 0, err
	}
	return msgBytes, int(msg.MessageType()), nil
}
