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
)

// PeerswapCustomMessageType converts a hexadecimal string representation of a message type
// to its corresponding MessageType. If the message type is not recognized, it returns an error.
func PeerswapCustomMessageType(msgType string) (MessageType, error) {
	// Parse the hexadecimal string to an integer.
	msgTypeInt, err := strconv.ParseInt(msgType, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("could not parse hex string to message type: %w", err)
	}

	// Match the parsed integer to the corresponding MessageType.
	switch MessageType(msgTypeInt) {
	case MESSAGETYPE_SWAPINREQUEST:
		return MESSAGETYPE_SWAPINREQUEST, nil
	case MESSAGETYPE_SWAPOUTREQUEST:
		return MESSAGETYPE_SWAPOUTREQUEST, nil
	case MESSAGETYPE_SWAPINAGREEMENT:
		return MESSAGETYPE_SWAPINAGREEMENT, nil
	case MESSAGETYPE_SWAPOUTAGREEMENT:
		return MESSAGETYPE_SWAPOUTAGREEMENT, nil
	case MESSAGETYPE_OPENINGTXBROADCASTED:
		return MESSAGETYPE_OPENINGTXBROADCASTED, nil
	case MESSAGETYPE_CANCELED:
		return MESSAGETYPE_CANCELED, nil
	case MESSAGETYPE_COOPCLOSE:
		return MESSAGETYPE_COOPCLOSE, nil
	case MESSAGETYPE_POLL:
		return MESSAGETYPE_POLL, nil
	case MESSAGETYPE_REQUEST_POLL:
		return MESSAGETYPE_REQUEST_POLL, nil
	default:
		// Return an error if the message type is not recognized.
		return 0, NewErrNotPeerswapCustomMessage(msgType)
	}
}

// MessageTypeToHexString returns the hex encoded string
// of the messagetype.
func MessageTypeToHexString(messageIndex MessageType) string {
	return strconv.FormatInt(int64(messageIndex), 16)
}
