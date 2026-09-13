// Package projection contains rebuildable views derived from Atoha facts.
package projection

import (
	"errors"
	"fmt"

	"github.com/habiohq/atoha"
)

var (
	ErrInvalidIdentity = errors.New("atoha projection: invalid identity")
	ErrUnrelatedEvent  = errors.New("atoha projection: unrelated event")
)

// Attempt is a rebuildable current view for one ExecutionAttempt.
type Attempt struct {
	actionID     atoha.ActionID
	attemptID    atoha.AttemptID
	admission    atoha.AdmissionStatus
	dispatch     atoha.DispatchStatus
	effect       atoha.EffectStatus
	verification atoha.VerificationStatus

	admissionConflicted bool
	dispatchConflicted  bool
	effectConflicted    bool
}

// NewAttempt creates an empty, fully unknown view.
func NewAttempt(actionID atoha.ActionID, attemptID atoha.AttemptID) (*Attempt, error) {
	if actionID == "" || attemptID == "" {
		return nil, ErrInvalidIdentity
	}
	return &Attempt{actionID: actionID, attemptID: attemptID}, nil
}

// Apply incorporates one fact in the caller's canonical replay order. Late
// events therefore remain visible in the append log. Weaker dispatch knowledge
// never replaces stronger knowledge, while incompatible claims mark the view
// conflicted and return the affected dimension to unknown.
func (p *Attempt) Apply(event atoha.ExecutionEvent) error {
	if event.ActionID() != p.actionID {
		return fmt.Errorf("%w: action %s", ErrUnrelatedEvent, event.ActionID())
	}
	if event.AttemptID() != "" && event.AttemptID() != p.attemptID {
		return fmt.Errorf("%w: attempt %s", ErrUnrelatedEvent, event.AttemptID())
	}

	switch event.Kind() {
	case atoha.EventActionRequested, atoha.EventAttemptStarted, atoha.EventRecoveryAuthorized, atoha.EventObservationRecorded:
	case atoha.EventActionAdmitted:
		p.mergeAdmission(atoha.AdmissionAdmitted)
	case atoha.EventActionRejected:
		p.mergeAdmission(atoha.AdmissionRejected)
		p.mergeDispatch(atoha.DispatchNotDispatched)
	case atoha.EventDispatchUnknown:
		// Unknown facts do not erase stronger evidence already recorded.
	case atoha.EventNotDispatched:
		p.mergeDispatch(atoha.DispatchNotDispatched)
	case atoha.EventActionDispatched:
		p.mergeDispatch(atoha.DispatchDispatched)
	case atoha.EventProviderAcknowledged:
		p.mergeDispatch(atoha.DispatchAcknowledged)
	case atoha.EventEffectUnknown:
	case atoha.EventEffectUnverified:
		p.mergeEffect(atoha.EffectUnverified)
	case atoha.EventEffectObservedSatisfied:
		p.mergeEffect(atoha.EffectObservedSatisfied)
	case atoha.EventEffectObservedUnsatisfied:
		p.mergeEffect(atoha.EffectObservedUnsatisfied)
	case atoha.EventVerificationVerified:
		p.verification = atoha.VerificationVerified
	case atoha.EventVerificationUnsatisfied:
		p.verification = atoha.VerificationUnsatisfied
	case atoha.EventVerificationInconclusive:
		if p.verification == atoha.VerificationUnknown {
			p.verification = atoha.VerificationInconclusive
		}
	}
	return nil
}

// Outcome returns the current orthogonal knowledge projection.
func (p *Attempt) Outcome() atoha.Outcome {
	outcome, _ := atoha.NewOutcome(atoha.OutcomeSpec{
		Admission: p.admission, Dispatch: p.dispatch, Effect: p.effect,
	})
	return outcome
}

func (p *Attempt) Verification() atoha.VerificationStatus { return p.verification }

// Conflicted reports whether any outcome dimension contains incompatible facts.
func (p *Attempt) Conflicted() bool {
	return p.admissionConflicted || p.dispatchConflicted || p.effectConflicted
}

// AdmissionConflicted reports whether admission contains incompatible facts.
func (p *Attempt) AdmissionConflicted() bool { return p.admissionConflicted }

// DispatchConflicted reports whether dispatch contains incompatible facts.
func (p *Attempt) DispatchConflicted() bool { return p.dispatchConflicted }

// EffectConflicted reports whether effect contains incompatible facts.
func (p *Attempt) EffectConflicted() bool { return p.effectConflicted }

func (p *Attempt) mergeAdmission(next atoha.AdmissionStatus) {
	if p.admissionConflicted {
		return
	}
	if p.admission == atoha.AdmissionUnknown || p.admission == next {
		p.admission = next
		return
	}
	p.admission = atoha.AdmissionUnknown
	p.admissionConflicted = true
}

func (p *Attempt) mergeDispatch(next atoha.DispatchStatus) {
	if p.dispatchConflicted {
		return
	}
	if p.dispatch == atoha.DispatchUnknown || p.dispatch == next {
		p.dispatch = next
		return
	}
	if p.dispatch == atoha.DispatchDispatched && next == atoha.DispatchAcknowledged {
		p.dispatch = next
		return
	}
	if p.dispatch == atoha.DispatchAcknowledged && next == atoha.DispatchDispatched {
		return
	}
	p.dispatch = atoha.DispatchUnknown
	p.dispatchConflicted = true
}

func (p *Attempt) mergeEffect(next atoha.EffectStatus) {
	if p.effectConflicted {
		return
	}
	if p.effect == atoha.EffectUnknown || p.effect == next || p.effect == atoha.EffectUnverified {
		p.effect = next
		return
	}
	if next == atoha.EffectUnverified {
		return
	}
	p.effect = atoha.EffectUnknown
	p.effectConflicted = true
}
