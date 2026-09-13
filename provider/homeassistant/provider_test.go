package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/habiohq/atoha"
)

func TestDispatchLightClimateAndMediaPlayerActions(t *testing.T) {
	var requests atomic.Int32
	var paths []string
	var bodies []map[string]any
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		bodies = append(bodies, body)
		return response(http.StatusOK, `[]`), nil
	})}

	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	provider := mustProvider(t, Config{
		BaseURL: "http://home-assistant.local:8123", Token: "test-token", Client: client, Now: func() time.Time { return now },
		Bindings: map[string]string{
			"living-room-light":   "light.living_room",
			"living-room-climate": "climate.living_room",
			"living-room-tv":      "media_player.living_room_tv",
		},
	})
	tests := []struct {
		target, name, input, path, entityID string
		wantTemperature                     bool
	}{
		{"living-room-light", "turn_on", `{}`, "/api/services/light/turn_on", "light.living_room", false},
		{"living-room-climate", "set_temperature", `{"temperature":24}`, "/api/services/climate/set_temperature", "climate.living_room", true},
		{"living-room-tv", "turn_off", `{}`, "/api/services/media_player/turn_off", "media_player.living_room_tv", false},
	}

	for i, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			action, attempt := actionAttempt(t, i, tt.target, tt.name, tt.input, now)
			target, err := provider.Resolve(context.Background(), action)
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Dispatch(context.Background(), attempt, action, target)
			if err != nil {
				t.Fatalf("Dispatch() error = %v", err)
			}
			if result.Status() != atoha.DispatchAcknowledged {
				t.Fatalf("Status() = %v; want acknowledged", result.Status())
			}
			if _, ok := result.Receipt(); !ok {
				t.Fatal("acknowledged dispatch has no receipt")
			}
		})
	}
	if requests.Load() != 3 {
		t.Fatalf("request count = %d; want exactly 3", requests.Load())
	}
	for i, tt := range tests {
		if paths[i] != tt.path {
			t.Errorf("request %d path = %q; want %q", i, paths[i], tt.path)
		}
		entityID, ok := bodies[i]["entity_id"].(string)
		if !ok || entityID != tt.entityID {
			t.Errorf("request %d entity_id = %#v; want %q", i, bodies[i]["entity_id"], tt.entityID)
		}
		if tt.wantTemperature && bodies[i]["temperature"] != float64(24) {
			t.Errorf("request %d temperature = %#v; want 24", i, bodies[i]["temperature"])
		}
	}
}

func TestDispatchAndObserveRejectMismatchedIdentityBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return response(http.StatusOK, `[]`), nil
	})}
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	provider := mustProvider(t, Config{
		BaseURL: "http://home-assistant.local:8123", Token: "test-token", Client: client,
		Bindings: map[string]string{
			"living-room-light": "light.living_room",
			"bedroom-light":     "light.bedroom",
		},
	})
	livingAction, livingAttempt := actionAttempt(t, 1, "living-room-light", "turn_off", `{}`, now)
	bedroomAction, bedroomAttempt := actionAttempt(t, 2, "bedroom-light", "turn_off", `{}`, now)
	livingTarget, err := provider.Resolve(context.Background(), livingAction)
	if err != nil {
		t.Fatal(err)
	}
	bedroomTarget, err := provider.Resolve(context.Background(), bedroomAction)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("attempt belongs to another action", func(t *testing.T) {
		result, err := provider.Dispatch(context.Background(), bedroomAttempt, livingAction, livingTarget)
		if !errors.Is(err, ErrAttemptMismatch) {
			t.Fatalf("Dispatch() error = %v; want ErrAttemptMismatch", err)
		}
		if result.Status() != atoha.DispatchNotDispatched {
			t.Fatalf("Status() = %v; want not dispatched", result.Status())
		}
	})

	t.Run("target belongs to another action", func(t *testing.T) {
		result, err := provider.Dispatch(context.Background(), livingAttempt, livingAction, bedroomTarget)
		if !errors.Is(err, ErrTargetMismatch) {
			t.Fatalf("Dispatch() error = %v; want ErrTargetMismatch", err)
		}
		if result.Status() != atoha.DispatchNotDispatched {
			t.Fatalf("Status() = %v; want not dispatched", result.Status())
		}
	})

	t.Run("observation target belongs to another action", func(t *testing.T) {
		if _, err := provider.Observe(context.Background(), livingAction, bedroomTarget); !errors.Is(err, ErrTargetMismatch) {
			t.Fatalf("Observe() error = %v; want ErrTargetMismatch", err)
		}
	})

	if calls.Load() != 0 {
		t.Fatalf("HTTP call count = %d; mismatches must be rejected before I/O", calls.Load())
	}
}

func TestDispatchRetainsAcknowledgementWhenResponseBodyCannotBeRetained(t *testing.T) {
	readErr := errors.New("fixture: response body read failed")
	tests := []struct {
		name            string
		body            io.ReadCloser
		wantErr         error
		wantEvidenceLen int
	}{
		{
			name: "read failure", body: io.NopCloser(io.MultiReader(strings.NewReader("partial"), errorReader{err: readErr})),
			wantErr: readErr, wantEvidenceLen: len("partial"),
		},
		{
			name: "too large", body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxResponseSize+1))),
			wantErr: ErrResponseTooLarge, wantEvidenceLen: maxResponseSize,
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK),
					Header: make(http.Header), Body: tt.body,
				}, nil
			})}
			now := time.Date(2026, time.September, 6, 12, 0, i, 0, time.UTC)
			provider := mustProvider(t, Config{
				BaseURL: "http://home-assistant.local:8123", Token: "test-token", Client: client,
				Now: func() time.Time { return now }, Bindings: map[string]string{"living-room-light": "light.living_room"},
			})
			action, attempt := actionAttempt(t, i, "living-room-light", "turn_on", `{}`, now)
			target, err := provider.Resolve(context.Background(), action)
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Dispatch(context.Background(), attempt, action, target)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Dispatch() error = %v; want %v", err, tt.wantErr)
			}
			if result.Status() != atoha.DispatchAcknowledged {
				t.Fatalf("Status() = %v; want acknowledged", result.Status())
			}
			receipt, ok := result.Receipt()
			if !ok {
				t.Fatal("acknowledgement evidence was lost")
			}
			if got := len(receipt.Evidence()); got != tt.wantEvidenceLen {
				t.Fatalf("Receipt evidence length = %d; want %d retained bytes", got, tt.wantEvidenceLen)
			}
		})
	}
}

func TestDispatchTimeoutIsUnknownAndNotRetried(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, context.DeadlineExceeded
	})}
	now := time.Now()
	provider := mustProvider(t, Config{
		BaseURL: "http://home-assistant.local:8123", Token: "test-token", Client: client,
		Bindings: map[string]string{"living-room-light": "light.living_room"},
	})
	action, attempt := actionAttempt(t, 1, "living-room-light", "turn_on", `{}`, now)
	target, err := provider.Resolve(context.Background(), action)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Dispatch(context.Background(), attempt, action, target)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Dispatch() error = %v; want deadline exceeded", err)
	}
	if result.Status() != atoha.DispatchUnknown {
		t.Fatalf("Status() = %v; timeout must be unknown", result.Status())
	}
	if calls.Load() != 1 {
		t.Fatalf("HTTP call count = %d; ambiguous action was retried", calls.Load())
	}
}

func TestObserveAndVerifyFreshState(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	client := stateClient(t, "light.living_room", "on", `{}`, now.Add(-time.Second))
	provider := mustProvider(t, Config{
		BaseURL: "http://home-assistant.local:8123", Token: "test-token", Client: client, Now: func() time.Time { return now },
		Bindings: map[string]string{"living-room-light": "light.living_room"},
	})
	action, _ := actionAttempt(t, 1, "living-room-light", "turn_on", `{}`, now.Add(-time.Minute))
	target, _ := provider.Resolve(context.Background(), action)
	observations, err := provider.Observe(context.Background(), action, target)
	if err != nil {
		t.Fatal(err)
	}
	verifier, _ := NewStateVerifier(30 * time.Second)
	result, err := verifier.Verify(context.Background(), action, observations, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status() != atoha.VerificationVerified {
		t.Fatalf("Status() = %v; want verified (%s)", result.Status(), result.Reason())
	}
}

func TestStateVerifierEvidenceCases(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	verifier, _ := NewStateVerifier(30 * time.Second)
	tests := []struct {
		name         string
		actionName   string
		input        string
		state        string
		attributes   string
		observedAt   time.Time
		source       string
		want         atoha.VerificationStatus
		observations bool
	}{
		{name: "missing", actionName: "turn_on", input: `{}`, want: atoha.VerificationInconclusive},
		{name: "stale", actionName: "turn_on", input: `{}`, state: "on", attributes: `{}`, observedAt: now.Add(-time.Minute), want: atoha.VerificationInconclusive, observations: true},
		{name: "contradictory", actionName: "turn_on", input: `{}`, state: "off", attributes: `{}`, observedAt: now.Add(-time.Second), want: atoha.VerificationUnsatisfied, observations: true},
		{name: "independent observation path", actionName: "turn_off", input: `{}`, state: "off", attributes: `{}`, observedAt: now.Add(-time.Second), source: "external-meter", want: atoha.VerificationVerified, observations: true},
		{name: "climate setpoint", actionName: "set_temperature", input: `{"temperature":24}`, state: "heat", attributes: `{"temperature":24.0}`, observedAt: now.Add(-time.Second), want: atoha.VerificationVerified, observations: true},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, _ := actionAttempt(t, i, "living-room-target", tt.actionName, tt.input, now.Add(-time.Minute))
			var observations []atoha.Observation
			if tt.observations {
				source := tt.source
				if source == "" {
					source = ProviderID + "/rest"
				}
				observations = []atoha.Observation{stateObservation(t, i, source, tt.state, tt.attributes, tt.observedAt, now)}
			}
			result, err := verifier.Verify(context.Background(), action, observations, now)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status() != tt.want {
				t.Fatalf("Status() = %v; want %v (%s)", result.Status(), tt.want, result.Reason())
			}
		})
	}
}

func stateClient(t *testing.T, entityID, state, attributes string, lastUpdated time.Time) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/states/"+entityID {
			t.Errorf("path = %q; want state endpoint", r.URL.Path)
		}
		body := `{"entity_id":"` + entityID + `","state":"` + state + `","attributes":` + attributes + `,"last_updated":"` + lastUpdated.Format(time.RFC3339Nano) + `"}`
		return response(http.StatusOK, body), nil
	})}
}

func stateObservation(t *testing.T, n int, source, state, attributes string, observedAt, recordedAt time.Time) atoha.Observation {
	t.Helper()
	value := []byte(`{"entity_id":"fixture.entity","state":"` + state + `","attributes":` + attributes + `,"last_updated":"` + observedAt.Format(time.RFC3339Nano) + `"}`)
	observation, err := atoha.NewObservation(atoha.ObservationSpec{
		ID: atoha.ObservationID("observation-" + string(rune('a'+n))), Source: source,
		Target: "living-room-target", Value: value, ObservedAt: observedAt, RecordedAt: recordedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func actionAttempt(t *testing.T, n int, target, name, input string, now time.Time) (atoha.Action, atoha.ExecutionAttempt) {
	t.Helper()
	suffix := string(rune('a' + n))
	action, err := atoha.NewAction(atoha.ActionSpec{
		ID: atoha.ActionID("action-" + suffix), Target: target, Name: name,
		Input: []byte(input), RequestedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := atoha.NewExecutionAttempt(atoha.ExecutionAttemptSpec{
		ID: atoha.AttemptID("attempt-" + suffix), ActionID: action.ID(), StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return action, attempt
}

func mustProvider(t *testing.T, config Config) *Provider {
	t.Helper()
	provider, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
