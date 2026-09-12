package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/habiohq/habio"
)

const defaultRecordTimeout = 5 * time.Second

// ExecuteConfig supplies the consumer-defined ports used by ExecuteAction.
type ExecuteConfig struct {
	Admitter      habio.Admitter
	Resolver      habio.Resolver
	Provider      habio.Provider
	Journal       Journal
	Now           func() time.Time
	RecordTimeout time.Duration
}

// ExecuteAction coordinates one initial physical execution attempt.
type ExecuteAction struct {
	admitter      habio.Admitter
	resolver      habio.Resolver
	provider      habio.Provider
	journal       Journal
	now           func() time.Time
	recordTimeout time.Duration
}

// ExecuteInput identifies the immutable intent and the unique attempt to claim.
type ExecuteInput struct {
	Action    habio.Action
	AttemptID habio.AttemptID
}

// ExecuteResult keeps semantic execution knowledge separate from operation errors.
type ExecuteResult struct {
	Action    habio.Action
	Attempt   habio.ExecutionAttempt
	Target    habio.ResolvedTarget
	Admission habio.AdmissionStatus
	Dispatch  habio.DispatchResult
	Outcome   habio.Outcome
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
	attemptSpec := habio.ExecutionAttemptSpec{ID: input.AttemptID, ActionID: input.Action.ID(), StartedAt: startedAt}
	if authorization != nil {
		attemptSpec.RecoveryOf = authorization.PreviousAttemptID()
	}
	attempt, err := habio.NewExecutionAttempt(attemptSpec)
	if err != nil {
		return ExecuteResult{}, stageError(StageValidate, errors.Join(ErrInvalidInput, err))
	}
	result := ExecuteResult{Action: input.Action, Attempt: attempt}
	result.Outcome = outcome(habio.AdmissionUnknown, habio.DispatchUnknown, habio.EffectUnknown)

	requested, err := actionRequestedEvent(input.Action, attempt.ID(), startedAt)
	if err != nil {
		return result, stageError(StageRecord, err)
	}
	started, err := attemptStartedEvent(input.Action, attempt, startedAt)
	if err != nil {
		return result, stageError(StageRecord, err)
	}
	initial := []habio.ExecutionEvent{requested, started}
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
	case habio.AdmissionUnknown, habio.AdmissionRejected, habio.AdmissionAdmitted:
	default:
		admitErr = errors.Join(admitErr, fmt.Errorf("%w: invalid status %d", ErrAdmissionUnknown, admission))
		admission = habio.AdmissionUnknown
	}
	result.Admission = admission
	if admission == habio.AdmissionAdmitted || admission == habio.AdmissionRejected {
		event, eventErr := admissionEvent(input.Action, attempt, admission, admitErr, u.now().UTC())
		if eventErr != nil {
			return result, stageError(StageRecord, eventErr)
		}
		if eventErr = u.appendAfterIO(ctx, event); eventErr != nil {
			return result, errors.Join(stageError(StageAdmit, admitErr), stageError(StageRecord, eventErr))
		}
	}
	if admission != habio.AdmissionAdmitted || admitErr != nil {
		cause := admitErr
		if cause == nil && admission != habio.AdmissionRejected {
			cause = ErrAdmissionUnknown
		}
		event, eventErr := notDispatchedEvent(input.Action, attempt, cause, u.now().UTC())
		if eventErr == nil {
			eventErr = u.appendAfterIO(ctx, event)
		}
		result.Outcome = outcome(admission, habio.DispatchNotDispatched, habio.EffectUnknown)
		return result, errors.Join(stageError(StageAdmit, cause), stageError(StageRecord, eventErr))
	}

	target, resolveErr := u.resolver.Resolve(ctx, input.Action)
	result.Target = target
	if resolveErr != nil {
		event, eventErr := notDispatchedEvent(input.Action, attempt, resolveErr, u.now().UTC())
		if eventErr == nil {
			eventErr = u.appendAfterIO(ctx, event)
		}
		result.Outcome = outcome(admission, habio.DispatchNotDispatched, habio.EffectUnknown)
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
	effect := habio.EffectUnknown
	if dispatch.Status() == habio.DispatchDispatched || dispatch.Status() == habio.DispatchAcknowledged {
		effect = habio.EffectUnverified
	}
	result.Outcome = outcome(admission, dispatch.Status(), effect)
	return result, errors.Join(stageError(StageDispatch, dispatchErr), stageError(StageRecord, eventErr))
}

func normalizeDispatch(result habio.DispatchResult, provider string, attemptID habio.AttemptID) (habio.DispatchResult, error) {
	if result.Provider() == provider && result.AttemptID() == attemptID {
		return result, nil
	}
	unknown, err := habio.NewDispatchResult(habio.DispatchResultSpec{
		Provider: provider, AttemptID: attemptID, Status: habio.DispatchUnknown,
	})
	if err != nil {
		return habio.DispatchResult{}, errors.Join(ErrInvalidProviderResult, err)
	}
	return unknown, fmt.Errorf("%w: expected provider %q and attempt %q", ErrInvalidProviderResult, provider, attemptID)
}

func (u *ExecuteAction) appendAfterIO(parent context.Context, events ...habio.ExecutionEvent) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), u.recordTimeout)
	defer cancel()
	return u.journal.AppendAll(ctx, events...)
}

func outcome(admission habio.AdmissionStatus, dispatch habio.DispatchStatus, effect habio.EffectStatus) habio.Outcome {
	value, _ := habio.NewOutcome(habio.OutcomeSpec{Admission: admission, Dispatch: dispatch, Effect: effect})
	return value
}

func sameAction(a, b habio.Action) bool {
	return a.ID() == b.ID() && a.Target() == b.Target() && a.Name() == b.Name() &&
		a.RequestedAt().Equal(b.RequestedAt()) && bytes.Equal(a.Input(), b.Input())
}
