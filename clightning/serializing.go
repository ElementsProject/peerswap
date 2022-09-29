package clightning

import "github.com/elementsproject/peerswap/swap"

// SerializedSwapStateMachine is the serialized representation of the internal
// state machine with all these massive (and unnecessary) amounts of data.
type SerializedSwapStateMachine struct {
	*swap.SwapStateMachine
	Type string `json:"type"`
	Role string `json:"role"`
}

func MSerializedSwapStateMachine(swapStateMachine *swap.SwapStateMachine) *SerializedSwapStateMachine {
	return &SerializedSwapStateMachine{
		SwapStateMachine: swapStateMachine,
		Type:             swapStateMachine.Type.String(),
		Role:             swapStateMachine.Role.String(),
	}
}
