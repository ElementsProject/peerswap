package messages

import "fmt"

var (
	ErrEvenMessageType   = fmt.Errorf("message type is even")
	ErrMessageNotInRange = fmt.Errorf("message type not in range")
)
