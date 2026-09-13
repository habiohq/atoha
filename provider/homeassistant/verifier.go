package homeassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/habiohq/atoha"
)

const stateVerifierID = "homeassistant/state-v1"

// StateVerifier verifies the three proof-of-concept action shapes against a
// Home Assistant state response. It is an edge implementation, not a canonical
// Atoha device model.
type StateVerifier struct {
	maxAge time.Duration
}

// NewStateVerifier creates a verifier with an explicit positive freshness
// window.
func NewStateVerifier(maxAge time.Duration) (StateVerifier, error) {
	if maxAge <= 0 {
		return StateVerifier{}, errors.New("homeassistant: verifier max age must be positive")
	}
	return StateVerifier{maxAge: maxAge}, nil
}

// Verify evaluates the freshest matching observation. Supported PoC actions
// are turn_on, turn_off, and set_temperature.
func (v StateVerifier) Verify(_ context.Context, action atoha.Action, observations []atoha.Observation, asOf time.Time) (atoha.VerificationResult, error) {
	var selected *atoha.Observation
	for i := range observations {
		if observations[i].Target() != action.Target() {
			continue
		}
		if selected == nil || observations[i].ObservedAt().After(selected.ObservedAt()) {
			selected = &observations[i]
		}
	}
	if selected == nil {
		return v.result(atoha.VerificationInconclusive, asOf, nil, "no matching observation")
	}
	age := asOf.Sub(selected.ObservedAt())
	if age < 0 {
		return v.result(atoha.VerificationInconclusive, asOf, selected, "observation is in the future")
	}
	if age > v.maxAge {
		return v.result(atoha.VerificationInconclusive, asOf, selected, "observation is stale")
	}

	var state struct {
		State      string                     `json:"state"`
		Attributes map[string]json.RawMessage `json:"attributes"`
	}
	if err := json.Unmarshal(selected.Value(), &state); err != nil {
		result, resultErr := v.result(atoha.VerificationInconclusive, asOf, selected, "malformed Home Assistant state")
		if resultErr != nil {
			return atoha.VerificationResult{}, resultErr
		}
		return result, fmt.Errorf("homeassistant: decode observation: %w", err)
	}

	switch action.Name() {
	case "turn_on":
		return v.compareState(state.State, "on", asOf, selected)
	case "turn_off":
		return v.compareState(state.State, "off", asOf, selected)
	case "set_temperature":
		return v.compareTemperature(action.Input(), state.Attributes["temperature"], asOf, selected)
	default:
		return v.result(atoha.VerificationInconclusive, asOf, selected, "unsupported proof-of-concept action")
	}
}

func (v StateVerifier) compareState(got, want string, asOf time.Time, observation *atoha.Observation) (atoha.VerificationResult, error) {
	if got == want {
		return v.result(atoha.VerificationVerified, asOf, observation, "fresh state matches desired effect")
	}
	return v.result(atoha.VerificationUnsatisfied, asOf, observation, "fresh state contradicts desired effect")
}

func (v StateVerifier) compareTemperature(input []byte, observed json.RawMessage, asOf time.Time, observation *atoha.Observation) (atoha.VerificationResult, error) {
	var desired struct {
		Temperature json.Number `json:"temperature"`
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	if err := decoder.Decode(&desired); err != nil {
		return atoha.VerificationResult{}, fmt.Errorf("homeassistant: decode desired temperature: %w", err)
	}
	if desired.Temperature == "" {
		return atoha.VerificationResult{}, errors.New("homeassistant: desired temperature is required")
	}
	var actual json.Number
	decoder = json.NewDecoder(bytes.NewReader(observed))
	decoder.UseNumber()
	if err := decoder.Decode(&actual); err != nil || actual == "" {
		result, resultErr := v.result(atoha.VerificationInconclusive, asOf, observation, "temperature attribute unavailable")
		if resultErr != nil {
			return atoha.VerificationResult{}, resultErr
		}
		return result, err
	}
	if equalNumber(desired.Temperature, actual) {
		return v.result(atoha.VerificationVerified, asOf, observation, "fresh setpoint matches desired value")
	}
	return v.result(atoha.VerificationUnsatisfied, asOf, observation, "fresh setpoint contradicts desired value")
}

func (v StateVerifier) result(status atoha.VerificationStatus, asOf time.Time, observation *atoha.Observation, reason string) (atoha.VerificationResult, error) {
	var ids []atoha.ObservationID
	if observation != nil {
		ids = []atoha.ObservationID{observation.ID()}
	}
	return atoha.NewVerificationResult(atoha.VerificationResultSpec{
		Status: status, Verifier: stateVerifierID, CheckedAt: asOf, ObservationIDs: ids, Reason: reason,
	})
}

func equalNumber(a, b json.Number) bool {
	left, ok := new(big.Rat).SetString(string(a))
	if !ok {
		return false
	}
	right, ok := new(big.Rat).SetString(string(b))
	return ok && left.Cmp(right) == 0
}

var _ atoha.Verifier = StateVerifier{}
