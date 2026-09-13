package atoha

import (
	"context"
	"errors"
	"testing"
)

func TestOutcomeZeroValueIsUnknown(t *testing.T) {
	var outcome Outcome
	if !outcome.IsUnknown() {
		t.Fatalf("zero Outcome = %+v; want all dimensions unknown", outcome)
	}
	if outcome.Admission() != AdmissionUnknown || outcome.Dispatch() != DispatchUnknown || outcome.Effect() != EffectUnknown {
		t.Fatalf("zero Outcome has unexpected dimensions: %+v", outcome)
	}
}

func TestOutcomeKeepsKnowledgeDimensionsSeparate(t *testing.T) {
	outcome, err := NewOutcome(OutcomeSpec{
		Admission: AdmissionAdmitted,
		Dispatch:  DispatchAcknowledged,
		Effect:    EffectUnverified,
	})
	if err != nil {
		t.Fatalf("NewOutcome() error = %v", err)
	}

	if outcome.Admission() != AdmissionAdmitted {
		t.Fatalf("Admission() = %v; want admitted", outcome.Admission())
	}
	if outcome.Dispatch() != DispatchAcknowledged {
		t.Fatalf("Dispatch() = %v; want acknowledged", outcome.Dispatch())
	}
	if outcome.Effect() != EffectUnverified {
		t.Fatalf("Effect() = %v; want unverified", outcome.Effect())
	}
}

func TestOutcomeAllowsContradictoryEvidenceProjection(t *testing.T) {
	// A dispatch subsystem can report not-dispatched while an independent
	// observation reports the desired state. Outcome retains both claims; a
	// later projection policy decides how to expose the conflict.
	outcome, err := NewOutcome(OutcomeSpec{
		Admission: AdmissionAdmitted,
		Dispatch:  DispatchNotDispatched,
		Effect:    EffectObservedSatisfied,
	})
	if err != nil {
		t.Fatalf("NewOutcome() error = %v", err)
	}
	if outcome.Dispatch() != DispatchNotDispatched || outcome.Effect() != EffectObservedSatisfied {
		t.Fatalf("Outcome discarded contradictory knowledge: %+v", outcome)
	}
}

func TestNewOutcomeRejectsUnknownEnumValues(t *testing.T) {
	tests := []OutcomeSpec{
		{Admission: AdmissionStatus(255)},
		{Dispatch: DispatchStatus(255)},
		{Effect: EffectStatus(255)},
	}
	for _, spec := range tests {
		if _, err := NewOutcome(spec); !errors.Is(err, ErrInvalidOutcome) {
			t.Errorf("NewOutcome(%+v) error = %v; want ErrInvalidOutcome", spec, err)
		}
	}
}

func TestTransportErrorDoesNotDeterminePhysicalOutcome(t *testing.T) {
	outcome, err := ambiguousTimeout()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v; want context deadline exceeded", err)
	}
	if outcome.Dispatch() != DispatchUnknown || outcome.Effect() != EffectUnknown {
		t.Fatalf("timeout was converted into physical certainty: %+v", outcome)
	}
}

func ambiguousTimeout() (Outcome, error) {
	return Outcome{}, context.DeadlineExceeded
}

func TestStatusStrings(t *testing.T) {
	if AdmissionRejected.String() != "rejected" {
		t.Fatalf("AdmissionRejected.String() = %q", AdmissionRejected.String())
	}
	if DispatchAcknowledged.String() != "acknowledged" {
		t.Fatalf("DispatchAcknowledged.String() = %q", DispatchAcknowledged.String())
	}
	if EffectUnverified.String() != "unverified" {
		t.Fatalf("EffectUnverified.String() = %q", EffectUnverified.String())
	}
}
