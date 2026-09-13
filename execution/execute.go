package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/habiohq/atoha"
)

const defaultRecordTimeout = 5 * time.Second

// ExecuteConfig supplies the consumer-defined ports used by ExecuteAction.
type ExecuteConfig struct {
	Admitter      atoha.Admitter
	Resolver      atoha.Resolver
	Provider      atoha.Provider
	Journal       Journal
	Now           func() time.Time
	RecordTimeout time.Duration
}

// ExecuteAction coordinates one initial physical execution attempt.
type ExecuteAction struct {
	admitter      atoha.Admitter
	resolver      atoha.Resolver
	provider      atoha.Provider
	journal       Journal
	now           func() time.Time
	recordTimeout time.Duration
}

// ExecuteInput identifies the immutable intent and the unique attempt to claim.
type ExecuteInput struct {
	Action    atoha.Action
	AttemptID atoha.AttemptID
}

// ExecuteResult keeps semantic execution knowledge separate from operation errors.
type ExecuteResult struct {
	Action    atoha.Action
	Attempt   atoha.ExecutionAttempt
	Target    atoha.ResolvedTarget
	Admission atoha.AdmissionStatus
	Dispatch  atoha.DispatchResult
	Outcome   atoha.Outcome
}

// NewExecuteAction constructs an execution use case at the composition root.
func NewExecuteAction(config ExecuteConfig) (*ExecuteAction, error) {
	if config.Admitter == nil || config.Resolver == nil || config.Provider == nil || config.Journal == nil {
		return nil, fmt.Errorf("%w: admitter, resolver, provider, and journal are required", ErrInvalidInput)
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	recordTimeout := config.RecordTimeout
	if recordTimeout <= 0 {
		recordTimeout = defaultRecordTimeout
	}
	return &ExecuteAction{
		admitter: config.Admitter, resolver: config.Resolver, provider: config.Provider,
		journal: config.Journal, now: now, recordTimeout: recordTimeout,
	}, nil
}

// Execute claims and runs an initial attempt. Duplicate AttemptIDs are rejected
// before admission, resolution, or provider I/O.
func (u *ExecuteAction) Execute(ctx context.Context, input ExecuteInput) (ExecuteResult, error) {
	return u.execute(ctx, input, nil)
}

func (u *ExecuteAction) execute(ctx context.Context, input ExecuteInput, authorization *RecoveryAuthorization) (ExecuteResult, error) {
	if u == nil {
		return ExecuteResult{}, stageError(StageValidate, fmt.Errorf("%w: use case is nil", ErrInvalidInput))
	}
	startedAt := u.now().UTC()
	attemptSpec := atoha.ExecutionAttemptSpec{ID: input.AttemptID, ActionID: input.Action.ID(), StartedAt: startedAt}
	if authorization != nil {
		attemptSpec.RecoveryOf = authorization.PreviousAttemptID()
	}
	attempt, err := atoha.NewExecutionAttempt(attemptSpec)
	if err != nil {
		return ExecuteResult{}, stageError(StageValidate, errors.Join(ErrInvalidInput, err))
	}
	result := ExecuteResult{Action: input.Action, Attempt: attempt}
	result.Outcome = outcome(atoha.AdmissionUnknown, atoha.DispatchUnknown, atoha.EffectUnknown)

	requested, err := actionRequestedEvent(input.Action, attempt.ID(), startedAt)
	if err != nil {
		return result, stageError(StageRecord, err)
	}
	started, err := attemptStartedEvent(input.Action, attempt, startedAt)
	if err != nil {
		return result, stageError(StageRecord, err)
	}
	initial := []atoha.ExecutionEvent{requested, started}
	if authorization != nil {
		authorized, eventErr := recoveryAuthorizedEvent(input.Action, attempt, *authorization, startedAt)
		if eventErr != nil {
			return result, stageError(StageRecord, eventErr)
		}
		initial = append(initial, authorized)
	}
	claimed, err := u.journal.BeginAttempt(ctx, input.Action, attempt, initial...)
	if err != nil {
		return result, stageError(StageRecord, err)
	}
	if !claimed {
		return result, stageError(StageValidate, ErrAttemptAlreadyStarted)
	}

	admission, admitErr := u.admitter.Admit(ctx, input.Action)
	switch admission {
	case atoha.AdmissionUnknown, atoha.AdmissionRejected, atoha.AdmissionAdmitted:
	default:
		admitErr = errors.Join(admitErr, fmt.Errorf("%w: invalid status %d", ErrAdmissionUnknown, admission))
		admission = atoha.AdmissionUnknown
	}
	result.Admission = admission
	if admission == atoha.AdmissionAdmitted || admission == atoha.AdmissionRejected {
		event, eventErr := admissionEvent(input.Action, attempt, admission, admitErr, u.now().UTC())
		if eventErr != nil {
			return result, stageError(StageRecord, eventErr)
		}
		if eventErr = u.appendAfterIO(ctx, event); eventErr != nil {
			return result, errors.Join(stageError(StageAdmit, admitErr), stageError(StageRecord, eventErr))
		}
	}
	if admission != atoha.AdmissionAdmitted || admitErr != nil {
		cause := admitErr
		if cause == nil && admission != atoha.AdmissionRejected {
			cause = ErrAdmissionUnknown
		}
		event, eventErr := notDispatchedEvent(input.Action, attempt, cause, u.now().UTC())
		if eventErr == nil {
			eventErr = u.appendAfterIO(ctx, event)
		}
		result.Outcome = outcome(admission, atoha.DispatchNotDispatched, atoha.EffectUnknown)
		return result, errors.Join(stageError(StageAdmit, cause), stageError(StageRecord, eventErr))
	}

	target, resolveErr := u.resolver.Resolve(ctx, input.Action)
	result.Target = target
	if resolveErr != nil {
		event, eventErr := notDispatchedEvent(input.Action, attempt, resolveErr, u.now().UTC())
		if eventErr == nil {
			eventErr = u.appendAfterIO(ctx, event)
		}
		result.Outcome = outcome(admission, atoha.DispatchNotDispatched, atoha.EffectUnknown)
		return result, errors.Join(stageError(StageResolve, resolveErr), stageError(StageRecord, eventErr))
	}

	dispatch, dispatchErr := u.provider.Dispatch(ctx, attempt, input.Action, target)
	dispatch, validationErr := normalizeDispatch(dispatch, target.Provider(), attempt.ID())
	result.Dispatch = dispatch
	dispatchErr = errors.Join(dispatchErr, validationErr)
	dispatchFacts, eventErr := dispatchEvents(input.Action, attempt, target, dispatch, dispatchErr, u.now().UTC())
	if eventErr == nil {
		eventErr = u.appendAfterIO(ctx, dispatchFacts...)
	}
	effect := atoha.EffectUnknown
	if dispatch.Status() == atoha.DispatchDispatched || dispatch.Status() == atoha.DispatchAcknowledged {
		effect = atoha.EffectUnverified
	}
	result.Outcome = outcome(admission, dispatch.Status(), effect)
	return result, errors.Join(stageError(StageDispatch, dispatchErr), stageError(StageRecord, eventErr))
}

func normalizeDispatch(result atoha.DispatchResult, provider string, attemptID atoha.AttemptID) (atoha.DispatchResult, error) {
	if result.Provider() == provider && result.AttemptID() == attemptID {
		return result, nil
	}
	unknown, err := atoha.NewDispatchResult(atoha.DispatchResultSpec{
		Provider: provider, AttemptID: attemptID, Status: atoha.DispatchUnknown,
	})
	if err != nil {
		return atoha.DispatchResult{}, errors.Join(ErrInvalidProviderResult, err)
	}
	return unknown, fmt.Errorf("%w: expected provider %q and attempt %q", ErrInvalidProviderResult, provider, attemptID)
}

func (u *ExecuteAction) appendAfterIO(parent context.Context, events ...atoha.ExecutionEvent) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), u.recordTimeout)
	defer cancel()
	return u.journal.AppendAll(ctx, events...)
}

func outcome(admission atoha.AdmissionStatus, dispatch atoha.DispatchStatus, effect atoha.EffectStatus) atoha.Outcome {
	value, _ := atoha.NewOutcome(atoha.OutcomeSpec{Admission: admission, Dispatch: dispatch, Effect: effect})
	return value
}

func sameAction(a, b atoha.Action) bool {
	return a.ID() == b.ID() && a.Target() == b.Target() && a.Name() == b.Name() &&
		a.RequestedAt().Equal(b.RequestedAt()) && bytes.Equal(a.Input(), b.Input())
}
