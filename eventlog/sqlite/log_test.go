package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/habiohq/atoha"
)

func TestAttemptClaimAndEventsSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atoha.db")
	log, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	attempt := testAttempt(t, "attempt-1")
	event := testEvent(t, "event-1", "attempt-1", atoha.EventAttemptStarted)
	action := testAction(t, "action-1", "target")
	claimed, err := log.BeginAttempt(context.Background(), action, attempt, event)
	if err != nil || !claimed {
		t.Fatalf("BeginAttempt() = %v, %v", claimed, err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}

	log, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	claimed, err = log.BeginAttempt(context.Background(), action, attempt, event)
	if err != nil || claimed {
		t.Fatalf("reopened BeginAttempt() = %v, %v; want false, nil", claimed, err)
	}
	events, err := log.EventsByAttempt(context.Background(), attempt.ID())
	if err != nil || len(events) != 1 || events[0].ID() != event.ID() {
		t.Fatalf("EventsByAttempt() = %#v, %v", events, err)
	}
	stored, found, err := log.ActionByID(context.Background(), action.ID())
	if err != nil || !found || stored.Target() != action.Target() {
		t.Fatalf("ActionByID() = %#v, %v, %v", stored, found, err)
	}
}

func TestConcurrentAttemptClaimHasOneWinner(t *testing.T) {
	log, err := Open(filepath.Join(t.TempDir(), "atoha.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	attempt := testAttempt(t, "attempt-concurrent")
	event := testEvent(t, "event-concurrent", attempt.ID(), atoha.EventAttemptStarted)
	action := testAction(t, "action-1", "target")
	var winners atomic.Int32
	var failures atomic.Int32
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			claimed, claimErr := log.BeginAttempt(context.Background(), action, attempt, event)
			if claimErr != nil {
				failures.Add(1)
			} else if claimed {
				winners.Add(1)
			}
		}()
	}
	wait.Wait()
	if winners.Load() != 1 || failures.Load() != 0 {
		t.Fatalf("winners/failures = %d/%d; want 1/0", winners.Load(), failures.Load())
	}
}

func TestActionIdentityCannotChangeAcrossAttempts(t *testing.T) {
	log, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	firstAttempt := testAttempt(t, "attempt-1")
	firstEvent := testEvent(t, "event-1", firstAttempt.ID(), atoha.EventAttemptStarted)
	if claimed, err := log.BeginAttempt(context.Background(), testAction(t, "action-1", "target-a"), firstAttempt, firstEvent); err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	secondAttempt := testAttempt(t, "attempt-2")
	secondEvent := testEvent(t, "event-2", secondAttempt.ID(), atoha.EventAttemptStarted)
	claimed, err := log.BeginAttempt(context.Background(), testAction(t, "action-1", "target-b"), secondAttempt, secondEvent)
	if !errors.Is(err, ErrActionConflict) || claimed {
		t.Fatalf("changed action claim = %v, %v; want action conflict", claimed, err)
	}
	ids, _ := log.AttemptIDs(context.Background())
	if len(ids) != 1 {
		t.Fatal("conflicting action partially claimed a second attempt")
	}
}

func TestAppendAllConflictRollsBackWholeBatch(t *testing.T) {
	log, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	existing := testEvent(t, "event-1", "attempt-1", atoha.EventAttemptStarted)
	if err := log.Append(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	newEvent := testEvent(t, "event-2", "attempt-1", atoha.EventActionAdmitted)
	conflict := testEvent(t, "event-1", "attempt-1", atoha.EventActionDispatched)
	if err := log.AppendAll(context.Background(), newEvent, conflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("AppendAll() error = %v; want conflict", err)
	}
	events, err := log.EventsByAttempt(context.Background(), "attempt-1")
	if err != nil || len(events) != 1 || events[0].ID() != existing.ID() {
		t.Fatalf("events after rollback = %#v, %v", events, err)
	}
}

func testAttempt(t *testing.T, id atoha.AttemptID) atoha.ExecutionAttempt {
	t.Helper()
	attempt, err := atoha.NewExecutionAttempt(atoha.ExecutionAttemptSpec{ID: id, ActionID: "action-1", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return attempt
}

func testAction(t *testing.T, id atoha.ActionID, target string) atoha.Action {
	t.Helper()
	action, err := atoha.NewAction(atoha.ActionSpec{ID: id, Target: target, Name: "turn_on", RequestedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func testEvent(t *testing.T, id atoha.EventID, attemptID atoha.AttemptID, kind atoha.EventKind) atoha.ExecutionEvent {
	t.Helper()
	now := time.Now()
	event, err := atoha.NewExecutionEvent(atoha.ExecutionEventSpec{
		ID: id, ActionID: "action-1", AttemptID: attemptID, Kind: kind, OccurredAt: now, RecordedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}
