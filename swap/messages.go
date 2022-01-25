package swap

import "github.com/sputn1ck/peerswap/messages"

// SwapInRequestMessage gets send when a peer wants to start a new swap.
type SwapInRequestMessage struct {
	SwapId          string
	Asset           string
	ChannelId       string
	Amount          uint64
	ProtocolVersion uint64
}

func (s SwapInRequestMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPINREQUEST
}

// SwapInAgreementMessage is the response by the swap-in peer if he accepts the swap.
type SwapInAgreementMessage struct {
	ProtocolVersion uint64
	SwapId          string
	TakerPubkeyHash string
}

func (s SwapInAgreementMessage) ApplyOnSwap(swap *SwapData) {
	swap.TakerPubkeyHash = s.TakerPubkeyHash
}

func (s SwapInAgreementMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPINAGREEMENT
}

// SwapOutRequestMessage gets send when a peer wants to start a new swap.
type SwapOutRequestMessage struct {
	SwapId          string
	Asset           string
	ChannelId       string
	Amount          uint64
	TakerPubkeyHash string
	ProtocolVersion uint64
}

func (s SwapOutRequestMessage) ApplyOnSwap(swap *SwapData) {
	swap.Id = s.SwapId
	swap.ChannelId = s.ChannelId
	swap.Asset = s.Asset
	swap.Amount = s.Amount
	swap.TakerPubkeyHash = s.TakerPubkeyHash
	swap.ProtocolVersion = s.ProtocolVersion
}

func (s SwapOutRequestMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPOUTREQUEST
}

// SwapOutAgreementMessage is the response by the swap-out peer if he accepts the swap
// it contains an Invoice that the swap-out initiator must pay.
type SwapOutAgreementMessage struct {
	ProtocolVersion uint64
	SwapId          string
	Invoice         string
}

func (s SwapOutAgreementMessage) ApplyOnSwap(swap *SwapData) {
	swap.FeeInvoice = s.Invoice
}

func (s SwapOutAgreementMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPOUTAGREEMENT
}

// OpeningTxBroadcastedMessage is the message sent by the creator of the opening tx
type OpeningTxBroadcastedMessage struct {
	SwapId          string
	MakerPubkeyHash string
	Invoice         string
	TxHex           string

	// BlindingStuff
	BlindingKeyHex string
}

func (t OpeningTxBroadcastedMessage) ApplyOnSwap(swap *SwapData) {
	swap.MakerPubkeyHash = t.MakerPubkeyHash
	swap.ClaimInvoice = t.Invoice
	swap.OpeningTxHex = t.TxHex

	swap.BlindingKeyHex = t.BlindingKeyHex
}

func (t OpeningTxBroadcastedMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_OPENINGTXBROADCASTED
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
	SwapId string

	TakerPrivKey string
}

func (c CoopCloseMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_COOPCLOSE
}

func (c CoopCloseMessage) ApplyOnSwap(swap *SwapData) {
	swap.TakerPrivkey = c.TakerPrivKey
}
