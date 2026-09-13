package atoha

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExecutionEventIsImmutable(t *testing.T) {
	now := time.Now()
	data := []byte("evidence")
	event, err := NewExecutionEvent(ExecutionEventSpec{
		ID: "event-1", ActionID: "action-1", AttemptID: "attempt-1",
		Kind: EventProviderAcknowledged, OccurredAt: now, RecordedAt: now, Data: data,
	})
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	got := event.Data()
	got[0] = 'Y'
	if string(event.Data()) != "evidence" {
		t.Fatal("ExecutionEvent did not retain immutable data")
	}
}

func TestNewExecutionEventValidation(t *testing.T) {
	now := time.Now()
	tests := []ExecutionEventSpec{
		{ActionID: "action-1", AttemptID: "attempt-1", Kind: EventAttemptStarted, OccurredAt: now, RecordedAt: now},
		{ID: "event-1", AttemptID: "attempt-1", Kind: EventAttemptStarted, OccurredAt: now, RecordedAt: now},
		{ID: "event-1", ActionID: "action-1", AttemptID: "attempt-1", Kind: "invented", OccurredAt: now, RecordedAt: now},
		{ID: "event-1", ActionID: "action-1", Kind: EventAttemptStarted, OccurredAt: now, RecordedAt: now},
		{ID: "event-1", ActionID: "action-1", AttemptID: "attempt-1", Kind: EventAttemptStarted, RecordedAt: now},
		{ID: "event-1", ActionID: "action-1", AttemptID: "attempt-1", Kind: EventAttemptStarted, OccurredAt: now},
	}
	for _, spec := range tests {
		if _, err := NewExecutionEvent(spec); !errors.Is(err, ErrInvalidExecutionEvent) {
			t.Errorf("NewExecutionEvent(%+v) error = %v; want ErrInvalidExecutionEvent", spec, err)
		}
	}
}

type discardSink struct{}

func (discardSink) Append(context.Context, ExecutionEvent) error { return nil }

var _ EventSink = discardSink{}
