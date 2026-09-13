// Package memory provides a small local EventSink for examples and tests. It is
// not a durable event-store commitment.
package memory

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/habiohq/atoha"
)

// ErrEventConflict means an existing EventID was reused for different facts.
var ErrEventConflict = errors.New("atoha memory event log: event ID conflict")

// ErrActionConflict means an ActionID was reused for different immutable intent.
var ErrActionConflict = atoha.ErrActionIdentityConflict

// Log retains immutable events in append order.
type Log struct {
	mu           sync.RWMutex
	events       []atoha.ExecutionEvent
	byID         map[atoha.EventID]atoha.ExecutionEvent
	actions      map[atoha.ActionID]atoha.Action
	attempts     map[atoha.AttemptID]struct{}
	attemptOrder []atoha.AttemptID
}

// New returns an empty memory Log.
func New() *Log {
	return &Log{
		byID:     make(map[atoha.EventID]atoha.ExecutionEvent),
		actions:  make(map[atoha.ActionID]atoha.Action),
		attempts: make(map[atoha.AttemptID]struct{}),
	}
}

// Append adds event, treats an identical EventID and fact as an idempotent
// duplicate, and rejects conflicting reuse of an EventID.
func (l *Log) Append(ctx context.Context, event atoha.ExecutionEvent) error {
	return l.AppendAll(ctx, event)
}

// BeginAttempt atomically claims attempt and appends its initial facts. It
// returns false without changing the log when the identity was already claimed.
func (l *Log) BeginAttempt(ctx context.Context, action atoha.Action, attempt atoha.ExecutionAttempt, events ...atoha.ExecutionEvent) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.initialize()
	if attempt.ActionID() != action.ID() {
		return false, fmt.Errorf("%w: attempt does not match action", ErrActionConflict)
	}
	if existing, ok := l.actions[action.ID()]; ok && !equalAction(existing, action) {
		return false, fmt.Errorf("%w: %s", ErrActionConflict, action.ID())
	}
	if _, ok := l.attempts[attempt.ID()]; ok {
		return false, nil
	}
	for _, event := range events {
		if event.AttemptID() != attempt.ID() || event.ActionID() != attempt.ActionID() {
			return false, fmt.Errorf("atoha memory event log: initial event does not match claimed attempt")
		}
	}
	if err := l.preflight(events); err != nil {
		return false, err
	}
	l.attempts[attempt.ID()] = struct{}{}
	l.actions[action.ID()] = action
	l.attemptOrder = append(l.attemptOrder, attempt.ID())
	l.appendPrepared(events)
	return true, nil
}

// AppendAll atomically appends events and is idempotent for identical EventIDs.
func (l *Log) AppendAll(ctx context.Context, events ...atoha.ExecutionEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.initialize()
	if err := l.preflight(events); err != nil {
		return err
	}
	l.appendPrepared(events)
	return nil
}

// Events returns a copy of events in canonical append order.
func (l *Log) Events() []atoha.ExecutionEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]atoha.ExecutionEvent(nil), l.events...)
}

// EventsByAttempt returns attempt facts in canonical append order.
func (l *Log) EventsByAttempt(ctx context.Context, attemptID atoha.AttemptID) ([]atoha.ExecutionEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	var result []atoha.ExecutionEvent
	for _, event := range l.events {
		if event.AttemptID() == attemptID {
			result = append(result, event)
		}
	}
	return result, nil
}

// ActionByID returns the immutable intent registered during the first claim.
func (l *Log) ActionByID(ctx context.Context, actionID atoha.ActionID) (atoha.Action, bool, error) {
	if err := ctx.Err(); err != nil {
		return atoha.Action{}, false, err
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	action, ok := l.actions[actionID]
	return action, ok, nil
}

// AttemptIDs returns claimed attempt identities in claim order.
func (l *Log) AttemptIDs(ctx context.Context) ([]atoha.AttemptID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]atoha.AttemptID(nil), l.attemptOrder...), nil
}

func (l *Log) initialize() {
	if l.byID == nil {
		l.byID = make(map[atoha.EventID]atoha.ExecutionEvent)
	}
	if l.attempts == nil {
		l.attempts = make(map[atoha.AttemptID]struct{})
	}
	if l.actions == nil {
		l.actions = make(map[atoha.ActionID]atoha.Action)
	}
}

func (l *Log) preflight(events []atoha.ExecutionEvent) error {
	batch := make(map[atoha.EventID]atoha.ExecutionEvent, len(events))
	for _, event := range events {
		if existing, ok := l.byID[event.ID()]; ok && !equal(existing, event) {
			return fmt.Errorf("%w: %s", ErrEventConflict, event.ID())
		}
		if existing, ok := batch[event.ID()]; ok && !equal(existing, event) {
			return fmt.Errorf("%w: %s", ErrEventConflict, event.ID())
		}
		batch[event.ID()] = event
	}
	return nil
}

func (l *Log) appendPrepared(events []atoha.ExecutionEvent) {
	for _, event := range events {
		if _, ok := l.byID[event.ID()]; ok {
			continue
		}
		l.byID[event.ID()] = event
		l.events = append(l.events, event)
	}
}

func equal(a, b atoha.ExecutionEvent) bool {
	return a.ID() == b.ID() &&
		a.ActionID() == b.ActionID() &&
		a.AttemptID() == b.AttemptID() &&
		a.Kind() == b.Kind() &&
		a.OccurredAt().Equal(b.OccurredAt()) &&
		a.RecordedAt().Equal(b.RecordedAt()) &&
		bytes.Equal(a.Data(), b.Data())
}

func equalAction(a, b atoha.Action) bool {
	return a.ID() == b.ID() && a.Target() == b.Target() && a.Name() == b.Name() &&
		a.RequestedAt().Equal(b.RequestedAt()) && bytes.Equal(a.Input(), b.Input())
}

var _ atoha.EventSink = (*Log)(nil)
