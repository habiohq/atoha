package execution

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidInput          = errors.New("atoha execution: invalid input")
	ErrAttemptAlreadyStarted = errors.New("atoha execution: attempt already started")
	ErrAttemptNotFound       = errors.New("atoha execution: attempt not found")
	ErrActionMismatch        = errors.New("atoha execution: action does not match attempt")
	ErrAdmissionUnknown      = errors.New("atoha execution: admission is unknown")
	ErrVerificationUnknown   = errors.New("atoha execution: verification is unknown")
	ErrRecoveryNotAuthorized = errors.New("atoha execution: recovery is not authorized")
	ErrInvalidProviderResult = errors.New("atoha execution: invalid provider result")
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
	return fmt.Sprintf("atoha execution %s: %v", e.Stage, e.Err)
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
