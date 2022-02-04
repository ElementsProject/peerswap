package swap

import (
	"errors"
	"log"
	"sync"
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
type EventContext interface{}

// Action represents the action to be executed in a given state.
type Action interface {
	Execute(services *SwapServices, swap *SwapData) EventType
}

// Events represents a mapping of events and states.
type Events map[EventType]StateType

// State binds a state with an action and a set of events it can handle.
type State struct {
	Action Action
	Events Events
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
	Id     string  `json:"id"`
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

	err := s.swapServices.swapStore.UpdateData(s)
	if err != nil {
		return false, err
	}

	for {
		// Determine the next state for the event given the machine's current state.
		log.Printf("[FSM] event:id: %s, %s on %s", s.Id, event, s.Current)
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
		s.Current = nextState

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
				log.Printf("[FSM] Action failure %v", s.Data.LastErr)
			}
		}

		event = nextEvent
	}
}

// Recover tries to continue from the current state, by doing the associated Action
func (s *SwapStateMachine) Recover() (bool, error) {
	state, ok := s.States[s.Current]

	if !ok || state.Action == nil {
		// configuration error
		return false, ErrFsmConfig
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
	case State_ClaimedPreimage:
		return true
	}
	return false
}
