package execution

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/habiohq/atoha"
	"github.com/habiohq/atoha/eventlog/memory"
)

var testNow = time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)

func TestExecuteAcknowledgedAndDuplicateAttemptIsBlockedBeforeIO(t *testing.T) {
	log := memory.New()
	var dispatches atomic.Int32
	useCase := mustExecutor(t, log, providerFunc(func(_ context.Context, attempt atoha.ExecutionAttempt, _ atoha.Action, _ atoha.ResolvedTarget) (atoha.DispatchResult, error) {
		dispatches.Add(1)
		receipt, _ := atoha.NewReceipt(atoha.ReceiptSpec{Provider: "fixture", AttemptID: attempt.ID(), ReceivedAt: testNow})
		return atoha.NewDispatchResult(atoha.DispatchResultSpec{
			Provider: "fixture", AttemptID: attempt.ID(), Status: atoha.DispatchAcknowledged, Receipt: &receipt,
		})
	}))
	action := mustAction(t, "action-1")
	result, err := useCase.Execute(context.Background(), ExecuteInput{Action: action, AttemptID: "attempt-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Admission() != atoha.AdmissionAdmitted || result.Outcome.Dispatch() != atoha.DispatchAcknowledged || result.Outcome.Effect() != atoha.EffectUnverified {
		t.Fatalf("Outcome() = %v/%v/%v", result.Outcome.Admission(), result.Outcome.Dispatch(), result.Outcome.Effect())
	}
	if _, err := useCase.Execute(context.Background(), ExecuteInput{Action: action, AttemptID: "attempt-1"}); !errors.Is(err, ErrAttemptAlreadyStarted) {
		t.Fatalf("duplicate Execute() error = %v; want ErrAttemptAlreadyStarted", err)
	}
	if dispatches.Load() != 1 {
		t.Fatalf("dispatch count = %d; want 1", dispatches.Load())
	}
	view := mustGet(t, log, "attempt-1")
	if view.Outcome.Dispatch() != atoha.DispatchAcknowledged || view.Outcome.Effect() != atoha.EffectUnverified {
		t.Fatal("persisted projection lost dispatch knowledge")
	}
}

func TestExecuteRejectedNeverResolvesOrDispatches(t *testing.T) {
	log := memory.New()
	var resolutions, dispatches atomic.Int32
	executor, err := NewExecuteAction(ExecuteConfig{
		Admitter: admitterFunc(func(context.Context, atoha.Action) (atoha.AdmissionStatus, error) {
			return atoha.AdmissionRejected, nil
		}),
		Resolver: resolverFunc(func(context.Context, atoha.Action) (atoha.ResolvedTarget, error) {
			resolutions.Add(1)
			return fixtureTarget()
		}),
		Provider: providerFunc(func(context.Context, atoha.ExecutionAttempt, atoha.Action, atoha.ResolvedTarget) (atoha.DispatchResult, error) {
			dispatches.Add(1)
			return atoha.DispatchResult{}, nil
		}),
		Journal: log, Now: func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(context.Background(), ExecuteInput{Action: mustAction(t, "action-rejected"), AttemptID: "attempt-rejected"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Admission() != atoha.AdmissionRejected || result.Outcome.Dispatch() != atoha.DispatchNotDispatched {
		t.Fatalf("Outcome() = %v/%v", result.Outcome.Admission(), result.Outcome.Dispatch())
	}
	if resolutions.Load() != 0 || dispatches.Load() != 0 {
		t.Fatalf("resolution/dispatch count = %d/%d; want 0/0", resolutions.Load(), dispatches.Load())
	}
}

func TestExecuteInvalidAdmissionIsUnknownAndNeverDispatches(t *testing.T) {
	log := memory.New()
	var dispatches atomic.Int32
	executor, err := NewExecuteAction(ExecuteConfig{
		Admitter: admitterFunc(func(context.Context, atoha.Action) (atoha.AdmissionStatus, error) {
			return atoha.AdmissionStatus(99), nil
		}),
		Resolver: resolverFunc(func(context.Context, atoha.Action) (atoha.ResolvedTarget, error) { return fixtureTarget() }),
		Provider: providerFunc(func(context.Context, atoha.ExecutionAttempt, atoha.Action, atoha.ResolvedTarget) (atoha.DispatchResult, error) {
			dispatches.Add(1)
			return atoha.DispatchResult{}, nil
		}),
		Journal: log, Now: func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(context.Background(), ExecuteInput{Action: mustAction(t, "action-invalid-admission"), AttemptID: "attempt-invalid-admission"})
	if !errors.Is(err, ErrAdmissionUnknown) {
		t.Fatalf("Execute() error = %v; want ErrAdmissionUnknown", err)
	}
	if result.Outcome.Admission() != atoha.AdmissionUnknown || result.Outcome.Dispatch() != atoha.DispatchNotDispatched {
		t.Fatalf("Outcome() = %v/%v", result.Outcome.Admission(), result.Outcome.Dispatch())
	}
	if dispatches.Load() != 0 {
		t.Fatal("invalid admission reached provider I/O")
	}
}

func TestExecuteAmbiguousProviderErrorIsRecordedAndNotRetried(t *testing.T) {
	log := memory.New()
	var dispatches atomic.Int32
	providerErr := context.DeadlineExceeded
	useCase := mustExecutor(t, log, providerFunc(func(_ context.Context, attempt atoha.ExecutionAttempt, _ atoha.Action, _ atoha.ResolvedTarget) (atoha.DispatchResult, error) {
		dispatches.Add(1)
		result, _ := atoha.NewDispatchResult(atoha.DispatchResultSpec{Provider: "fixture", AttemptID: attempt.ID(), Status: atoha.DispatchUnknown})
		return result, providerErr
	}))
	result, err := useCase.Execute(context.Background(), ExecuteInput{Action: mustAction(t, "action-timeout"), AttemptID: "attempt-timeout"})
	if !errors.Is(err, providerErr) {
		t.Fatalf("Execute() error = %v; want deadline", err)
	}
	if result.Outcome.Dispatch() != atoha.DispatchUnknown || result.Outcome.Effect() != atoha.EffectUnknown {
		t.Fatal("ambiguous dispatch was turned into a physical conclusion")
	}
	if dispatches.Load() != 1 {
		t.Fatal("ambiguous dispatch was retried")
	}
	view := mustGet(t, log, "attempt-timeout")
	if view.Outcome.Dispatch() != atoha.DispatchUnknown {
		t.Fatal("unknown dispatch fact was not replayable")
	}
}

func TestExecuteReturnsKnownDispatchWhenPostIORecordingFails(t *testing.T) {
	base := memory.New()
	journal := &failAppendJournal{Log: base, failAt: 2, err: errors.New("fixture: disk full")}
	useCase := mustExecutor(t, journal, providerFunc(func(_ context.Context, attempt atoha.ExecutionAttempt, _ atoha.Action, _ atoha.ResolvedTarget) (atoha.DispatchResult, error) {
		return atoha.NewDispatchResult(atoha.DispatchResultSpec{Provider: "fixture", AttemptID: attempt.ID(), Status: atoha.DispatchDispatched})
	}))
	result, err := useCase.Execute(context.Background(), ExecuteInput{Action: mustAction(t, "action-record-fail"), AttemptID: "attempt-record-fail"})
	if err == nil || result.Outcome.Dispatch() != atoha.DispatchDispatched || result.Outcome.Effect() != atoha.EffectUnverified {
		t.Fatalf("Execute() = dispatch %v, effect %v, err %v", result.Outcome.Dispatch(), result.Outcome.Effect(), err)
	}
	if result.Dispatch.Status() != atoha.DispatchDispatched {
		t.Fatal("known provider evidence was discarded after journal failure")
	}
}

func TestVerifyIsSeparateAndPersistsObservationAssessment(t *testing.T) {
	log := memory.New()
	action := mustAction(t, "action-verify")
	executor := mustExecutor(t, log, providerFunc(func(_ context.Context, attempt atoha.ExecutionAttempt, _ atoha.Action, _ atoha.ResolvedTarget) (atoha.DispatchResult, error) {
		return atoha.NewDispatchResult(atoha.DispatchResultSpec{Provider: "fixture", AttemptID: attempt.ID(), Status: atoha.DispatchDispatched})
	}))
	if _, err := executor.Execute(context.Background(), ExecuteInput{Action: action, AttemptID: "attempt-verify"}); err != nil {
		t.Fatal(err)
	}
	observation, _ := atoha.NewObservation(atoha.ObservationSpec{
		ID: "observation-1", Source: "fixture", Target: action.Target(), Value: []byte("on"),
		ObservedAt: testNow, RecordedAt: testNow,
	})
	verify, err := NewVerifyAttempt(VerifyConfig{
		Resolver: resolverFunc(func(context.Context, atoha.Action) (atoha.ResolvedTarget, error) { return fixtureTarget() }),
		Observer: observerFunc(func(context.Context, atoha.Action, atoha.ResolvedTarget) ([]atoha.Observation, error) {
			return []atoha.Observation{observation}, nil
		}),
		Verifier: verifierFunc(func(_ context.Context, _ atoha.Action, observations []atoha.Observation, asOf time.Time) (atoha.VerificationResult, error) {
			return atoha.NewVerificationResult(atoha.VerificationResultSpec{
				Status: atoha.VerificationVerified, Verifier: "fixture", CheckedAt: asOf,
				ObservationIDs: []atoha.ObservationID{observations[0].ID()}, Reason: "matches",
			})
		}),
		Journal: log, Reader: log, Actions: log, Now: func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := verify.Verify(context.Background(), VerifyInput{Action: action, AttemptID: "attempt-verify"})
	if err != nil || result.Verification.Status() != atoha.VerificationVerified {
		t.Fatalf("Verify() status/error = %v/%v", result.Verification.Status(), err)
	}
	view := mustGet(t, log, "attempt-verify")
	if view.Verification != atoha.VerificationVerified || view.Outcome.Effect() != atoha.EffectObservedSatisfied {
		t.Fatal("verification facts were not projected")
	}
}

func TestVerifyRejectsChangedActionBeforeObservation(t *testing.T) {
	log := memory.New()
	action := mustAction(t, "action-verify-immutable")
	executor := mustExecutor(t, log, providerFunc(func(_ context.Context, attempt atoha.ExecutionAttempt, _ atoha.Action, _ atoha.ResolvedTarget) (atoha.DispatchResult, error) {
		return atoha.NewDispatchResult(atoha.DispatchResultSpec{Provider: "fixture", AttemptID: attempt.ID(), Status: atoha.DispatchDispatched})
	}))
	if _, err := executor.Execute(context.Background(), ExecuteInput{Action: action, AttemptID: "attempt-verify-immutable"}); err != nil {
		t.Fatal(err)
	}
	changed, _ := atoha.NewAction(atoha.ActionSpec{
		ID: action.ID(), Target: "bedroom-light", Name: action.Name(), Input: action.Input(), RequestedAt: action.RequestedAt(),
	})
	var observations atomic.Int32
	verify, err := NewVerifyAttempt(VerifyConfig{
		Resolver: resolverFunc(func(context.Context, atoha.Action) (atoha.ResolvedTarget, error) { return fixtureTarget() }),
		Observer: observerFunc(func(context.Context, atoha.Action, atoha.ResolvedTarget) ([]atoha.Observation, error) {
			observations.Add(1)
			return nil, nil
		}),
		Verifier: verifierFunc(func(context.Context, atoha.Action, []atoha.Observation, time.Time) (atoha.VerificationResult, error) {
			return atoha.VerificationResult{}, nil
		}),
		Journal: log, Reader: log, Actions: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verify.Verify(context.Background(), VerifyInput{Action: changed, AttemptID: "attempt-verify-immutable"}); !errors.Is(err, atoha.ErrActionIdentityConflict) {
		t.Fatalf("Verify() error = %v; want action identity conflict", err)
	}
	if observations.Load() != 0 {
		t.Fatal("changed action reached observation I/O")
	}
}

func TestRecoveryRequiresAuthorizationAndLinksPriorAttempt(t *testing.T) {
	log := memory.New()
	action := mustAction(t, "action-recovery")
	executor := mustExecutor(t, log, providerFunc(func(_ context.Context, attempt atoha.ExecutionAttempt, _ atoha.Action, _ atoha.ResolvedTarget) (atoha.DispatchResult, error) {
		return atoha.NewDispatchResult(atoha.DispatchResultSpec{Provider: "fixture", AttemptID: attempt.ID(), Status: atoha.DispatchUnknown})
	}))
	if _, err := executor.Execute(context.Background(), ExecuteInput{Action: action, AttemptID: "attempt-original"}); err != nil {
		t.Fatal(err)
	}
	recoverUseCase, _ := NewRecoverAttempt(executor, log)
	if _, err := recoverUseCase.Recover(context.Background(), RecoverInput{Action: action, AttemptID: "attempt-recovery"}); !errors.Is(err, ErrRecoveryNotAuthorized) {
		t.Fatalf("Recover() error = %v; want authorization error", err)
	}
	authorization, err := NewRecoveryAuthorization(RecoveryAuthorizationSpec{
		ActionID: action.ID(), PreviousAttemptID: "attempt-original", AuthorizedBy: "operator@example.com",
		AuthorizedAt: testNow.Add(time.Minute), Reason: "operator verified that retry is safe",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := recoverUseCase.Recover(context.Background(), RecoverInput{
		Action: action, AttemptID: "attempt-recovery", Authorization: authorization,
	})
	if err != nil {
		t.Fatal(err)
	}
	if prior, ok := result.Attempt.RecoveryOf(); !ok || prior != "attempt-original" {
		t.Fatalf("RecoveryOf() = %q/%v", prior, ok)
	}
	events, _ := log.EventsByAttempt(context.Background(), "attempt-recovery")
	found := false
	for _, event := range events {
		found = found || event.Kind() == atoha.EventRecoveryAuthorized
	}
	if !found {
		t.Fatal("recovery authorization fact was not recorded")
	}
}

func TestScanIncompleteAttemptsNeverRecoversAutomatically(t *testing.T) {
	log := memory.New()
	action := mustAction(t, "action-incomplete")
	attempt, _ := atoha.NewExecutionAttempt(atoha.ExecutionAttemptSpec{ID: "attempt-incomplete", ActionID: action.ID(), StartedAt: testNow})
	requested, _ := actionRequestedEvent(action, attempt.ID(), testNow)
	started, _ := attemptStartedEvent(action, attempt, testNow)
	claimed, err := log.BeginAttempt(context.Background(), action, attempt, requested, started)
	if err != nil || !claimed {
		t.Fatal(err)
	}
	scanner, _ := NewScanIncompleteAttempts(log, log)
	items, err := scanner.Scan(context.Background())
	if err != nil || len(items) != 1 || items[0].AttemptID != attempt.ID() {
		t.Fatalf("Scan() = %#v, %v", items, err)
	}
	if len(log.Events()) != 2 {
		t.Fatal("scanner mutated or recovered the incomplete attempt")
	}
}

func mustExecutor(t *testing.T, journal Journal, provider atoha.Provider) *ExecuteAction {
	t.Helper()
	executor, err := NewExecuteAction(ExecuteConfig{
		Admitter: admitterFunc(func(context.Context, atoha.Action) (atoha.AdmissionStatus, error) {
			return atoha.AdmissionAdmitted, nil
		}),
		Resolver: resolverFunc(func(context.Context, atoha.Action) (atoha.ResolvedTarget, error) { return fixtureTarget() }),
		Provider: provider, Journal: journal, Now: func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func mustAction(t *testing.T, id atoha.ActionID) atoha.Action {
	t.Helper()
	action, err := atoha.NewAction(atoha.ActionSpec{
		ID: id, Target: "living-room-light", Name: "turn_on", Input: []byte(`{}`), RequestedAt: testNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func fixtureTarget() (atoha.ResolvedTarget, error) {
	return atoha.NewResolvedTarget("fixture", []byte("light.living_room"))
}

func mustGet(t *testing.T, reader AttemptEventReader, id atoha.AttemptID) AttemptView {
	t.Helper()
	query, _ := NewGetAttempt(reader)
	view, err := query.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

type admitterFunc func(context.Context, atoha.Action) (atoha.AdmissionStatus, error)

func (f admitterFunc) Admit(ctx context.Context, action atoha.Action) (atoha.AdmissionStatus, error) {
	return f(ctx, action)
}

type resolverFunc func(context.Context, atoha.Action) (atoha.ResolvedTarget, error)

func (f resolverFunc) Resolve(ctx context.Context, action atoha.Action) (atoha.ResolvedTarget, error) {
	return f(ctx, action)
}

type providerFunc func(context.Context, atoha.ExecutionAttempt, atoha.Action, atoha.ResolvedTarget) (atoha.DispatchResult, error)

func (f providerFunc) Dispatch(ctx context.Context, attempt atoha.ExecutionAttempt, action atoha.Action, target atoha.ResolvedTarget) (atoha.DispatchResult, error) {
	return f(ctx, attempt, action, target)
}

type observerFunc func(context.Context, atoha.Action, atoha.ResolvedTarget) ([]atoha.Observation, error)

func (f observerFunc) Observe(ctx context.Context, action atoha.Action, target atoha.ResolvedTarget) ([]atoha.Observation, error) {
	return f(ctx, action, target)
}

type verifierFunc func(context.Context, atoha.Action, []atoha.Observation, time.Time) (atoha.VerificationResult, error)

func (f verifierFunc) Verify(ctx context.Context, action atoha.Action, observations []atoha.Observation, at time.Time) (atoha.VerificationResult, error) {
	return f(ctx, action, observations, at)
}

type failAppendJournal struct {
	*memory.Log
	appendCalls int
	failAt      int
	err         error
}

func (j *failAppendJournal) AppendAll(ctx context.Context, events ...atoha.ExecutionEvent) error {
	j.appendCalls++
	if j.appendCalls == j.failAt {
		return j.err
	}
	return j.Log.AppendAll(ctx, events...)
}

func (j *failAppendJournal) String() string { return fmt.Sprintf("fail at %d", j.failAt) }
