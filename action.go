package atoha

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ActionID identifies one immutable intent. Reusing an ActionID does not make
// executing the intent idempotent or safe to replay.
type ActionID string

// AttemptID identifies one attempt to execute an Action.
type AttemptID string

var (
	// ErrInvalidAction indicates that an ActionSpec cannot define an Action.
	ErrInvalidAction = errors.New("atoha: invalid action")
	// ErrInvalidExecutionAttempt indicates that an ExecutionAttemptSpec cannot
	// define an ExecutionAttempt.
	ErrInvalidExecutionAttempt = errors.New("atoha: invalid execution attempt")
	// ErrActionIdentityConflict means an existing ActionID was presented with
	// different immutable intent.
	ErrActionIdentityConflict = errors.New("atoha: action identity conflict")
)

// ActionSpec contains the caller-supplied data needed to create an Action.
// Input is opaque to the core and its encoding is agreed by the caller and the
// selected provider.
type ActionSpec struct {
	ID          ActionID
	Target      string
	Name        string
	Input       []byte
	RequestedAt time.Time
}

// Action is an immutable expression of intent.
//
// The zero value is invalid. Construct an Action with NewAction. Accessors that
// expose mutable data return copies.
type Action struct {
	id          ActionID
	target      string
	name        string
	input       []byte
	requestedAt time.Time
}

// NewAction validates spec and returns an immutable Action.
func NewAction(spec ActionSpec) (Action, error) {
	if err := validateIdentity("action ID", string(spec.ID)); err != nil {
		return Action{}, fmt.Errorf("%w: %v", ErrInvalidAction, err)
	}
	if err := validateText("target", spec.Target); err != nil {
		return Action{}, fmt.Errorf("%w: %v", ErrInvalidAction, err)
	}
	if err := validateText("name", spec.Name); err != nil {
		return Action{}, fmt.Errorf("%w: %v", ErrInvalidAction, err)
	}
	if spec.RequestedAt.IsZero() {
		return Action{}, fmt.Errorf("%w: requested time is required", ErrInvalidAction)
	}

	return Action{
		id:          spec.ID,
		target:      spec.Target,
		name:        spec.Name,
		input:       cloneBytes(spec.Input),
		requestedAt: spec.RequestedAt,
	}, nil
}

// ID returns the identity of the immutable intent.
func (a Action) ID() ActionID { return a.id }

// Target returns the logical target supplied by the caller.
func (a Action) Target() string { return a.target }

// Name returns the provider-independent operation name supplied by the caller.
func (a Action) Name() string { return a.name }

// Input returns a copy of the opaque action input.
func (a Action) Input() []byte { return cloneBytes(a.input) }

// RequestedAt returns when the caller created the intent.
func (a Action) RequestedAt() time.Time { return a.requestedAt }

// ExecutionAttemptSpec contains the data needed to record one attempt to
// execute an Action. RecoveryOf is optional and links an explicit recovery to a
// prior attempt; it does not assert that replay is safe.
type ExecutionAttemptSpec struct {
	ID         AttemptID
	ActionID   ActionID
	StartedAt  time.Time
	RecoveryOf AttemptID
}

// ExecutionAttempt identifies one attempt to execute an Action.
//
// The zero value is invalid. Construct an ExecutionAttempt with
// NewExecutionAttempt.
type ExecutionAttempt struct {
	id         AttemptID
	actionID   ActionID
	startedAt  time.Time
	recoveryOf AttemptID
}

// NewExecutionAttempt validates spec and returns an ExecutionAttempt.
func NewExecutionAttempt(spec ExecutionAttemptSpec) (ExecutionAttempt, error) {
	if err := validateIdentity("attempt ID", string(spec.ID)); err != nil {
		return ExecutionAttempt{}, fmt.Errorf("%w: %v", ErrInvalidExecutionAttempt, err)
	}
	if err := validateIdentity("action ID", string(spec.ActionID)); err != nil {
		return ExecutionAttempt{}, fmt.Errorf("%w: %v", ErrInvalidExecutionAttempt, err)
	}
	if spec.StartedAt.IsZero() {
		return ExecutionAttempt{}, fmt.Errorf("%w: start time is required", ErrInvalidExecutionAttempt)
	}
	if spec.RecoveryOf != "" {
		if err := validateIdentity("recovery attempt ID", string(spec.RecoveryOf)); err != nil {
			return ExecutionAttempt{}, fmt.Errorf("%w: %v", ErrInvalidExecutionAttempt, err)
		}
		if spec.RecoveryOf == spec.ID {
			return ExecutionAttempt{}, fmt.Errorf("%w: an attempt cannot recover itself", ErrInvalidExecutionAttempt)
		}
	}

	return ExecutionAttempt{
		id:         spec.ID,
		actionID:   spec.ActionID,
		startedAt:  spec.StartedAt,
		recoveryOf: spec.RecoveryOf,
	}, nil
}

// ID returns the identity of this individual execution attempt.
func (a ExecutionAttempt) ID() AttemptID { return a.id }

// ActionID returns the identity of the immutable intent being attempted.
func (a ExecutionAttempt) ActionID() ActionID { return a.actionID }

// StartedAt returns when this attempt began.
func (a ExecutionAttempt) StartedAt() time.Time { return a.startedAt }

// RecoveryOf returns the prior attempt this attempt explicitly recovers. The
// boolean is false for an initial or otherwise unrelated attempt.
func (a ExecutionAttempt) RecoveryOf() (AttemptID, bool) {
	return a.recoveryOf, a.recoveryOf != ""
}

func validateIdentity(name, value string) error {
	return validateText(name, value)
}

func validateText(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not have leading or trailing whitespace", name)
	}
	return nil
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}
