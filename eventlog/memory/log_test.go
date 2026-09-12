package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/habiohq/habio"
)

func TestAppendIsIdempotentAndAppendOnly(t *testing.T) {
	log := New()
	event := mustEvent(t, "event-1", habio.EventAttemptStarted)
	if err := log.Append(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), event); err != nil {
		t.Fatalf("identical duplicate error = %v", err)
	}
	if got := len(log.Events()); got != 1 {
		t.Fatalf("len(Events()) = %d; want 1", got)
	}

	conflict, err := habio.NewExecutionEvent(habio.ExecutionEventSpec{
		ID: "event-1", ActionID: "action-1", AttemptID: "attempt-1",
		Kind: habio.EventActionDispatched, OccurredAt: event.OccurredAt(), RecordedAt: event.RecordedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), conflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("conflicting duplicate error = %v; want ErrEventConflict", err)
	}
	if log.Events()[0].Kind() != habio.EventAttemptStarted {
		t.Fatal("conflicting duplicate replaced an append-only fact")
	}
}

func TestAppendPreservesCanonicalDeliveryOrder(t *testing.T) {
	log := New()
	laterOccurred := mustEvent(t, "event-later", habio.EventActionDispatched)
	earlierOccurred, err := habio.NewExecutionEvent(habio.ExecutionEventSpec{
		ID: "event-earlier", ActionID: "action-1", AttemptID: "attempt-1",
		Kind:       habio.EventActionAdmitted,
		OccurredAt: laterOccurred.OccurredAt().Add(-time.Minute), RecordedAt: laterOccurred.RecordedAt().Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), laterOccurred); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), earlierOccurred); err != nil {
		t.Fatal(err)
	}
	events := log.Events()
	if events[0].ID() != "event-later" || events[1].ID() != "event-earlier" {
		t.Fatal("Log reordered late facts instead of preserving append order")
	}
}

func TestBeginAttemptClaimsOnceAndAppendsAtomically(t *testing.T) {
	log := New()
	now := time.Now()
	attempt, err := habio.NewExecutionAttempt(habio.ExecutionAttemptSpec{
		ID: "attempt-1", ActionID: "action-1", StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	started := mustEvent(t, "event-started", habio.EventAttemptStarted)
	action, _ := habio.NewAction(habio.ActionSpec{ID: "action-1", Target: "target", Name: "name", RequestedAt: now})
	claimed, err := log.BeginAttempt(context.Background(), action, attempt, started)
	if err != nil || !claimed {
		t.Fatalf("BeginAttempt() = (%v, %v); want (true, nil)", claimed, err)
	}
	claimed, err = log.BeginAttempt(context.Background(), action, attempt, started)
	if err != nil || claimed {
		t.Fatalf("duplicate BeginAttempt() = (%v, %v); want (false, nil)", claimed, err)
	}
	ids, _ := log.AttemptIDs(context.Background())
	if len(ids) != 1 || ids[0] != attempt.ID() || len(log.Events()) != 1 {
		t.Fatalf("attempt IDs/events = %v/%d; want one of each", ids, len(log.Events()))
	}

	conflicting := mustEvent(t, "event-started", habio.EventActionDispatched)
	other, _ := habio.NewExecutionAttempt(habio.ExecutionAttemptSpec{
		ID: "attempt-2", ActionID: "action-1", StartedAt: now,
	})
	claimed, err = log.BeginAttempt(context.Background(), action, other, conflicting)
	if err == nil || claimed {
		t.Fatalf("conflicting BeginAttempt() = (%v, %v); want atomic failure", claimed, err)
	}
	ids, _ = log.AttemptIDs(context.Background())
	if len(ids) != 1 || len(log.Events()) != 1 {
		t.Fatal("failed batch changed the log")
	}
}

func TestAppendAllRejectsWholeConflictingBatch(t *testing.T) {
	log := New()
	first := mustEvent(t, "event-1", habio.EventAttemptStarted)
	conflict := mustEvent(t, "event-1", habio.EventActionDispatched)
	second := mustEvent(t, "event-2", habio.EventActionAdmitted)
	if err := log.AppendAll(context.Background(), first, conflict, second); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("AppendAll() error = %v; want ErrEventConflict", err)
	}
	if len(log.Events()) != 0 {
		t.Fatal("conflicting batch was partially appended")
	}
}

func TestBeginAttemptRejectsChangedActionIdentity(t *testing.T) {
	log := New()
	now := time.Now()
	firstAction, _ := habio.NewAction(habio.ActionSpec{ID: "action-1", Target: "target-a", Name: "name", RequestedAt: now})
	secondAction, _ := habio.NewAction(habio.ActionSpec{ID: "action-1", Target: "target-b", Name: "name", RequestedAt: now})
	firstAttempt, _ := habio.NewExecutionAttempt(habio.ExecutionAttemptSpec{ID: "attempt-1", ActionID: "action-1", StartedAt: now})
	secondAttempt, _ := habio.NewExecutionAttempt(habio.ExecutionAttemptSpec{ID: "attempt-2", ActionID: "action-1", StartedAt: now})
	if claimed, err := log.BeginAttempt(context.Background(), firstAction, firstAttempt, mustEvent(t, "event-1", habio.EventAttemptStarted)); err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	claimed, err := log.BeginAttempt(context.Background(), secondAction, secondAttempt, mustEventForAttempt(t, "event-2", "attempt-2", habio.EventAttemptStarted))
	if !errors.Is(err, ErrActionConflict) || claimed {
		t.Fatalf("changed action claim = %v, %v; want action conflict", claimed, err)
	}
}

func mustEvent(t *testing.T, id habio.EventID, kind habio.EventKind) habio.ExecutionEvent {
	return mustEventForAttempt(t, id, "attempt-1", kind)
}

func mustEventForAttempt(t *testing.T, id habio.EventID, attemptID habio.AttemptID, kind habio.EventKind) habio.ExecutionEvent {
	t.Helper()
	now := time.Now()
	event, err := habio.NewExecutionEvent(habio.ExecutionEventSpec{
		ID: id, ActionID: "action-1", AttemptID: attemptID, Kind: kind,
		OccurredAt: now, RecordedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}
