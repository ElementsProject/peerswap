package messages

import (
	"fmt"
	"strconv"
)

type MessageType int

const (
	// BASE_MESSAGE_TYPE is the first message type of the
	// peerswap messages in the range of custom message
	// types according to BOLT #1. All other peerswap
	// messages add even numbers to the base type to keep
	// the message type odd.
	BASE_MESSAGE_TYPE = 42069
	// These are the peeerswap swap related messages as
	// specified by the peerswap protocol. They are
	// evenly separated to keep the resulting message type
	// odd.
	MESSAGETYPE_SWAPINREQUEST MessageType = BASE_MESSAGE_TYPE - 1 + iota
	_
	MESSAGETYPE_SWAPOUTREQUEST
	_
	MESSAGETYPE_SWAPINAGREEMENT
	_
	MESSAGETYPE_SWAPOUTAGREEMENT
	_
	MESSAGETYPE_OPENINGTXBROADCASTED
	_
	MESSAGETYPE_CANCELED
	_
	MESSAGETYPE_COOPCLOSE
	_
	MESSAGETYPE_POLL
	_
	MESSAGETYPE_REQUEST_POLL
	UPPER_MESSAGE_BOUND
)

// InRange checks if the message type lays in the
// peerswap message range.
func InRange(msgType MessageType) (bool, error) {
	// MessageType we do not accept even message types
	if msgType%2 == 0 {
		return false, ErrEvenMessageType
	}
	return BASE_MESSAGE_TYPE <= msgType && msgType < UPPER_MESSAGE_BOUND, nil
}

// MessageTypeToHexStr returns the hex encoded string
// of the messagetype.
func MessageTypeToHexString(messageIndex MessageType) string {
	return strconv.FormatInt(int64(messageIndex), 16)
}

// HexStrToMsgType returns the message type from a
// hex encoded string.
func HexStringToMessageType(msgTypeStr string) (MessageType, error) {
	msgTypeInt, err := strconv.ParseInt(msgTypeStr, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("could not parse hex string to message type: %w", err)
	}

	msgType := MessageType(msgTypeInt)

	inRange, err := InRange(msgType)
	if err != nil {
		return 0, err
	}
	if !inRange {
		return 0, ErrMessageNotInRange
	}
	return msgType, nil
}
