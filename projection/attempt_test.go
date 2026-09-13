package projection

import (
	"testing"
	"time"

	"github.com/habiohq/atoha"
)

func TestAttemptReconstructsCurrentView(t *testing.T) {
	view, err := NewAttempt("action-1", "attempt-1")
	if err != nil {
		t.Fatal(err)
	}
	kinds := []atoha.EventKind{
		atoha.EventAttemptStarted,
		atoha.EventActionAdmitted,
		atoha.EventActionDispatched,
		atoha.EventProviderAcknowledged,
		atoha.EventObservationRecorded,
		atoha.EventEffectObservedSatisfied,
		atoha.EventVerificationVerified,
	}
	for i, kind := range kinds {
		if err := view.Apply(event(t, i, kind)); err != nil {
			t.Fatal(err)
		}
	}
	outcome := view.Outcome()
	if outcome.Admission() != atoha.AdmissionAdmitted || outcome.Dispatch() != atoha.DispatchAcknowledged || outcome.Effect() != atoha.EffectObservedSatisfied {
		t.Fatalf("Outcome() = %+v; want admitted, acknowledged, observed satisfied", outcome)
	}
	if view.Verification() != atoha.VerificationVerified || view.Conflicted() {
		t.Fatalf("verification=%v conflicted=%v", view.Verification(), view.Conflicted())
	}
}

func TestAttemptDoesNotRegressOnOutOfOrderDispatchEvidence(t *testing.T) {
	view, _ := NewAttempt("action-1", "attempt-1")
	if err := view.Apply(event(t, 1, atoha.EventProviderAcknowledged)); err != nil {
		t.Fatal(err)
	}
	if err := view.Apply(event(t, 2, atoha.EventActionDispatched)); err != nil {
		t.Fatal(err)
	}
	if got := view.Outcome().Dispatch(); got != atoha.DispatchAcknowledged {
		t.Fatalf("Dispatch() = %v; weaker late fact regressed acknowledgement", got)
	}
}

func TestAttemptMakesContradictoryDispatchUnknown(t *testing.T) {
	view, _ := NewAttempt("action-1", "attempt-1")
	if err := view.Apply(event(t, 1, atoha.EventNotDispatched)); err != nil {
		t.Fatal(err)
	}
	if err := view.Apply(event(t, 2, atoha.EventProviderAcknowledged)); err != nil {
		t.Fatal(err)
	}
	if got := view.Outcome().Dispatch(); got != atoha.DispatchUnknown || !view.Conflicted() {
		t.Fatalf("Dispatch()=%v conflicted=%v; want unknown conflict", got, view.Conflicted())
	}
}

func TestAttemptKeepsConflictedDimensionsUnknown(t *testing.T) {
	tests := []struct {
		name       string
		kinds      []atoha.EventKind
		got        func(atoha.Outcome) string
		conflicted func(*Attempt) bool
		want       string
	}{
		{
			name:  "admission",
			kinds: []atoha.EventKind{atoha.EventActionAdmitted, atoha.EventActionRejected, atoha.EventActionAdmitted},
			got:   func(outcome atoha.Outcome) string { return outcome.Admission().String() },
			conflicted: func(view *Attempt) bool {
				return view.AdmissionConflicted() && !view.DispatchConflicted() && !view.EffectConflicted()
			},
			want: atoha.AdmissionUnknown.String(),
		},
		{
			name:  "dispatch",
			kinds: []atoha.EventKind{atoha.EventNotDispatched, atoha.EventProviderAcknowledged, atoha.EventActionDispatched},
			got:   func(outcome atoha.Outcome) string { return outcome.Dispatch().String() },
			conflicted: func(view *Attempt) bool {
				return !view.AdmissionConflicted() && view.DispatchConflicted() && !view.EffectConflicted()
			},
			want: atoha.DispatchUnknown.String(),
		},
		{
			name:  "effect",
			kinds: []atoha.EventKind{atoha.EventEffectObservedSatisfied, atoha.EventEffectObservedUnsatisfied, atoha.EventEffectObservedSatisfied},
			got:   func(outcome atoha.Outcome) string { return outcome.Effect().String() },
			conflicted: func(view *Attempt) bool {
				return !view.AdmissionConflicted() && !view.DispatchConflicted() && view.EffectConflicted()
			},
			want: atoha.EffectUnknown.String(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view, _ := NewAttempt("action-1", "attempt-1")
			for i, kind := range tt.kinds {
				if err := view.Apply(event(t, i, kind)); err != nil {
					t.Fatal(err)
				}
			}
			if got := tt.got(view.Outcome()); got != tt.want || !view.Conflicted() || !tt.conflicted(view) {
				t.Fatalf("value=%s conflicted=%v; want %s conflict", got, view.Conflicted(), tt.want)
			}
		})
	}
}

func TestAttemptRejectsUnrelatedEvents(t *testing.T) {
	view, _ := NewAttempt("action-1", "attempt-1")
	e := event(t, 1, atoha.EventAttemptStarted)
	other, err := atoha.NewExecutionEvent(atoha.ExecutionEventSpec{
		ID: e.ID(), ActionID: "other-action", AttemptID: e.AttemptID(), Kind: e.Kind(),
		OccurredAt: e.OccurredAt(), RecordedAt: e.RecordedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := view.Apply(other); err == nil {
		t.Fatal("Apply() accepted an unrelated event")
	}
}

func event(t *testing.T, n int, kind atoha.EventKind) atoha.ExecutionEvent {
	t.Helper()
	now := time.Date(2026, time.September, 6, 11, 0, n, 0, time.UTC)
	event, err := atoha.NewExecutionEvent(atoha.ExecutionEventSpec{
		ID:       atoha.EventID("event-" + string(rune('a'+n))),
		ActionID: "action-1", AttemptID: "attempt-1", Kind: kind,
		OccurredAt: now, RecordedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}
