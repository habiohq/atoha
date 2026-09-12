package execution

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/habiohq/habio"
)

// VerifyConfig supplies independent observation and verification ports.
type VerifyConfig struct {
	Resolver      habio.Resolver
	Observer      habio.Observer
	Verifier      habio.Verifier
	Journal       Journal
	Reader        AttemptEventReader
	Actions       ActionReader
	Now           func() time.Time
	RecordTimeout time.Duration
}

// VerifyAttempt observes and assesses an existing attempt without dispatching it.
type VerifyAttempt struct {
	resolver      habio.Resolver
	observer      habio.Observer
	verifier      habio.Verifier
	journal       Journal
	reader        AttemptEventReader
	actions       ActionReader
	now           func() time.Time
	recordTimeout time.Duration
}

type VerifyInput struct {
	Action    habio.Action
	AttemptID habio.AttemptID
}

type VerifyResult struct {
	ActionID     habio.ActionID
	AttemptID    habio.AttemptID
	Target       habio.ResolvedTarget
	Observations []habio.Observation
	Verification habio.VerificationResult
}

func NewVerifyAttempt(config VerifyConfig) (*VerifyAttempt, error) {
	if config.Resolver == nil || config.Observer == nil || config.Verifier == nil || config.Journal == nil || config.Reader == nil || config.Actions == nil {
		return nil, fmt.Errorf("%w: resolver, observer, verifier, journal, event reader, and action reader are required", ErrInvalidInput)
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	recordTimeout := config.RecordTimeout
	if recordTimeout <= 0 {
		recordTimeout = defaultRecordTimeout
	}
	return &VerifyAttempt{
		resolver: config.Resolver, observer: config.Observer, verifier: config.Verifier,
		journal: config.Journal, reader: config.Reader, actions: config.Actions, now: now, recordTimeout: recordTimeout,
	}, nil
}

func (u *VerifyAttempt) Verify(ctx context.Context, input VerifyInput) (VerifyResult, error) {
	if u == nil || input.AttemptID == "" || input.Action.ID() == "" {
		return VerifyResult{}, stageError(StageValidate, ErrInvalidInput)
	}
	result := VerifyResult{ActionID: input.Action.ID(), AttemptID: input.AttemptID}
	events, err := u.reader.EventsByAttempt(ctx, input.AttemptID)
	if err != nil {
		return result, stageError(StageRecord, err)
	}
	if len(events) == 0 {
		return result, stageError(StageValidate, ErrAttemptNotFound)
	}
	for _, event := range events {
		if event.ActionID() != input.Action.ID() {
			return result, stageError(StageValidate, ErrActionMismatch)
		}
	}
	storedAction, found, err := u.actions.ActionByID(ctx, input.Action.ID())
	if err != nil {
		return result, stageError(StageRecord, err)
	}
	if !found {
		return result, stageError(StageValidate, ErrAttemptNotFound)
	}
	if !sameAction(storedAction, input.Action) {
		return result, stageError(StageValidate, habio.ErrActionIdentityConflict)
	}

	target, err := u.resolver.Resolve(ctx, input.Action)
	result.Target = target
	if err != nil {
		return result, stageError(StageResolve, err)
	}
	observations, observeErr := u.observer.Observe(ctx, input.Action, target)
	result.Observations = append([]habio.Observation(nil), observations...)
	checkedAt := u.now().UTC()
	verification, verifyErr := u.verifier.Verify(ctx, input.Action, observations, checkedAt)
	result.Verification = verification
	if verification.Status() == habio.VerificationUnknown && verifyErr == nil {
		verifyErr = ErrVerificationUnknown
	}

	facts := make([]habio.ExecutionEvent, 0, len(observations)+2)
	var buildErr error
	for _, observation := range observations {
		event, eventErr := observationEvent(input.Action, input.AttemptID, observation)
		if eventErr != nil {
			buildErr = errors.Join(buildErr, eventErr)
			continue
		}
		facts = append(facts, event)
	}
	verificationFacts, eventErr := verificationEvents(input.Action, input.AttemptID, verification, verifyErr, checkedAt)
	buildErr = errors.Join(buildErr, eventErr)
	facts = append(facts, verificationFacts...)
	var recordErr error
	if buildErr == nil && len(facts) != 0 {
		recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), u.recordTimeout)
		recordErr = u.journal.AppendAll(recordCtx, facts...)
		cancel()
	}
	return result, errors.Join(
		stageError(StageObserve, observeErr),
		stageError(StageVerify, verifyErr),
		stageError(StageRecord, errors.Join(buildErr, recordErr)),
	)
}
