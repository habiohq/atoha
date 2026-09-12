package habio

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// VerificationStatus reports a Verifier's assessment of available evidence.
type VerificationStatus uint8

const (
	// VerificationUnknown is the valid zero value and means that no assessment
	// has been recorded.
	VerificationUnknown VerificationStatus = iota
	// VerificationVerified means the named Verifier found sufficient evidence
	// that the desired effect is satisfied.
	VerificationVerified
	// VerificationUnsatisfied means sufficient evidence contradicts the desired
	// effect.
	VerificationUnsatisfied
	// VerificationInconclusive means the Verifier ran but evidence was missing,
	// stale, contradictory, or otherwise insufficient.
	VerificationInconclusive
)

var ErrInvalidVerification = errors.New("habio: invalid verification")

// VerificationResultSpec contains one named Verifier's immutable assessment.
type VerificationResultSpec struct {
	Status         VerificationStatus
	Verifier       string
	CheckedAt      time.Time
	ObservationIDs []ObservationID
	Reason         string
}

// VerificationResult records an assessment and the observations it used.
// Its zero value is valid and means that verification has not been recorded.
type VerificationResult struct {
	status         VerificationStatus
	verifier       string
	checkedAt      time.Time
	observationIDs []ObservationID
	reason         string
}

// NewVerificationResult validates spec. A non-unknown result names the
// Verifier and evaluation time. Verified and unsatisfied results require at
// least one ObservationID; an inconclusive result may be caused by missing
// evidence and therefore have none.
func NewVerificationResult(spec VerificationResultSpec) (VerificationResult, error) {
	if !spec.Status.valid() {
		return VerificationResult{}, fmt.Errorf("%w: status %d", ErrInvalidVerification, spec.Status)
	}
	if spec.Status == VerificationUnknown {
		if spec.Verifier != "" || !spec.CheckedAt.IsZero() || len(spec.ObservationIDs) != 0 || spec.Reason != "" {
			return VerificationResult{}, fmt.Errorf("%w: unknown result cannot claim an assessment", ErrInvalidVerification)
		}
		return VerificationResult{}, nil
	}
	if err := validateText("verifier", spec.Verifier); err != nil {
		return VerificationResult{}, fmt.Errorf("%w: %v", ErrInvalidVerification, err)
	}
	if spec.CheckedAt.IsZero() {
		return VerificationResult{}, fmt.Errorf("%w: checked time is required", ErrInvalidVerification)
	}
	if (spec.Status == VerificationVerified || spec.Status == VerificationUnsatisfied) && len(spec.ObservationIDs) == 0 {
		return VerificationResult{}, fmt.Errorf("%w: %s result requires observation evidence", ErrInvalidVerification, spec.Status)
	}
	for _, id := range spec.ObservationIDs {
		if err := validateIdentity("observation ID", string(id)); err != nil {
			return VerificationResult{}, fmt.Errorf("%w: %v", ErrInvalidVerification, err)
		}
	}

	return VerificationResult{
		status:         spec.Status,
		verifier:       spec.Verifier,
		checkedAt:      spec.CheckedAt,
		observationIDs: cloneObservationIDs(spec.ObservationIDs),
		reason:         spec.Reason,
	}, nil
}

// Status returns the Verifier's assessment.
func (r VerificationResult) Status() VerificationStatus { return r.status }

// Verifier returns the identity of the policy or implementation that assessed
// the observations.
func (r VerificationResult) Verifier() string { return r.verifier }

// CheckedAt returns the explicit time at which freshness and other evidence
// rules were evaluated.
func (r VerificationResult) CheckedAt() time.Time { return r.checkedAt }

// ObservationIDs returns a copy of the evidence identities used.
func (r VerificationResult) ObservationIDs() []ObservationID {
	return cloneObservationIDs(r.observationIDs)
}

// Reason returns a human-readable diagnostic. Callers must use Status, not
// parse Reason, for control flow.
func (r VerificationResult) Reason() string { return r.reason }

// Verifier assesses whether observations establish an Action's desired effect.
// asOf makes freshness evaluation deterministic. Concrete evidence policy lives
// in extension packages, not in the Habio core.
type Verifier interface {
	Verify(ctx context.Context, action Action, observations []Observation, asOf time.Time) (VerificationResult, error)
}

func (s VerificationStatus) valid() bool { return s <= VerificationInconclusive }

func (s VerificationStatus) String() string {
	switch s {
	case VerificationUnknown:
		return "unknown"
	case VerificationVerified:
		return "verified"
	case VerificationUnsatisfied:
		return "unsatisfied"
	case VerificationInconclusive:
		return "inconclusive"
	default:
		return fmt.Sprintf("VerificationStatus(%d)", s)
	}
}

func cloneObservationIDs(ids []ObservationID) []ObservationID {
	if ids == nil {
		return nil
	}
	return append([]ObservationID(nil), ids...)
}
