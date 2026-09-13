package atoha

import (
	"errors"
	"fmt"
)

// AdmissionStatus describes what is known about the decision to admit an
// Action for execution.
type AdmissionStatus uint8

const (
	// AdmissionUnknown means no conclusive admission decision is recorded.
	AdmissionUnknown AdmissionStatus = iota
	// AdmissionRejected means the Action was rejected by an enforcement point.
	AdmissionRejected
	// AdmissionAdmitted means the Action was allowed to proceed.
	AdmissionAdmitted
)

// DispatchStatus describes what is known about dispatch to a Provider.
type DispatchStatus uint8

const (
	// DispatchUnknown means available evidence cannot establish whether the
	// Provider operation was sent.
	DispatchUnknown DispatchStatus = iota
	// DispatchNotDispatched means there is evidence that no Provider operation
	// was sent.
	DispatchNotDispatched
	// DispatchDispatched means there is evidence that the Provider operation was
	// sent, but not necessarily accepted or physically executed.
	DispatchDispatched
	// DispatchAcknowledged means the Provider reported accepting the operation.
	// It does not establish a physical effect.
	DispatchAcknowledged
)

// EffectStatus describes what is known about the desired physical effect.
// Verification is modeled separately from the observation knowledge recorded
// here.
type EffectStatus uint8

const (
	// EffectUnknown means available evidence cannot determine the physical
	// effect. It is the safe default after an ambiguous dispatch failure.
	EffectUnknown EffectStatus = iota
	// EffectUnverified means dispatch knowledge exists but no sufficient
	// observation has established whether the desired effect is present.
	EffectUnverified
	// EffectObservedSatisfied means observations report the desired state. A
	// Verifier still determines whether those observations are sufficient.
	EffectObservedSatisfied
	// EffectObservedUnsatisfied means observations report that the desired state
	// is not satisfied.
	EffectObservedUnsatisfied
)

var ErrInvalidOutcome = errors.New("atoha: invalid outcome")

// OutcomeSpec describes independent knowledge about admission, dispatch, and
// physical effect. The axes are deliberately not collapsed into success or
// failure.
type OutcomeSpec struct {
	Admission AdmissionStatus
	Dispatch  DispatchStatus
	Effect    EffectStatus
}

// Outcome is an immutable projection of current execution knowledge.
//
// Its zero value is valid and means that all three dimensions are unknown.
// Transport and implementation errors are returned separately by operations;
// an Outcome never infers physical success or failure from a Go error.
type Outcome struct {
	admission AdmissionStatus
	dispatch  DispatchStatus
	effect    EffectStatus
}

// NewOutcome validates the individual knowledge dimensions. It intentionally
// permits combinations that appear contradictory because independent evidence
// can disagree; fact/projection policy decides how to surface that conflict.
func NewOutcome(spec OutcomeSpec) (Outcome, error) {
	if !spec.Admission.valid() {
		return Outcome{}, fmt.Errorf("%w: admission status %d", ErrInvalidOutcome, spec.Admission)
	}
	if !spec.Dispatch.valid() {
		return Outcome{}, fmt.Errorf("%w: dispatch status %d", ErrInvalidOutcome, spec.Dispatch)
	}
	if !spec.Effect.valid() {
		return Outcome{}, fmt.Errorf("%w: effect status %d", ErrInvalidOutcome, spec.Effect)
	}
	return Outcome{
		admission: spec.Admission,
		dispatch:  spec.Dispatch,
		effect:    spec.Effect,
	}, nil
}

// Admission returns current admission knowledge.
func (o Outcome) Admission() AdmissionStatus { return o.admission }

// Dispatch returns current dispatch knowledge.
func (o Outcome) Dispatch() DispatchStatus { return o.dispatch }

// Effect returns current physical-effect knowledge.
func (o Outcome) Effect() EffectStatus { return o.effect }

// IsUnknown reports whether no dimension contains conclusive knowledge.
func (o Outcome) IsUnknown() bool {
	return o.admission == AdmissionUnknown &&
		o.dispatch == DispatchUnknown &&
		o.effect == EffectUnknown
}

func (s AdmissionStatus) valid() bool { return s <= AdmissionAdmitted }

func (s DispatchStatus) valid() bool { return s <= DispatchAcknowledged }

func (s EffectStatus) valid() bool { return s <= EffectObservedUnsatisfied }

func (s AdmissionStatus) String() string {
	switch s {
	case AdmissionUnknown:
		return "unknown"
	case AdmissionRejected:
		return "rejected"
	case AdmissionAdmitted:
		return "admitted"
	default:
		return fmt.Sprintf("AdmissionStatus(%d)", s)
	}
}

func (s DispatchStatus) String() string {
	switch s {
	case DispatchUnknown:
		return "unknown"
	case DispatchNotDispatched:
		return "not-dispatched"
	case DispatchDispatched:
		return "dispatched"
	case DispatchAcknowledged:
		return "acknowledged"
	default:
		return fmt.Sprintf("DispatchStatus(%d)", s)
	}
}

func (s EffectStatus) String() string {
	switch s {
	case EffectUnknown:
		return "unknown"
	case EffectUnverified:
		return "unverified"
	case EffectObservedSatisfied:
		return "observed-satisfied"
	case EffectObservedUnsatisfied:
		return "observed-unsatisfied"
	default:
		return fmt.Sprintf("EffectStatus(%d)", s)
	}
}
