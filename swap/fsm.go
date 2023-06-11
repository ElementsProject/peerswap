package swap

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/log"
)

// ErrEventRejected is the error returned when the state machine cannot process
// an event in the state that it is in.
var ErrEventRejected = errors.New("event rejected")

// ErrDataNotAvailable is the error returned when the store does not have the data stored yet
var ErrDataNotAvailable = errors.New("data not in store")

// ErrFsmConfig is the error returned when the fsm configuartion is invalid
// i.e. the fsm does not contain the next state
var ErrFsmConfig = errors.New("fsm config invalid")

const (
	// Default represents the default state of the system.
	Default StateType = ""

	// NoOp represents a no-op event.
	NoOp EventType = "NoOp"
)

// StateType represents an extensible state type in the state machine.
type StateType string

// EventType represents an extensible event type in the state machine.
type EventType string

// EventContext represents the context to be passed to the action implementation.
type EventContext interface {
	ApplyToSwapData(data *SwapData) error
	Validate(data *SwapData) error
}

// Action represents the action to be executed in a given state.
type Action interface {
	Execute(services *SwapServices, swap *SwapData) EventType
}

// Events represents a mapping of events and states.
type Events map[EventType]StateType

// State binds a state with an action and a set of events it can handle.
type State struct {
	Action        Action
	Events        Events
	FailOnrecover bool
}

type Store interface {
	UpdateData(data *SwapStateMachine) error
	GetData(id string) (*SwapStateMachine, error)
	ListAll() ([]*SwapStateMachine, error)
	ListAllByPeer(peer string) ([]*SwapStateMachine, error)
}

// States represents a mapping of states and their implementations.
type States map[StateType]State

// SwapStateMachine represents the state machine.
type SwapStateMachine struct {
	// Id holds the unique Id for the store
	SwapId *SwapId `json:"swap_id"`

	// Data holds the statemachine metadata
	Data *SwapData `json:"data"`

	// Type holds the SwapType
	Type SwapType `json:"type"`

	// Role holds the local Role
	Role SwapRole `json:"role"`

	// Previous represents the previous state.
	Previous StateType `json:"previous"`

	// Current represents the current state.
	Current StateType `json:"current"`

	// States holds the configuration of states and events handled by the state machine.
	States States `json:"-"`

	// mutex ensures that only 1 event is processed by the state machine at any given time.
	mutex sync.Mutex

	// SwapServices stores services the statemachine may use
	swapServices *SwapServices
	// retries counts how many retries a event has already done
	retries int

	failures int

	stateMutex  sync.Mutex
	stateChange *sync.Cond
}

func (s *SwapStateMachine) setState(newState StateType) {
	s.stateMutex.Lock()
	s.Current = newState
	s.stateMutex.Unlock()
	s.stateChange.Broadcast()
}

// WaitForStateChange calls the isDesiredState callback on every state change.
// It returns true if the callback returned true and false if the timeout is
// reached.
func (s *SwapStateMachine) WaitForStateChange(isDesiredState func(StateType) bool, timeout time.Duration) bool {
	timeoutCh := time.After(timeout)
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()

	var timedOut bool
	for !isDesiredState(s.Current) {
		unlockCh := make(chan struct{})

		go func() {
			select {
			case <-unlockCh:
			case <-timeoutCh:
				timedOut = true
				s.stateChange.Broadcast()
			}
		}()

		s.stateChange.Wait()
		close(unlockCh)

		if timedOut {
			return isDesiredState(s.Current)
		}
	}

	return true
}

// getNextState returns the next state for the event given the machine's current
// state, or an error if the event can't be handled in the given state.
func (s *SwapStateMachine) getNextState(event EventType) (StateType, error) {
	if state, ok := s.States[s.Current]; ok {
		if state.Events != nil {
			if next, ok := state.Events[event]; ok {
				return next, nil
			}
		}
	}
	return Default, ErrEventRejected
}

// EventIsValid returns true if the event is valid for the current statemachine transition
func (s *SwapStateMachine) EventIsValid(event EventType) bool {
	nextState, err := s.getNextState(event)
	if err != nil {
		return false
	}

	// Identify the state definition for the next state.
	state, ok := s.States[nextState]
	if !ok || state.Action == nil {
		// configuration error
		return false
	}

	return true
}

// SendEvent sends an event to the state machine.
func (s *SwapStateMachine) SendEvent(event EventType, eventCtx EventContext) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if event == Event_Done {
		return true, nil
	}
	var err error

	// validate and apply event context
	if eventCtx != nil {
		err = eventCtx.Validate(s.Data)
		if err != nil {
			s.mutex.Unlock()
			log.Infof("Message validation error: %v on msg %v", err, eventCtx)
			res, err := s.SendEvent(Event_OnInvalid_Message, nil)
			s.mutex.Lock()
			return res, err
		}
		err = eventCtx.ApplyToSwapData(s.Data)
		if err != nil {
			if event == Event_OnSwapOutStarted || event == Event_SwapInSender_OnSwapInRequested {
				return true, err
			}
			return false, err
		}
	}

	err = s.swapServices.swapStore.UpdateData(s)
	if err != nil {
		return false, err
	}

	for {
		// Determine the next state for the event given the machine's current state.
		log.Debugf("[FSM] event:id: %s, %s on %s", s.SwapId.String(), event, s.Current)
		nextState, err := s.getNextState(event)
		if err != nil {
			return false, ErrEventRejected
		}

		// Identify the state definition for the next state.
		state, ok := s.States[nextState]
		if !ok || state.Action == nil {
			// configuration error
			return false, ErrFsmConfig
		}

		// Transition over to the next state.
		s.Previous = s.Current
		s.setState(nextState)
		s.Data.SetState(nextState)

		// Print Swap information
		s.logSwapInfo()

		// Execute the next state's action and loop over again if the event returned
		// is not a no-op.
		nextEvent := state.Action.Execute(s.swapServices, s.Data)
		err = s.swapServices.swapStore.UpdateData(s)
		if err != nil {
			return false, err
		}

		switch nextEvent {
		case Event_Done:
			return true, nil
		case NoOp:
			return false, nil
		case Event_OnRetry:
			s.retries++
			if s.retries > 20 {
				s.retries = 0
				return false, nil
			}
		case Event_ActionFailed:
			if s.Data.LastErr != nil {
				log.Infof("[FSM] Action failure %v", s.Data.LastErr)
			}
		}

		event = nextEvent
	}
}

// Recover tries to continue from the current state, by doing the associated Action
func (s *SwapStateMachine) Recover() (bool, error) {
	log.Infof("[Swap:%s]: Recovering from state %s", s.SwapId.String(), s.Current)
	state, ok := s.States[s.Current]
	if !ok {
		return false, fmt.Errorf("unknown state: %s for swap %s", s.Current, s.SwapId.String())
	}

	if !ok || state.Action == nil {
		// configuration error
		return false, ErrFsmConfig
	}
	if state.FailOnrecover {
		return s.SendEvent(Event_ActionFailed, nil)
	}

	nextEvent := state.Action.Execute(s.swapServices, s.Data)
	err := s.swapServices.swapStore.UpdateData(s)
	if err != nil {
		return false, err
	}
	if nextEvent == NoOp {
		return false, nil
	}
	return s.SendEvent(nextEvent, nil)
}

// IsFinished returns true if the swap is already finished
func (s *SwapStateMachine) IsFinished() bool {
	switch s.Current {
	case State_ClaimedCsv:
		return true
	case State_SwapCanceled:
		return true
	case State_ClaimedPreimage:
		return true
	case State_ClaimedCoop:
		return true
	case State_ClaimedUndefined:
	}
	return false
}

// logSwapInfo give the user useful information depending on the swap statemachine
func (s *SwapStateMachine) logSwapInfo() {

	// Start Swap as Sender
	if s.Current == State_SwapInSender_CreateSwap ||
		s.Current == State_SwapOutSender_CreateSwap {
		s.Infof("Start new %s: peer: %s chanId: %s initiator: %s amount %v",
			s.Data.GetType(), s.Data.PeerNodeId, s.Data.GetScid(), s.Data.InitiatorNodeId, s.Data.GetAmount())
	}

	// Swap Request accepted
	if s.Current == State_SwapInReceiver_SendAgreement ||
		s.Current == State_SwapOutReceiver_SendFeeInvoice {
		s.Infof("%s request received: peer: %s chanId: %s initiator: %s amount %v",
			s.Data.GetType(), s.Data.PeerNodeId, s.Data.GetScid(), s.Data.InitiatorNodeId, s.Data.GetAmount())
	}

	// Swap Claimed with preimage
	if s.Current == State_ClaimedPreimage {
		s.Infof("Swap claimed with preimage %s", s.Data.GetPreimage())
	}

	// Swap Claimed by coop case
	if s.Current == State_ClaimedCoop {
		s.Infof("Swap claimed cooperatively")
	}

	// Swap Claimed by csv case
	if s.Current == State_ClaimedCsv {
		s.Infof("Swap claimed by csv")
	}

	// Swap Canceled
	if s.Current == State_SwapCanceled {
		s.Infof("Swap canceled. Reason: %s", s.Data.GetCancelMessage())
	}

	// Special cases

	// The swap was canceled after paying the fee invoice
	if s.Current == State_SwapCanceled &&
		s.Previous == State_SwapOutSender_AwaitTxBroadcastedMessage {
		s.Infof("Warning: Paid swap-out prepayment, but swap canceled before receiving opening transaction")
	}

	// The fee invoice was paid
	if s.Current == State_SwapOutSender_AwaitTxBroadcastedMessage &&
		s.Previous == State_SwapOutSender_PayFeeInvoice {
		s.printFeeInvoiceInfo()
	}

}

func (s *SwapStateMachine) printFeeInvoiceInfo() {
	if s.Data.SwapOutAgreement == nil {
		return
	}

	ll := s.swapServices.lightning
	_, msatAmt, _, err := ll.DecodePayreq(s.Data.SwapOutAgreement.Payreq)
	if err != nil {
		return
	}
	paidAmt := msatAmt / 1000

	s.Infof("Paid Feeinvoice of %v sats", paidAmt)
}

func (s *SwapStateMachine) Infof(format string, v ...interface{}) {
	idString := fmt.Sprintf("%s", s.SwapId.String())
	log.Infof("[Swap:"+idString+"] "+format, v...)
}
