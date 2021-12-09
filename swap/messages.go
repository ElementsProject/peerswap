package swap

import "github.com/sputn1ck/peerswap/messages"

// SwapInRequest gets send when a peer wants to start a new swap.
type SwapInRequest struct {
	SwapId          string
	Asset           string
	ChannelId       string
	Amount          uint64
	ProtocolVersion uint64
}

func (s SwapInRequest) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPINREQUEST
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

func (s SwapOutRequest) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPOUTREQUEST
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

func (s FeeMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_FEERESPONSE
}

// SwapInAgreementMessage is the response by the swap-in peer if he accepts the swap
type SwapInAgreementMessage struct {
	SwapId          string
	TakerPubkeyHash string
}

func (s SwapInAgreementMessage) ApplyOnSwap(swap *SwapData) {
	swap.TakerPubkeyHash = s.TakerPubkeyHash
}

func (s SwapInAgreementMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPINAGREEMENT
}

// TxOpenedMessage is the message sent by the creator of the opening tx
type TxOpenedMessage struct {
	SwapId          string
	MakerPubkeyHash string
	RefundAddr      string
	RefundFee       uint64
	Invoice         string
	TxHex           string
}

func (t TxOpenedMessage) ApplyOnSwap(swap *SwapData) {
	swap.MakerPubkeyHash = t.MakerPubkeyHash
	swap.ClaimInvoice = t.Invoice
	swap.OpeningTxHex = t.TxHex
	swap.MakerRefundAddr = t.RefundAddr
	swap.RefundFee = t.RefundFee
}

func (t TxOpenedMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_TXOPENEDRESPONSE
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

func (c ClaimedMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_CLAIMED
}

// CancelMessage is the message sent by a peer if he wants to / has to cancel the swap
type CancelMessage struct {
	SwapId string
	Error  string
}

func (e CancelMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_CANCELED
}

func (c CancelMessage) ApplyOnSwap(swap *SwapData) {
	swap.CancelMessage = c.Error
}

// CoopCloseMessage is the message sent by the transaction taker if he wants to cancel the swap, but allow the maker a quick close
type CoopCloseMessage struct {
	SwapId             string
	TakerRefundSigHash string
}

func (c CoopCloseMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_COOPCLOSE
}

func (c CoopCloseMessage) ApplyOnSwap(swap *SwapData) {
	swap.TakerRefundSigHash = c.TakerRefundSigHash
}
