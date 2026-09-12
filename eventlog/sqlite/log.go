// Package sqlite provides a durable execution Journal backed by SQLite.
package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/habiohq/habio"
	_ "modernc.org/sqlite"
)

var (
	ErrInvalidConfig  = errors.New("habio sqlite event log: invalid config")
	ErrEventConflict  = errors.New("habio sqlite event log: event ID conflict")
	ErrActionConflict = habio.ErrActionIdentityConflict
	ErrCorruptData    = errors.New("habio sqlite event log: corrupt data")
)

// Log persists attempt claims and immutable facts in one database.
type Log struct{ db *sql.DB }

// Open opens or creates a Journal. The caller owns Close.
func Open(path string) (*Log, error) {
	if strings.TrimSpace(path) == "" || path != strings.TrimSpace(path) {
		return nil, fmt.Errorf("%w: database path is required", ErrInvalidConfig)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection gives :memory: databases stable semantics and keeps write
	// serialization explicit. SQLite still provides process-safe file locking.
	db.SetMaxOpenConns(1)
	log := &Log{db: db}
	if err := log.initialize(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return log, nil
}

func (l *Log) initialize(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
		`CREATE TABLE IF NOT EXISTS actions (
			action_id TEXT PRIMARY KEY,
			target TEXT NOT NULL,
			name TEXT NOT NULL,
			input BLOB,
			requested_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS attempts (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			attempt_id TEXT NOT NULL UNIQUE,
			action_id TEXT NOT NULL,
			started_at TEXT NOT NULL,
			recovery_of TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS events (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL UNIQUE,
			action_id TEXT NOT NULL,
			attempt_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			occurred_at TEXT NOT NULL,
			recorded_at TEXT NOT NULL,
			data BLOB
		)`,
		`CREATE INDEX IF NOT EXISTS events_attempt_sequence ON events(attempt_id, sequence)`,
	}
	for _, statement := range statements {
		if _, err := l.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("habio sqlite event log: initialize: %w", err)
		}
	}
	return nil
}

func (l *Log) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	return l.db.Close()
}

// Append preserves compatibility with the core EventSink contract.
func (l *Log) Append(ctx context.Context, event habio.ExecutionEvent) error {
	return l.AppendAll(ctx, event)
}

// BeginAttempt atomically claims an AttemptID and appends its initial facts.
func (l *Log) BeginAttempt(ctx context.Context, action habio.Action, attempt habio.ExecutionAttempt, events ...habio.ExecutionEvent) (claimed bool, err error) {
	if l == nil || l.db == nil {
		return false, fmt.Errorf("%w: log is nil", ErrInvalidConfig)
	}
	if attempt.ActionID() != action.ID() {
		return false, fmt.Errorf("%w: attempt does not match action", ErrActionConflict)
	}
	for _, event := range events {
		if event.AttemptID() != attempt.ID() || event.ActionID() != attempt.ActionID() {
			return false, fmt.Errorf("%w: initial event does not match claimed attempt", ErrInvalidConfig)
		}
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	insertAction, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO actions(action_id, target, name, input, requested_at) VALUES (?, ?, ?, ?, ?)`,
		action.ID(), action.Target(), action.Name(), action.Input(), formatTime(action.RequestedAt()),
	)
	if err != nil {
		return false, err
	}
	actionRows, err := insertAction.RowsAffected()
	if err != nil {
		return false, err
	}
	if actionRows == 0 {
		var id, target, name, requestedAt string
		var input []byte
		if err := tx.QueryRowContext(ctx, `SELECT action_id, target, name, input, requested_at FROM actions WHERE action_id = ?`, action.ID()).Scan(&id, &target, &name, &input, &requestedAt); err != nil {
			return false, err
		}
		storedAt, err := time.Parse(time.RFC3339Nano, requestedAt)
		if err != nil {
			return false, fmt.Errorf("%w: requested_at: %v", ErrCorruptData, err)
		}
		if habio.ActionID(id) != action.ID() || target != action.Target() || name != action.Name() ||
			!bytes.Equal(input, action.Input()) || !storedAt.Equal(action.RequestedAt()) {
			return false, fmt.Errorf("%w: %s", ErrActionConflict, action.ID())
		}
	}
	recoveryOf, hasRecovery := attempt.RecoveryOf()
	var recovery any
	if hasRecovery {
		recovery = string(recoveryOf)
	}
	insert, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO attempts(attempt_id, action_id, started_at, recovery_of) VALUES (?, ?, ?, ?)`,
		attempt.ID(), attempt.ActionID(), formatTime(attempt.StartedAt()), recovery,
	)
	if err != nil {
		return false, err
	}
	rows, err := insert.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		if err := tx.Rollback(); err != nil {
			return false, err
		}
		return false, nil
	}
	if err = appendEvents(ctx, tx, events); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// AppendAll appends an event group atomically and idempotently by EventID.
func (l *Log) AppendAll(ctx context.Context, events ...habio.ExecutionEvent) (err error) {
	if l == nil || l.db == nil {
		return fmt.Errorf("%w: log is nil", ErrInvalidConfig)
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = appendEvents(ctx, tx, events); err != nil {
		return err
	}
	return tx.Commit()
}

func appendEvents(ctx context.Context, tx *sql.Tx, events []habio.ExecutionEvent) error {
	for _, event := range events {
		result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO events(
			event_id, action_id, attempt_id, kind, occurred_at, recorded_at, data
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			event.ID(), event.ActionID(), event.AttemptID(), event.Kind(),
			formatTime(event.OccurredAt()), formatTime(event.RecordedAt()), event.Data(),
		)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			existing, err := readEvent(tx.QueryRowContext(ctx, `SELECT event_id, action_id, attempt_id, kind, occurred_at, recorded_at, data FROM events WHERE event_id = ?`, event.ID()))
			if err != nil {
				return err
			}
			if !equal(existing, event) {
				return fmt.Errorf("%w: %s", ErrEventConflict, event.ID())
			}
		}
	}
	return nil
}

// EventsByAttempt returns facts in durable append sequence.
func (l *Log) EventsByAttempt(ctx context.Context, attemptID habio.AttemptID) ([]habio.ExecutionEvent, error) {
	if l == nil || l.db == nil {
		return nil, fmt.Errorf("%w: log is nil", ErrInvalidConfig)
	}
	rows, err := l.db.QueryContext(ctx, `SELECT event_id, action_id, attempt_id, kind, occurred_at, recorded_at, data FROM events WHERE attempt_id = ? ORDER BY sequence`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []habio.ExecutionEvent
	for rows.Next() {
		event, err := readEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// ActionByID returns the immutable intent registered during the first claim.
func (l *Log) ActionByID(ctx context.Context, actionID habio.ActionID) (habio.Action, bool, error) {
	if l == nil || l.db == nil {
		return habio.Action{}, false, fmt.Errorf("%w: log is nil", ErrInvalidConfig)
	}
	var id, target, name, requestedAt string
	var input []byte
	err := l.db.QueryRowContext(ctx, `SELECT action_id, target, name, input, requested_at FROM actions WHERE action_id = ?`, actionID).
		Scan(&id, &target, &name, &input, &requestedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return habio.Action{}, false, nil
	}
	if err != nil {
		return habio.Action{}, false, err
	}
	requested, err := time.Parse(time.RFC3339Nano, requestedAt)
	if err != nil {
		return habio.Action{}, false, fmt.Errorf("%w: requested_at: %v", ErrCorruptData, err)
	}
	action, err := habio.NewAction(habio.ActionSpec{
		ID: habio.ActionID(id), Target: target, Name: name, Input: input, RequestedAt: requested,
	})
	if err != nil {
		return habio.Action{}, false, fmt.Errorf("%w: %v", ErrCorruptData, err)
	}
	return action, true, nil
}

// AttemptIDs returns claimed identities in durable claim sequence.
func (l *Log) AttemptIDs(ctx context.Context) ([]habio.AttemptID, error) {
	if l == nil || l.db == nil {
		return nil, fmt.Errorf("%w: log is nil", ErrInvalidConfig)
	}
	rows, err := l.db.QueryContext(ctx, `SELECT attempt_id FROM attempts ORDER BY sequence`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []habio.AttemptID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, habio.AttemptID(id))
	}
	return ids, rows.Err()
}

type scanner interface{ Scan(...any) error }

func readEvent(row scanner) (habio.ExecutionEvent, error) {
	var id, actionID, attemptID, kind, occurred, recorded string
	var data []byte
	if err := row.Scan(&id, &actionID, &attemptID, &kind, &occurred, &recorded, &data); err != nil {
		return habio.ExecutionEvent{}, err
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, occurred)
	if err != nil {
		return habio.ExecutionEvent{}, fmt.Errorf("%w: occurred_at: %v", ErrCorruptData, err)
	}
	recordedAt, err := time.Parse(time.RFC3339Nano, recorded)
	if err != nil {
		return habio.ExecutionEvent{}, fmt.Errorf("%w: recorded_at: %v", ErrCorruptData, err)
	}
	event, err := habio.NewExecutionEvent(habio.ExecutionEventSpec{
		ID: habio.EventID(id), ActionID: habio.ActionID(actionID), AttemptID: habio.AttemptID(attemptID),
		Kind: habio.EventKind(kind), OccurredAt: occurredAt, RecordedAt: recordedAt, Data: data,
	})
	if err != nil {
		return habio.ExecutionEvent{}, fmt.Errorf("%w: %v", ErrCorruptData, err)
	}
	return event, nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func equal(a, b habio.ExecutionEvent) bool {
	return a.ID() == b.ID() && a.ActionID() == b.ActionID() && a.AttemptID() == b.AttemptID() &&
		a.Kind() == b.Kind() && a.OccurredAt().Equal(b.OccurredAt()) &&
		a.RecordedAt().Equal(b.RecordedAt()) && bytes.Equal(a.Data(), b.Data())
}

var _ habio.EventSink = (*Log)(nil)
