package atoha

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProviderContractAcknowledgementRetainsReceiptAndError(t *testing.T) {
	action, attempt, target := providerFixtures(t)
	provider := fixtureProvider{dispatch: func(_ context.Context, attempt ExecutionAttempt, _ Action, _ ResolvedTarget) (DispatchResult, error) {
		receipt, err := NewReceipt(ReceiptSpec{
			Provider: "fixture", AttemptID: attempt.ID(), ReceivedAt: time.Now(),
			Evidence: []byte(`{"accepted":true}`),
		})
		if err != nil {
			return DispatchResult{}, err
		}
		result, err := NewDispatchResult(DispatchResultSpec{
			Provider: "fixture", AttemptID: attempt.ID(), Status: DispatchAcknowledged, Receipt: &receipt,
		})
		if err != nil {
			return DispatchResult{}, err
		}
		return result, errors.New("fixture: response trailer could not be decoded")
	}}

	result, err := provider.Dispatch(context.Background(), attempt, action, target)
	if err == nil {
		t.Fatal("Dispatch() error = nil; want independent software-path error")
	}
	if result.Status() != DispatchAcknowledged {
		t.Fatalf("Status() = %v; want acknowledged", result.Status())
	}
	receipt, ok := result.Receipt()
	if !ok || string(receipt.Evidence()) != `{"accepted":true}` {
		t.Fatalf("Receipt() = %+v, %v; want retained acknowledgement", receipt, ok)
	}
}

func TestProviderContractLostResponseRemainsUnknown(t *testing.T) {
	action, attempt, target := providerFixtures(t)
	provider := fixtureProvider{dispatch: func(_ context.Context, attempt ExecutionAttempt, _ Action, _ ResolvedTarget) (DispatchResult, error) {
		result, err := NewDispatchResult(DispatchResultSpec{
			Provider: "fixture", AttemptID: attempt.ID(), Status: DispatchUnknown,
		})
		if err != nil {
			return DispatchResult{}, err
		}
		return result, context.DeadlineExceeded
	}}

	result, err := provider.Dispatch(context.Background(), attempt, action, target)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Dispatch() error = %v; want deadline exceeded", err)
	}
	if result.Status() != DispatchUnknown {
		t.Fatalf("Status() = %v; timeout must remain unknown", result.Status())
	}
	if _, ok := result.Receipt(); ok {
		t.Fatal("lost response unexpectedly produced a receipt")
	}
}

func TestProviderContractLateAcknowledgementSurvivesCancellation(t *testing.T) {
	action, attempt, target := providerFixtures(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := fixtureProvider{dispatch: func(ctx context.Context, attempt ExecutionAttempt, _ Action, _ ResolvedTarget) (DispatchResult, error) {
		receipt, err := NewReceipt(ReceiptSpec{
			Provider: "fixture", AttemptID: attempt.ID(), ReceivedAt: time.Now(),
			Evidence: []byte("late-acknowledgement"),
		})
		if err != nil {
			return DispatchResult{}, err
		}
		result, err := NewDispatchResult(DispatchResultSpec{
			Provider: "fixture", AttemptID: attempt.ID(), Status: DispatchAcknowledged, Receipt: &receipt,
		})
		if err != nil {
			return DispatchResult{}, err
		}
		return result, ctx.Err()
	}}

	result, err := provider.Dispatch(ctx, attempt, action, target)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Dispatch() error = %v; want context canceled", err)
	}
	receipt, ok := result.Receipt()
	if result.Status() != DispatchAcknowledged || !ok || string(receipt.Evidence()) != "late-acknowledgement" {
		t.Fatalf("late acknowledgement was lost: result=%+v receipt=%+v ok=%v", result, receipt, ok)
	}
}

func TestProviderOptionalCapabilitiesAreSeparate(t *testing.T) {
	var provider Provider = fixtureProvider{}
	if _, ok := provider.(Observer); ok {
		t.Fatal("minimum Provider unexpectedly implements Observer")
	}
	if _, ok := provider.(Resolver); ok {
		t.Fatal("minimum Provider unexpectedly implements Resolver")
	}
	if _, ok := provider.(Admitter); ok {
		t.Fatal("minimum Provider unexpectedly implements Admitter")
	}
}

func TestResolvedTargetAndReceiptAreImmutable(t *testing.T) {
	endpoint := []byte(`{"entity_id":"light.living_room"}`)
	target, err := NewResolvedTarget("fixture", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	endpoint[0] = 'X'
	got := target.Endpoint()
	got[0] = 'Y'
	if string(target.Endpoint()) != `{"entity_id":"light.living_room"}` {
		t.Fatal("ResolvedTarget did not retain an immutable endpoint")
	}
}

func TestNewDispatchResultRejectsInvalidReceiptRelationships(t *testing.T) {
	now := time.Now()
	receipt, err := NewReceipt(ReceiptSpec{Provider: "fixture", AttemptID: "attempt-1", ReceivedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	tests := []DispatchResultSpec{
		{Provider: "fixture", AttemptID: "attempt-1", Status: DispatchAcknowledged},
		{Provider: "fixture", AttemptID: "attempt-1", Status: DispatchDispatched, Receipt: &receipt},
		{Provider: "other", AttemptID: "attempt-1", Status: DispatchAcknowledged, Receipt: &receipt},
		{Provider: "fixture", AttemptID: "attempt-2", Status: DispatchAcknowledged, Receipt: &receipt},
	}
	for _, spec := range tests {
		if _, err := NewDispatchResult(spec); !errors.Is(err, ErrInvalidDispatchResult) {
			t.Errorf("NewDispatchResult(%+v) error = %v; want ErrInvalidDispatchResult", spec, err)
		}
	}
}

type fixtureProvider struct {
	dispatch func(context.Context, ExecutionAttempt, Action, ResolvedTarget) (DispatchResult, error)
}

func (p fixtureProvider) Dispatch(ctx context.Context, attempt ExecutionAttempt, action Action, target ResolvedTarget) (DispatchResult, error) {
	if p.dispatch == nil {
		return DispatchResult{}, errors.New("fixture: dispatch is not configured")
	}
	return p.dispatch(ctx, attempt, action, target)
}

func providerFixtures(t *testing.T) (Action, ExecutionAttempt, ResolvedTarget) {
	t.Helper()
	now := time.Now()
	action, err := NewAction(ActionSpec{
		ID: "action-1", Target: "living-room-light", Name: "turn_on", RequestedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := NewExecutionAttempt(ExecutionAttemptSpec{
		ID: "attempt-1", ActionID: action.ID(), StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewResolvedTarget("fixture", []byte("fixture-endpoint"))
	if err != nil {
		t.Fatal(err)
	}
	return action, attempt, target
}
