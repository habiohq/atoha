// Package execution coordinates Atoha's application-level execution use cases.
// It depends only on core contracts and never imports a concrete provider,
// storage engine, or transport adapter.
package execution

import (
	"context"

	"github.com/habiohq/atoha"
)

// Journal is the durable boundary required before physical dispatch. BeginAttempt
// atomically claims one attempt identity and appends its initial facts. A false
// claimed value means that the attempt was already claimed and no facts changed.
// AppendAll appends a group of later facts atomically and idempotently by EventID.
type Journal interface {
	BeginAttempt(ctx context.Context, action atoha.Action, attempt atoha.ExecutionAttempt, events ...atoha.ExecutionEvent) (claimed bool, err error)
	AppendAll(ctx context.Context, events ...atoha.ExecutionEvent) error
}

// AttemptEventReader supplies canonical facts for one attempt.
type AttemptEventReader interface {
	EventsByAttempt(ctx context.Context, attemptID atoha.AttemptID) ([]atoha.ExecutionEvent, error)
}

// ActionReader retrieves the immutable intent registered for an ActionID.
type ActionReader interface {
	ActionByID(ctx context.Context, actionID atoha.ActionID) (atoha.Action, bool, error)
}

// RecoveryReader supplies both prior-attempt facts and their immutable Action.
type RecoveryReader interface {
	AttemptEventReader
	ActionReader
}

// AttemptLister lists claimed attempt identities for recovery inspection.
type AttemptLister interface {
	AttemptIDs(ctx context.Context) ([]atoha.AttemptID, error)
}
