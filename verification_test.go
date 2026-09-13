package atoha

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestVerifierEvidenceCases(t *testing.T) {
	now := time.Date(2026, time.September, 6, 10, 0, 0, 0, time.UTC)
	action, err := NewAction(ActionSpec{
		ID: "action-1", Target: "living-room-climate", Name: "set_temperature",
		Input: []byte("24"), RequestedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier := exactValueVerifier{ID: "fixture/exact-value-v1", MaxAge: 30 * time.Second}

	tests := []struct {
		name         string
		observations []Observation
		want         VerificationStatus
	}{
		{name: "missing", want: VerificationInconclusive},
		{name: "fresh match", observations: []Observation{mustObservation(t, "fresh", "24", now.Add(-time.Second), now)}, want: VerificationVerified},
		{name: "stale match", observations: []Observation{mustObservation(t, "stale", "24", now.Add(-time.Minute), now)}, want: VerificationInconclusive},
		{name: "fresh contradiction", observations: []Observation{mustObservation(t, "contradiction", "28", now.Add(-time.Second), now)}, want: VerificationUnsatisfied},
		{name: "future clock skew", observations: []Observation{mustObservation(t, "future", "24", now.Add(time.Second), now)}, want: VerificationInconclusive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := verifier.Verify(context.Background(), action, tt.observations, now)
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if result.Status() != tt.want {
				t.Fatalf("Status() = %v; want %v (reason: %s)", result.Status(), tt.want, result.Reason())
			}
		})
	}
}

func TestVerificationResultIsImmutable(t *testing.T) {
	now := time.Now()
	ids := []ObservationID{"observation-1"}
	result, err := NewVerificationResult(VerificationResultSpec{
		Status: VerificationVerified, Verifier: "verifier-v1", CheckedAt: now,
		ObservationIDs: ids, Reason: "fresh matching state",
	})
	if err != nil {
		t.Fatalf("NewVerificationResult() error = %v", err)
	}
	ids[0] = "changed"
	got := result.ObservationIDs()
	got[0] = "changed-again"
	if result.ObservationIDs()[0] != "observation-1" {
		t.Fatal("ObservationIDs() did not return an immutable copy")
	}
}

func TestNewVerificationResultRejectsInvalidSpecs(t *testing.T) {
	now := time.Now()
	tests := []VerificationResultSpec{
		{Status: VerificationStatus(255)},
		{Status: VerificationUnknown, Verifier: "claimed-verifier"},
		{Status: VerificationVerified, CheckedAt: now},
		{Status: VerificationVerified, Verifier: "verifier-v1"},
		{Status: VerificationVerified, Verifier: "verifier-v1", CheckedAt: now},
		{Status: VerificationUnsatisfied, Verifier: "verifier-v1", CheckedAt: now},
		{Status: VerificationVerified, Verifier: "verifier-v1", CheckedAt: now, ObservationIDs: []ObservationID{""}},
	}
	for _, spec := range tests {
		if _, err := NewVerificationResult(spec); !errors.Is(err, ErrInvalidVerification) {
			t.Errorf("NewVerificationResult(%+v) error = %v; want ErrInvalidVerification", spec, err)
		}
	}
}

type exactValueVerifier struct {
	ID     string
	MaxAge time.Duration
}

func (v exactValueVerifier) Verify(_ context.Context, action Action, observations []Observation, asOf time.Time) (VerificationResult, error) {
	if len(observations) == 0 {
		return NewVerificationResult(VerificationResultSpec{
			Status: VerificationInconclusive, Verifier: v.ID, CheckedAt: asOf,
			Reason: "no observation",
		})
	}
	observation := observations[len(observations)-1]
	age := asOf.Sub(observation.ObservedAt())
	if age < 0 || age > v.MaxAge {
		return NewVerificationResult(VerificationResultSpec{
			Status: VerificationInconclusive, Verifier: v.ID, CheckedAt: asOf,
			ObservationIDs: []ObservationID{observation.ID()}, Reason: "observation outside freshness window",
		})
	}
	status := VerificationUnsatisfied
	if string(observation.Value()) == string(action.Input()) {
		status = VerificationVerified
	}
	return NewVerificationResult(VerificationResultSpec{
		Status: status, Verifier: v.ID, CheckedAt: asOf,
		ObservationIDs: []ObservationID{observation.ID()}, Reason: "exact fixture comparison",
	})
}

func mustObservation(t *testing.T, id, value string, observedAt, recordedAt time.Time) Observation {
	t.Helper()
	observation, err := NewObservation(ObservationSpec{
		ID: ObservationID(id), Source: "fixture-sensor", Target: "living-room-climate",
		Value: []byte(value), ObservedAt: observedAt, RecordedAt: recordedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}
