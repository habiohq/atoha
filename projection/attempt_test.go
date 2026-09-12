package projection

import (
	"testing"
	"time"

	"github.com/habiohq/habio"
)

func TestAttemptReconstructsCurrentView(t *testing.T) {
	view, err := NewAttempt("action-1", "attempt-1")
	if err != nil {
		t.Fatal(err)
	}
	kinds := []habio.EventKind{
		habio.EventAttemptStarted,
		habio.EventActionAdmitted,
		habio.EventActionDispatched,
		habio.EventProviderAcknowledged,
		habio.EventObservationRecorded,
		habio.EventEffectObservedSatisfied,
		habio.EventVerificationVerified,
	}
	for i, kind := range kinds {
		if err := view.Apply(event(t, i, kind)); err != nil {
			t.Fatal(err)
		}
	}
	outcome := view.Outcome()
	if outcome.Admission() != habio.AdmissionAdmitted || outcome.Dispatch() != habio.DispatchAcknowledged || outcome.Effect() != habio.EffectObservedSatisfied {
		t.Fatalf("Outcome() = %+v; want admitted, acknowledged, observed satisfied", outcome)
	}
	if view.Verification() != habio.VerificationVerified || view.Conflicted() {
		t.Fatalf("verification=%v conflicted=%v", view.Verification(), view.Conflicted())
	}
}

func TestAttemptDoesNotRegressOnOutOfOrderDispatchEvidence(t *testing.T) {
	view, _ := NewAttempt("action-1", "attempt-1")
	if err := view.Apply(event(t, 1, habio.EventProviderAcknowledged)); err != nil {
		t.Fatal(err)
	}
	if err := view.Apply(event(t, 2, habio.EventActionDispatched)); err != nil {
		t.Fatal(err)
	}
	if got := view.Outcome().Dispatch(); got != habio.DispatchAcknowledged {
		t.Fatalf("Dispatch() = %v; weaker late fact regressed acknowledgement", got)
	}
}

func TestAttemptMakesContradictoryDispatchUnknown(t *testing.T) {
	view, _ := NewAttempt("action-1", "attempt-1")
	if err := view.Apply(event(t, 1, habio.EventNotDispatched)); err != nil {
		t.Fatal(err)
	}
	if err := view.Apply(event(t, 2, habio.EventProviderAcknowledged)); err != nil {
		t.Fatal(err)
	}
	if got := view.Outcome().Dispatch(); got != habio.DispatchUnknown || !view.Conflicted() {
		t.Fatalf("Dispatch()=%v conflicted=%v; want unknown conflict", got, view.Conflicted())
	}
}

func TestAttemptKeepsConflictedDimensionsUnknown(t *testing.T) {
	tests := []struct {
		name       string
		kinds      []habio.EventKind
		got        func(habio.Outcome) string
		conflicted func(*Attempt) bool
		want       string
	}{
		{
			name:  "admission",
			kinds: []habio.EventKind{habio.EventActionAdmitted, habio.EventActionRejected, habio.EventActionAdmitted},
			got:   func(outcome habio.Outcome) string { return outcome.Admission().String() },
			conflicted: func(view *Attempt) bool {
				return view.AdmissionConflicted() && !view.DispatchConflicted() && !view.EffectConflicted()
			},
			want: habio.AdmissionUnknown.String(),
		},
		{
			name:  "dispatch",
			kinds: []habio.EventKind{habio.EventNotDispatched, habio.EventProviderAcknowledged, habio.EventActionDispatched},
			got:   func(outcome habio.Outcome) string { return outcome.Dispatch().String() },
			conflicted: func(view *Attempt) bool {
				return !view.AdmissionConflicted() && view.DispatchConflicted() && !view.EffectConflicted()
			},
			want: habio.DispatchUnknown.String(),
		},
		{
			name:  "effect",
			kinds: []habio.EventKind{habio.EventEffectObservedSatisfied, habio.EventEffectObservedUnsatisfied, habio.EventEffectObservedSatisfied},
			got:   func(outcome habio.Outcome) string { return outcome.Effect().String() },
			conflicted: func(view *Attempt) bool {
				return !view.AdmissionConflicted() && !view.DispatchConflicted() && view.EffectConflicted()
			},
			want: habio.EffectUnknown.String(),
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
	e := event(t, 1, habio.EventAttemptStarted)
	other, err := habio.NewExecutionEvent(habio.ExecutionEventSpec{
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

func event(t *testing.T, n int, kind habio.EventKind) habio.ExecutionEvent {
	t.Helper()
	now := time.Date(2026, time.September, 6, 11, 0, n, 0, time.UTC)
	event, err := habio.NewExecutionEvent(habio.ExecutionEventSpec{
		ID:       habio.EventID("event-" + string(rune('a'+n))),
		ActionID: "action-1", AttemptID: "attempt-1", Kind: kind,
		OccurredAt: now, RecordedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}
