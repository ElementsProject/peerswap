package messages

import "fmt"

// ErrNotPeerswapCustomMessage represents an error indicating
// that the message type is not a peerswap custom message.
type ErrNotPeerswapCustomMessage struct {
	MessageType string
}

// NewErrNotPeerswapCustomMessage creates a new ErrNotPeerswapCustomMessage with the given message type.
func NewErrNotPeerswapCustomMessage(messageType string) ErrNotPeerswapCustomMessage {
	return ErrNotPeerswapCustomMessage{MessageType: messageType}
}

// Error returns the error message for ErrNotPeerswapCustomMessage.
func (e ErrNotPeerswapCustomMessage) Error() string {
	return fmt.Sprintf("message type %s is not a peerswap custom message", e.MessageType)
}

// Is checks if the target error is of type ErrNotPeerswapCustomMessage.
func (e ErrNotPeerswapCustomMessage) Is(target error) bool {
	_, ok := target.(*ErrNotPeerswapCustomMessage)
	return ok
}

type ErrAlreadyHasASender string

func (e ErrAlreadyHasASender) Error() string {
	return fmt.Sprintf("already has a sender with id %s", string(e))
}
