package habio

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// EventID identifies one immutable execution fact.
type EventID string

// EventKind names a fact without implying that facts form a total lifecycle.
type EventKind string

const (
	EventActionRequested           EventKind = "action-requested"
	EventAttemptStarted            EventKind = "attempt-started"
	EventRecoveryAuthorized        EventKind = "recovery-authorized"
	EventActionAdmitted            EventKind = "action-admitted"
	EventActionRejected            EventKind = "action-rejected"
	EventDispatchUnknown           EventKind = "dispatch-unknown"
	EventNotDispatched             EventKind = "not-dispatched"
	EventActionDispatched          EventKind = "action-dispatched"
	EventProviderAcknowledged      EventKind = "provider-acknowledged"
	EventObservationRecorded       EventKind = "observation-recorded"
	EventEffectUnknown             EventKind = "effect-unknown"
	EventEffectUnverified          EventKind = "effect-unverified"
	EventEffectObservedSatisfied   EventKind = "effect-observed-satisfied"
	EventEffectObservedUnsatisfied EventKind = "effect-observed-unsatisfied"
	EventVerificationVerified      EventKind = "verification-verified"
	EventVerificationUnsatisfied   EventKind = "verification-unsatisfied"
	EventVerificationInconclusive  EventKind = "verification-inconclusive"
)

var ErrInvalidExecutionEvent = errors.New("habio: invalid execution event")

// ExecutionEventSpec contains one immutable fact. Data is optional opaque
// evidence and does not alter the core meaning of Kind.
type ExecutionEventSpec struct {
	ID         EventID
	ActionID   ActionID
	AttemptID  AttemptID
	Kind       EventKind
	OccurredAt time.Time
	RecordedAt time.Time
	Data       []byte
}

// ExecutionEvent is an append-only fact. Its zero value is invalid.
type ExecutionEvent struct {
	id         EventID
	actionID   ActionID
	attemptID  AttemptID
	kind       EventKind
	occurredAt time.Time
	recordedAt time.Time
	data       []byte
}

// NewExecutionEvent validates and copies an execution fact. ActionRequested may
// exist before an attempt; every other v0.1 kind requires AttemptID.
func NewExecutionEvent(spec ExecutionEventSpec) (ExecutionEvent, error) {
	if err := validateIdentity("event ID", string(spec.ID)); err != nil {
		return ExecutionEvent{}, fmt.Errorf("%w: %v", ErrInvalidExecutionEvent, err)
	}
	if err := validateIdentity("action ID", string(spec.ActionID)); err != nil {
		return ExecutionEvent{}, fmt.Errorf("%w: %v", ErrInvalidExecutionEvent, err)
	}
	if !spec.Kind.valid() {
		return ExecutionEvent{}, fmt.Errorf("%w: unsupported kind %q", ErrInvalidExecutionEvent, spec.Kind)
	}
	if spec.Kind != EventActionRequested {
		if err := validateIdentity("attempt ID", string(spec.AttemptID)); err != nil {
			return ExecutionEvent{}, fmt.Errorf("%w: %v", ErrInvalidExecutionEvent, err)
		}
	} else if spec.AttemptID != "" {
		if err := validateIdentity("attempt ID", string(spec.AttemptID)); err != nil {
			return ExecutionEvent{}, fmt.Errorf("%w: %v", ErrInvalidExecutionEvent, err)
		}
	}
	if spec.OccurredAt.IsZero() {
		return ExecutionEvent{}, fmt.Errorf("%w: occurrence time is required", ErrInvalidExecutionEvent)
	}
	if spec.RecordedAt.IsZero() {
		return ExecutionEvent{}, fmt.Errorf("%w: recorded time is required", ErrInvalidExecutionEvent)
	}

	return ExecutionEvent{
		id: spec.ID, actionID: spec.ActionID, attemptID: spec.AttemptID, kind: spec.Kind,
		occurredAt: spec.OccurredAt, recordedAt: spec.RecordedAt, data: cloneBytes(spec.Data),
	}, nil
}

func (e ExecutionEvent) ID() EventID { return e.id }

func (e ExecutionEvent) ActionID() ActionID { return e.actionID }

func (e ExecutionEvent) AttemptID() AttemptID { return e.attemptID }

func (e ExecutionEvent) Kind() EventKind { return e.kind }

func (e ExecutionEvent) OccurredAt() time.Time { return e.occurredAt }

func (e ExecutionEvent) RecordedAt() time.Time { return e.recordedAt }

func (e ExecutionEvent) Data() []byte { return cloneBytes(e.data) }

// EventSink receives execution facts. Append implementations define durable
// ordering and duplicate handling; the reference memory log is idempotent by
// EventID and rejects conflicting reuse of an ID.
type EventSink interface {
	Append(ctx context.Context, event ExecutionEvent) error
}

func (k EventKind) valid() bool {
	switch k {
	case EventActionRequested,
		EventAttemptStarted,
		EventRecoveryAuthorized,
		EventActionAdmitted,
		EventActionRejected,
		EventDispatchUnknown,
		EventNotDispatched,
		EventActionDispatched,
		EventProviderAcknowledged,
		EventObservationRecorded,
		EventEffectUnknown,
		EventEffectUnverified,
		EventEffectObservedSatisfied,
		EventEffectObservedUnsatisfied,
		EventVerificationVerified,
		EventVerificationUnsatisfied,
		EventVerificationInconclusive:
		return true
	default:
		return false
	}
}
