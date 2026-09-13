package atoha

import (
	"errors"
	"fmt"
	"time"
)

// ObservationID identifies one immutable piece of observed evidence.
type ObservationID string

var ErrInvalidObservation = errors.New("atoha: invalid observation")

// ObservationSpec contains evidence obtained from a Provider or another
// physical-world source. Value and Evidence are opaque to the core.
type ObservationSpec struct {
	ID         ObservationID
	Source     string
	Target     string
	Value      []byte
	Evidence   []byte
	ObservedAt time.Time
	RecordedAt time.Time
}

// Observation is immutable evidence, not absolute truth.
//
// ObservedAt is the source's time for the observation. RecordedAt is when the
// observation entered Atoha. Keeping both lets a Verifier apply an explicit
// freshness policy without trusting ingestion time as physical freshness.
// The zero value is invalid.
type Observation struct {
	id         ObservationID
	source     string
	target     string
	value      []byte
	evidence   []byte
	observedAt time.Time
	recordedAt time.Time
}

// NewObservation validates spec and copies mutable byte slices.
func NewObservation(spec ObservationSpec) (Observation, error) {
	if err := validateIdentity("observation ID", string(spec.ID)); err != nil {
		return Observation{}, fmt.Errorf("%w: %v", ErrInvalidObservation, err)
	}
	if err := validateText("source", spec.Source); err != nil {
		return Observation{}, fmt.Errorf("%w: %v", ErrInvalidObservation, err)
	}
	if err := validateText("target", spec.Target); err != nil {
		return Observation{}, fmt.Errorf("%w: %v", ErrInvalidObservation, err)
	}
	if spec.ObservedAt.IsZero() {
		return Observation{}, fmt.Errorf("%w: observed time is required", ErrInvalidObservation)
	}
	if spec.RecordedAt.IsZero() {
		return Observation{}, fmt.Errorf("%w: recorded time is required", ErrInvalidObservation)
	}

	return Observation{
		id:         spec.ID,
		source:     spec.Source,
		target:     spec.Target,
		value:      cloneBytes(spec.Value),
		evidence:   cloneBytes(spec.Evidence),
		observedAt: spec.ObservedAt,
		recordedAt: spec.RecordedAt,
	}, nil
}

// ID returns the observation identity.
func (o Observation) ID() ObservationID { return o.id }

// Source returns the identifier of the system that produced the observation.
func (o Observation) Source() string { return o.source }

// Target returns the logical target described by the observation.
func (o Observation) Target() string { return o.target }

// Value returns a copy of the opaque observed value.
func (o Observation) Value() []byte { return cloneBytes(o.value) }

// Evidence returns a copy of source-specific evidence retained for audit or a
// domain Verifier.
func (o Observation) Evidence() []byte { return cloneBytes(o.evidence) }

// ObservedAt returns the source's time for the observation.
func (o Observation) ObservedAt() time.Time { return o.observedAt }

// RecordedAt returns when the observation entered Atoha.
func (o Observation) RecordedAt() time.Time { return o.recordedAt }
