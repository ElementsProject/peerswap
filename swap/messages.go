package swap

import (
	"errors"
	"strconv"
)

type MessageType int

const (
	MESSAGETYPE_SWAPINREQUEST MessageType = iota
	_
	MESSAGETYPE_SWAPOUTREQUEST
	_
	MESSAGETYPE_SWAPINAGREEMENT
	_
	MESSAGETYPE_FEERESPONSE
	_
	MESSAGETYPE_TXOPENEDRESPONSE
	_
	MESSAGETYPE_CANCELED
	_
	MESSAGETYPE_CLAIMED
	_
	MESSAGETYPE_COOPCLOSE
	MESSAGE_END int64 = iota

	MESSAGE_BASE = 42069
)

// InRange returns true if the message is in the range of our defined messages
func InRange(msg string) (bool, error) {
	msgInt, err := strconv.ParseInt(msg, 16, 64)
	if err != nil {
		return false, err
	}
	if msgInt%2 == 0 {
		return false, err
	}
	return msgInt >= MESSAGE_BASE && msgInt < MESSAGE_BASE+MESSAGE_END, nil
}

// SwapInRequest gets send when a peer wants to start a new swap.
type SwapInRequest struct {
	SwapId          string
	Asset           string
	ChannelId       string
	Amount          uint64
	ProtocolVersion uint64
}

func (s SwapInRequest) MessageType() MessageType {
	return MESSAGETYPE_SWAPINREQUEST
}

// SwapOutRequest gets send when a peer wants to start a new swap.
type SwapOutRequest struct {
	SwapId          string
	Asset           string
	ChannelId       string
	Amount          uint64
	TakerPubkeyHash string
	ProtocolVersion uint64
}

func (s SwapOutRequest) ApplyOnSwap(swap *SwapData) {
	swap.Id = s.SwapId
	swap.ChannelId = s.ChannelId
	swap.Asset = s.Asset
	swap.Amount = s.Amount
	swap.TakerPubkeyHash = s.TakerPubkeyHash
	swap.ProtocolVersion = s.ProtocolVersion
}

func (s SwapOutRequest) MessageType() MessageType {
	return MESSAGETYPE_SWAPOUTREQUEST
}

// FeeMessage is the response by the swap-out peer if he accepts the swap
// it contains an Invoice that the swap-out initiator must pay
type FeeMessage struct {
	SwapId  string
	Invoice string
}

func (s FeeMessage) ApplyOnSwap(swap *SwapData) {
	swap.FeeInvoice = s.Invoice
}

func (s FeeMessage) MessageType() MessageType {
	return MESSAGETYPE_FEERESPONSE
}

// SwapInAgreementMessage is the response by the swap-in peer if he accepts the swap
type SwapInAgreementMessage struct {
	SwapId          string
	TakerPubkeyHash string
}

func (s SwapInAgreementMessage) ApplyOnSwap(swap *SwapData) {
	swap.TakerPubkeyHash = s.TakerPubkeyHash
}

func (s SwapInAgreementMessage) MessageType() MessageType {
	return MESSAGETYPE_SWAPINAGREEMENT
}

// TxOpenedMessage is the message sent by the creator of the opening tx
type TxOpenedMessage struct {
	SwapId          string
	MakerPubkeyHash string
	RefundAddr      string
	Invoice         string
	TxId string
}

func (t TxOpenedMessage) ApplyOnSwap(swap *SwapData) {
	swap.MakerPubkeyHash = t.MakerPubkeyHash
	swap.ClaimInvoice = t.Invoice
	swap.OpeningTxId = t.TxId
	swap.MakerRefundAddr = t.RefundAddr
}

func (t TxOpenedMessage) MessageType() MessageType {
	return MESSAGETYPE_TXOPENEDRESPONSE
}

// ClaimedMessage is the message sent by the peer who claims the opening tx
type ClaimedMessage struct {
	SwapId    string
	ClaimType ClaimType
	ClaimTxId string
}

func (c ClaimedMessage) ApplyOnSwap(swap *SwapData) {
	swap.ClaimTxId = c.ClaimTxId
}

func (c ClaimedMessage) MessageType() MessageType {
	return MESSAGETYPE_CLAIMED
}

// CancelMessage is the message sent by a peer if he wants to / has to cancel the swap
type CancelMessage struct {
	SwapId string
	Error  string
}

func (e CancelMessage) MessageType() MessageType {
	return MESSAGETYPE_CANCELED
}

func (c CancelMessage) ApplyOnSwap(swap *SwapData) {
	swap.CancelMessage = c.Error
}

// CoopCloseMessage is the message sent by the transaction taker if he wants to cancel the swap, but allow the maker a quick close
type CoopCloseMessage struct {
	SwapId             string
	TakerRefundSigHash string
}

func (c CoopCloseMessage) MessageType() MessageType {
	return MESSAGETYPE_COOPCLOSE
}

func (c CoopCloseMessage) ApplyOnSwap(swap *SwapData) {
	swap.TakerRefundSigHash = c.TakerRefundSigHash
}

// MessageTypeToHexString returns the hex encoded string of the messagetype
func MessageTypeToHexString(messageIndex MessageType) string {
	return strconv.FormatInt(MESSAGE_BASE+int64(messageIndex), 16)
}

// HexStrToMsgType returns the message type from a hex encoded string
func HexStrToMsgType(msgType string) (MessageType, error) {
	inRange, err := InRange(msgType)
	if err != nil {
		return 0, err
	}
	if !inRange {
		return 0, errors.New("message not in range")
	}
	msgInt, err := strconv.ParseInt(msgType, 16, 64)
	if err != nil {
		return 0, err
	}
	return MessageType(msgInt - MESSAGE_BASE), nil
}
