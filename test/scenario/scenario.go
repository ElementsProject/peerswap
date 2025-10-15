package scenario

import (
	"fmt"
	"time"
)

// LogWaiter captures the subset of daemon wait functionality required by scenarios.
type LogWaiter interface {
	WaitForLog(message string, timeout time.Duration) error
}

// LogExpectation ties an action to the log message we expect to observe.
type LogExpectation struct {
	Action  string
	Message string
	Waiter  LogWaiter
	Timeout time.Duration
}

// LogBook stores the expectations that can be awaited in tests.
type LogBook struct {
	expectations map[string]LogExpectation
}

// NewLogBook returns a LogBook ready for registrations.
func NewLogBook() *LogBook {
	return &LogBook{
		expectations: make(map[string]LogExpectation),
	}
}

// Register records the expectation used later by Await.
func (lb *LogBook) Register(exp LogExpectation) {
	if exp.Action == "" {
		return
	}
	lb.expectations[exp.Action] = exp
}

// Await waits for the registered log entry and annotates failures with the action name.
func (lb *LogBook) Await(action string) error {
	exp, ok := lb.expectations[action]
	if !ok {
		return fmt.Errorf("log expectation for action %q not registered", action)
	}
	if exp.Waiter == nil {
		return fmt.Errorf("log expectation for action %q has no waiter", action)
	}
	if err := exp.Waiter.WaitForLog(exp.Message, exp.Timeout); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}
