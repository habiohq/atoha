package execution

import (
	"context"
	"fmt"

	"github.com/habiohq/atoha"
	"github.com/habiohq/atoha/projection"
)

// AttemptView is a rebuildable application query result.
type AttemptView struct {
	ActionID            atoha.ActionID
	AttemptID           atoha.AttemptID
	Outcome             atoha.Outcome
	Verification        atoha.VerificationStatus
	AdmissionConflicted bool
	DispatchConflicted  bool
	EffectConflicted    bool
}

// GetAttempt rebuilds one view from canonical facts on every call.
type GetAttempt struct{ reader AttemptEventReader }

func NewGetAttempt(reader AttemptEventReader) (*GetAttempt, error) {
	if reader == nil {
		return nil, fmt.Errorf("%w: attempt reader is required", ErrInvalidInput)
	}
	return &GetAttempt{reader: reader}, nil
}

func (u *GetAttempt) Get(ctx context.Context, attemptID atoha.AttemptID) (AttemptView, error) {
	if u == nil || attemptID == "" {
		return AttemptView{}, stageError(StageValidate, ErrInvalidInput)
	}
	events, err := u.reader.EventsByAttempt(ctx, attemptID)
	if err != nil {
		return AttemptView{}, stageError(StageRecord, err)
	}
	if len(events) == 0 {
		return AttemptView{}, stageError(StageValidate, ErrAttemptNotFound)
	}
	actionID := events[0].ActionID()
	view, err := projection.NewAttempt(actionID, attemptID)
	if err != nil {
		return AttemptView{}, stageError(StageValidate, err)
	}
	for _, event := range events {
		if err := view.Apply(event); err != nil {
			return AttemptView{}, stageError(StageRecord, err)
		}
	}
	return AttemptView{
		ActionID: actionID, AttemptID: attemptID, Outcome: view.Outcome(), Verification: view.Verification(),
		AdmissionConflicted: view.AdmissionConflicted(), DispatchConflicted: view.DispatchConflicted(),
		EffectConflicted: view.EffectConflicted(),
	}, nil
}

// IncompleteAttempt identifies a claimed attempt with no terminal dispatch fact.
// Detection is informational and never triggers automatic replay.
type IncompleteAttempt struct {
	ActionID  atoha.ActionID  `json:"action_id"`
	AttemptID atoha.AttemptID `json:"attempt_id"`
}

type ScanIncompleteAttempts struct {
	lister AttemptLister
	reader AttemptEventReader
}

func NewScanIncompleteAttempts(lister AttemptLister, reader AttemptEventReader) (*ScanIncompleteAttempts, error) {
	if lister == nil || reader == nil {
		return nil, fmt.Errorf("%w: attempt lister and reader are required", ErrInvalidInput)
	}
	return &ScanIncompleteAttempts{lister: lister, reader: reader}, nil
}

func (u *ScanIncompleteAttempts) Scan(ctx context.Context) ([]IncompleteAttempt, error) {
	if u == nil {
		return nil, stageError(StageValidate, ErrInvalidInput)
	}
	ids, err := u.lister.AttemptIDs(ctx)
	if err != nil {
		return nil, stageError(StageRecord, err)
	}
	result := make([]IncompleteAttempt, 0)
	for _, id := range ids {
		events, readErr := u.reader.EventsByAttempt(ctx, id)
		if readErr != nil {
			return nil, stageError(StageRecord, readErr)
		}
		if actionID, incomplete := incomplete(events); incomplete {
			result = append(result, IncompleteAttempt{ActionID: actionID, AttemptID: id})
		}
	}
	return result, nil
}

func incomplete(events []atoha.ExecutionEvent) (atoha.ActionID, bool) {
	var actionID atoha.ActionID
	started := false
	finished := false
	for _, event := range events {
		actionID = event.ActionID()
		switch event.Kind() {
		case atoha.EventAttemptStarted:
			started = true
		case atoha.EventActionRejected, atoha.EventNotDispatched, atoha.EventDispatchUnknown,
			atoha.EventActionDispatched, atoha.EventProviderAcknowledged:
			finished = true
		}
	}
	return actionID, started && !finished
}
