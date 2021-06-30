package fsm

import (
	"errors"
	"sync"
)

// ErrEventRejected is the error returned when the state machine cannot process
// an event in the state that it is in.
var ErrEventRejected = errors.New("event rejected")
var ErrDataNotAvailable = errors.New("data not in store")
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
	Execute(services *SwapServices, data Data, eventCtx EventContext) EventType
}

type Data interface {
	SetState(stateType StateType)
	GetCurrentState() StateType
	GetId() string
}

// Events represents a mapping of events and states.
type Events map[EventType]StateType

// State binds a state with an action and a set of events it can handle.
type State struct {
	Action Action
	Events Events
}

type Store interface {
	UpdateData(data Data) error
	GetData(id string) (Data, error)
}

// States represents a mapping of states and their implementations.
type States map[StateType]State

// SwapStateMachine represents the state machine.
type StateMachine struct {
	// Id holds the unique Id for the store
	Id string
	// Data holds the statemachine metadata
	Data Data

	// Type holds the SwapType
	Type SwapType

	// Role holds the local Role
	Role SwapRole

	// Previous represents the previous state.
	Previous StateType

	// Current represents the current state.
	Current StateType

	// States holds the configuration of states and events handled by the state machine.
	States States

	// mutex ensures that only 1 event is processed by the state machine at any given time.
	mutex sync.Mutex

	// Store saves the current state
	store Store

	// SwapServices stores services the statemachine may use
	swapServices *SwapServices
}

// getNextState returns the next state for the event given the machine's current
// state, or an error if the event can't be handled in the given state.
func (s *StateMachine) getNextState(event EventType) (StateType, error) {
	if state, ok := s.States[s.Current]; ok {
		if state.Events != nil {
			if next, ok := state.Events[event]; ok {
				return next, nil
			}
		}
	}
	return Default, ErrEventRejected
}

// SendEvent sends an event to the state machine.
func (s *StateMachine) SendEvent(event EventType, eventCtx EventContext) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.Id != "" {
		// todo recovery logic e.g. state is noop
		data, err := s.store.GetData(s.Id)
		if err == ErrDataNotAvailable {
			s.Data = &Swap{}
		} else if err != nil {
			return err
		} else {
			s.Data = data
			s.Current = data.GetCurrentState()
		}
	} else {
		return errors.New("id must be set")
	}

	for {
		// Determine the next state for the event given the machine's current state.
		nextState, err := s.getNextState(event)
		if err != nil {
			return ErrEventRejected
		}

		// Identify the state definition for the next state.
		state, ok := s.States[nextState]
		if !ok || state.Action == nil {
			// configuration error
			return ErrFsmConfig
		}

		// Transition over to the next state.
		s.Previous = s.Current
		s.Current = nextState
		if s.Data != nil {
			s.Data.SetState(s.Current)
		}

		// Execute the next state's action and loop over again if the event returned
		// is not a no-op.
		nextEvent := state.Action.Execute(s.swapServices, s.Data, eventCtx)
		s.Id = s.Data.GetId()
		err = s.store.UpdateData(s.Data)
		if err != nil {
			return err
		}
		if nextEvent == NoOp {
			return nil
		}
		event = nextEvent

	}
}
