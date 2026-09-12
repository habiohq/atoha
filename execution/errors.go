package execution

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidInput          = errors.New("habio execution: invalid input")
	ErrAttemptAlreadyStarted = errors.New("habio execution: attempt already started")
	ErrAttemptNotFound       = errors.New("habio execution: attempt not found")
	ErrActionMismatch        = errors.New("habio execution: action does not match attempt")
	ErrAdmissionUnknown      = errors.New("habio execution: admission is unknown")
	ErrVerificationUnknown   = errors.New("habio execution: verification is unknown")
	ErrRecoveryNotAuthorized = errors.New("habio execution: recovery is not authorized")
	ErrInvalidProviderResult = errors.New("habio execution: invalid provider result")
)

// Stage identifies the application step at which a software-path error arose.
// It does not describe the physical outcome.
type Stage string

const (
	StageValidate Stage = "validate"
	StageRecord   Stage = "record"
	StageAdmit    Stage = "admit"
	StageResolve  Stage = "resolve"
	StageDispatch Stage = "dispatch"
	StageObserve  Stage = "observe"
	StageVerify   Stage = "verify"
	StageRecover  Stage = "recover"
)

// Error adds application-stage context while preserving errors.Is/errors.As.
// Callers must inspect semantic result values separately from Error.
type Error struct {
	Stage Stage
	Err   error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("habio execution %s: %v", e.Stage, e.Err)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func stageError(stage Stage, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Stage: stage, Err: err}
}
