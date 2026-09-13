package atoha

import (
	"errors"
	"testing"
	"time"
)

func TestObservationIsImmutable(t *testing.T) {
	now := time.Date(2026, time.September, 6, 10, 0, 0, 0, time.UTC)
	value := []byte(`{"setpoint":24}`)
	evidence := []byte(`{"entity_id":"climate.living_room"}`)

	observation, err := NewObservation(ObservationSpec{
		ID:         "observation-1",
		Source:     "home-assistant",
		Target:     "living-room-climate",
		Value:      value,
		Evidence:   evidence,
		ObservedAt: now.Add(-time.Second),
		RecordedAt: now,
	})
	if err != nil {
		t.Fatalf("NewObservation() error = %v", err)
	}

	value[0] = 'X'
	evidence[0] = 'Y'
	gotValue := observation.Value()
	gotEvidence := observation.Evidence()
	gotValue[0] = 'Z'
	gotEvidence[0] = 'Z'

	if got := string(observation.Value()); got != `{"setpoint":24}` {
		t.Fatalf("Value() = %q; want immutable copy", got)
	}
	if got := string(observation.Evidence()); got != `{"entity_id":"climate.living_room"}` {
		t.Fatalf("Evidence() = %q; want immutable copy", got)
	}
	if observation.ID() != "observation-1" || observation.Source() != "home-assistant" || observation.Target() != "living-room-climate" {
		t.Fatalf("Observation accessors returned unexpected values: %+v", observation)
	}
}

func TestNewObservationRejectsInvalidSpecs(t *testing.T) {
	now := time.Now()
	tests := []ObservationSpec{
		{Source: "source", Target: "target", ObservedAt: now, RecordedAt: now},
		{ID: "observation-1", Target: "target", ObservedAt: now, RecordedAt: now},
		{ID: "observation-1", Source: "source", ObservedAt: now, RecordedAt: now},
		{ID: "observation-1", Source: "source", Target: "target", RecordedAt: now},
		{ID: "observation-1", Source: "source", Target: "target", ObservedAt: now},
	}
	for _, spec := range tests {
		if _, err := NewObservation(spec); !errors.Is(err, ErrInvalidObservation) {
			t.Errorf("NewObservation(%+v) error = %v; want ErrInvalidObservation", spec, err)
		}
	}
}

func TestObservationPreservesSourceClockSkew(t *testing.T) {
	now := time.Now()
	observation, err := NewObservation(ObservationSpec{
		ID:         "observation-1",
		Source:     "external-sensor",
		Target:     "target",
		ObservedAt: now.Add(time.Second),
		RecordedAt: now,
	})
	if err != nil {
		t.Fatalf("NewObservation() error = %v", err)
	}
	if !observation.ObservedAt().After(observation.RecordedAt()) {
		t.Fatal("constructor should preserve clock skew as evidence")
	}
}
