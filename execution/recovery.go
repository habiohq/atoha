package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/habiohq/habio"
)

// RecoveryAuthorizationSpec records the explicit human or policy decision that
// permits one new attempt after an earlier attempt. It never asserts replay safety.
type RecoveryAuthorizationSpec struct {
	ActionID          habio.ActionID
	PreviousAttemptID habio.AttemptID
	AuthorizedBy      string
	AuthorizedAt      time.Time
	Reason            string
}

// RecoveryAuthorization is immutable evidence required by RecoverAttempt.
type RecoveryAuthorization struct {
	actionID          habio.ActionID
	previousAttemptID habio.AttemptID
	authorizedBy      string
	authorizedAt      time.Time
	reason            string
}

func NewRecoveryAuthorization(spec RecoveryAuthorizationSpec) (RecoveryAuthorization, error) {
	if strings.TrimSpace(string(spec.ActionID)) == "" || strings.TrimSpace(string(spec.PreviousAttemptID)) == "" ||
		strings.TrimSpace(spec.AuthorizedBy) == "" || strings.TrimSpace(spec.Reason) == "" || spec.AuthorizedAt.IsZero() {
		return RecoveryAuthorization{}, fmt.Errorf("%w: all recovery authorization fields are required", ErrRecoveryNotAuthorized)
	}
	if string(spec.ActionID) != strings.TrimSpace(string(spec.ActionID)) ||
		string(spec.PreviousAttemptID) != strings.TrimSpace(string(spec.PreviousAttemptID)) ||
		spec.AuthorizedBy != strings.TrimSpace(spec.AuthorizedBy) || spec.Reason != strings.TrimSpace(spec.Reason) {
		return RecoveryAuthorization{}, fmt.Errorf("%w: fields must not have surrounding whitespace", ErrRecoveryNotAuthorized)
	}
	return RecoveryAuthorization{
		actionID: spec.ActionID, previousAttemptID: spec.PreviousAttemptID,
		authorizedBy: spec.AuthorizedBy, authorizedAt: spec.AuthorizedAt, reason: spec.Reason,
	}, nil
}

func (a RecoveryAuthorization) ActionID() habio.ActionID           { return a.actionID }
func (a RecoveryAuthorization) PreviousAttemptID() habio.AttemptID { return a.previousAttemptID }
func (a RecoveryAuthorization) AuthorizedBy() string               { return a.authorizedBy }
func (a RecoveryAuthorization) AuthorizedAt() time.Time            { return a.authorizedAt }
func (a RecoveryAuthorization) Reason() string                     { return a.reason }

// RecoverAttempt requires prior facts and explicit authorization before it can
// create a new, separately identified physical execution attempt.
type RecoverAttempt struct {
	executor *ExecuteAction
	reader   RecoveryReader
}

func NewRecoverAttempt(executor *ExecuteAction, reader RecoveryReader) (*RecoverAttempt, error) {
	if executor == nil || reader == nil {
		return nil, fmt.Errorf("%w: executor and attempt reader are required", ErrInvalidInput)
	}
	return &RecoverAttempt{executor: executor, reader: reader}, nil
}

type RecoverInput struct {
	Action        habio.Action
	AttemptID     habio.AttemptID
	Authorization RecoveryAuthorization
}

func (u *RecoverAttempt) Recover(ctx context.Context, input RecoverInput) (ExecuteResult, error) {
	if u == nil {
		return ExecuteResult{}, stageError(StageRecover, fmt.Errorf("%w: use case is nil", ErrInvalidInput))
	}
	authorization := input.Authorization
	if authorization.ActionID() != input.Action.ID() || authorization.PreviousAttemptID() == "" {
		return ExecuteResult{}, stageError(StageRecover, ErrRecoveryNotAuthorized)
	}
	previous, err := u.reader.EventsByAttempt(ctx, authorization.PreviousAttemptID())
	if err != nil {
		return ExecuteResult{}, stageError(StageRecover, err)
	}
	if len(previous) == 0 {
		return ExecuteResult{}, stageError(StageRecover, errors.Join(ErrAttemptNotFound, ErrRecoveryNotAuthorized))
	}
	for _, event := range previous {
		if event.ActionID() != input.Action.ID() {
			return ExecuteResult{}, stageError(StageRecover, errors.Join(ErrActionMismatch, ErrRecoveryNotAuthorized))
		}
	}
	storedAction, found, err := u.reader.ActionByID(ctx, input.Action.ID())
	if err != nil {
		return ExecuteResult{}, stageError(StageRecover, err)
	}
	if !found || !sameAction(storedAction, input.Action) {
		return ExecuteResult{}, stageError(StageRecover, errors.Join(habio.ErrActionIdentityConflict, ErrRecoveryNotAuthorized))
	}
	return u.executor.execute(ctx, ExecuteInput{Action: input.Action, AttemptID: input.AttemptID}, &authorization)
}
