package atoha

import (
	"errors"
	"testing"
	"time"
)

func TestNewAction(t *testing.T) {
	now := time.Date(2026, time.September, 6, 9, 0, 0, 0, time.UTC)
	input := []byte(`{"temperature":24}`)

	action, err := NewAction(ActionSpec{
		ID:          "action-1",
		Target:      "living-room-climate",
		Name:        "set_temperature",
		Input:       input,
		RequestedAt: now,
	})
	if err != nil {
		t.Fatalf("NewAction() error = %v", err)
	}

	input[0] = 'X'
	gotInput := action.Input()
	gotInput[0] = 'Y'

	if got := string(action.Input()); got != `{"temperature":24}` {
		t.Fatalf("Input() = %q; want immutable copy", got)
	}
	if action.ID() != "action-1" || action.Target() != "living-room-climate" || action.Name() != "set_temperature" {
		t.Fatalf("Action accessors returned unexpected values: %#v", action)
	}
	if !action.RequestedAt().Equal(now) {
		t.Fatalf("RequestedAt() = %v; want %v", action.RequestedAt(), now)
	}
}

func TestNewActionRejectsInvalidSpecs(t *testing.T) {
	now := time.Now()
	tests := []ActionSpec{
		{Target: "target", Name: "name", RequestedAt: now},
		{ID: " action-1", Target: "target", Name: "name", RequestedAt: now},
		{ID: "action-1", Name: "name", RequestedAt: now},
		{ID: "action-1", Target: " target", Name: "name", RequestedAt: now},
		{ID: "action-1", Target: "target", RequestedAt: now},
		{ID: "action-1", Target: "target", Name: "name"},
	}

	for _, spec := range tests {
		if _, err := NewAction(spec); !errors.Is(err, ErrInvalidAction) {
			t.Errorf("NewAction(%+v) error = %v; want ErrInvalidAction", spec, err)
		}
	}
}

func TestExecutionAttemptIdentityAndRecovery(t *testing.T) {
	now := time.Date(2026, time.September, 6, 9, 0, 0, 0, time.UTC)
	first, err := NewExecutionAttempt(ExecutionAttemptSpec{
		ID:        "attempt-1",
		ActionID:  "action-1",
		StartedAt: now,
	})
	if err != nil {
		t.Fatalf("NewExecutionAttempt(first) error = %v", err)
	}
	second, err := NewExecutionAttempt(ExecutionAttemptSpec{
		ID:         "attempt-2",
		ActionID:   "action-1",
		StartedAt:  now.Add(time.Second),
		RecoveryOf: first.ID(),
	})
	if err != nil {
		t.Fatalf("NewExecutionAttempt(second) error = %v", err)
	}

	if first.ID() == second.ID() {
		t.Fatal("attempt IDs must identify individual attempts")
	}
	if first.ActionID() != second.ActionID() {
		t.Fatal("both attempts should refer to the same immutable Action")
	}
	if _, ok := first.RecoveryOf(); ok {
		t.Fatal("initial attempt unexpectedly reports a recovery link")
	}
	if got, ok := second.RecoveryOf(); !ok || got != first.ID() {
		t.Fatalf("RecoveryOf() = %q, %v; want %q, true", got, ok, first.ID())
	}
}

func TestNewExecutionAttemptRejectsInvalidSpecs(t *testing.T) {
	now := time.Now()
	tests := []ExecutionAttemptSpec{
		{ActionID: "action-1", StartedAt: now},
		{ID: "attempt-1", StartedAt: now},
		{ID: "attempt-1", ActionID: "action-1"},
		{ID: "attempt-1", ActionID: " action-1", StartedAt: now},
		{ID: "attempt-1", ActionID: "action-1", StartedAt: now, RecoveryOf: " attempt-0"},
		{ID: "attempt-1", ActionID: "action-1", StartedAt: now, RecoveryOf: "attempt-1"},
	}

	for _, spec := range tests {
		if _, err := NewExecutionAttempt(spec); !errors.Is(err, ErrInvalidExecutionAttempt) {
			t.Errorf("NewExecutionAttempt(%+v) error = %v; want ErrInvalidExecutionAttempt", spec, err)
		}
	}
}
