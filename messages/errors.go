package messages

import "fmt"

var (
	ErrEvenMessageType   = fmt.Errorf("message type is even")
	ErrMessageNotInRange = fmt.Errorf("message type not in range")
)

type ErrAlreadyHasASender string

func (e ErrAlreadyHasASender) Error() string {
	return fmt.Sprintf("already has a sender with id %s", string(e))
}
