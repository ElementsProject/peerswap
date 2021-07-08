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
	MESSAGE_END int64 = iota

	MESSAGE_BASE = 42069
)

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

type MessageBase struct {
	SwapId string
}

// SwapInRequest gets send when a peer wants to start a new swap.
type SwapInRequest struct {
	SwapId    string
	ChannelId string
	Amount    uint64
}

func (s *SwapInRequest) MessageType() MessageType {
	return MESSAGETYPE_SWAPINREQUEST
}

// SwapOutRequest gets send when a peer wants to start a new swap.
type SwapOutRequest struct {
	SwapId          string
	ChannelId       string
	Amount          uint64
	TakerPubkeyHash string
}

func (s *SwapOutRequest) ApplyOnSwap(swap *Swap) {
	swap.Id = s.SwapId
	swap.ChannelId = s.ChannelId
	swap.Amount = s.Amount
	swap.TakerPubkeyHash = s.TakerPubkeyHash
}

func (s *SwapOutRequest) MessageType() MessageType {
	return MESSAGETYPE_SWAPOUTREQUEST
}

type FeeResponse struct {
	SwapId  string
	Invoice string
}

func (s *FeeResponse) ApplyOnSwap(swap *Swap) {
	swap.FeeInvoice = s.Invoice
}

func (s *FeeResponse) MessageType() MessageType {
	return MESSAGETYPE_FEERESPONSE
}

type SwapInAgreementResponse struct {
	SwapId          string
	TakerPubkeyHash string
}

func (s *SwapInAgreementResponse) ApplyOnSwap(swap *Swap) {
	swap.TakerPubkeyHash = s.TakerPubkeyHash
}

func (s *SwapInAgreementResponse) MessageType() MessageType {
	return MESSAGETYPE_SWAPINAGREEMENT
}

type TxOpenedResponse struct {
	SwapId          string
	MakerPubkeyHash string
	Invoice         string
	TxId            string
	TxHex           string
	Cltv            int64
}

func (t *TxOpenedResponse) ApplyOnSwap(swap *Swap) {
	swap.MakerPubkeyHash = t.MakerPubkeyHash
	swap.ClaimPayreq = t.Invoice
	swap.OpeningTxId = t.TxId
	swap.OpeningTxHex = t.TxHex
	swap.Cltv = t.Cltv
}

func (t *TxOpenedResponse) MessageType() MessageType {
	return MESSAGETYPE_TXOPENEDRESPONSE
}

type ClaimedMessage struct {
	SwapId    string
	ClaimType ClaimType
	ClaimTxId string
}

func (c *ClaimedMessage) ApplyOnSwap(swap *Swap) {
	swap.ClaimTxId = c.ClaimTxId
}

func (c *ClaimedMessage) MessageType() MessageType {
	return MESSAGETYPE_CLAIMED
}

type CancelResponse struct {
	SwapId string
	Error  string
}

func (e *CancelResponse) MessageType() MessageType {
	return MESSAGETYPE_CANCELED
}

func MessageTypeToHexString(messageIndex MessageType) string {
	return strconv.FormatInt(MESSAGE_BASE+int64(messageIndex), 16)
}

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
