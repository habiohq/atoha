package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/habiohq/habio"
	"github.com/habiohq/habio/eventlog/memory"
	"github.com/habiohq/habio/execution"
)

func TestExecutePreservesAmbiguousOutcomeInSuccessfulHTTPResponse(t *testing.T) {
	handler := testHandler(t)
	body := `{"action":{"id":"action-1","target":"light","name":"turn_on","input":{},"requested_at":"2026-09-12T12:00:00Z"},"attempt_id":"attempt-1"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/actions/execute", bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
	var value executeResponse
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.Dispatch != "unknown" || value.Effect != "unknown" || value.OperationError == "" {
		t.Fatalf("response = %#v; ambiguity was lost", value)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/actions/execute", bytes.NewBufferString(body))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d; want 409; body = %s", response.Code, response.Body.String())
	}
}

func TestAuthenticationAndStrictJSON(t *testing.T) {
	handler := testHandler(t)
	handler.apiToken = "secret"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/recovery/incomplete", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", response.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/actions/execute", bytes.NewBufferString(`{"unknown":true}`))
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("strict JSON status = %d; want 400", response.Code)
	}
}

func testHandler(t *testing.T) *Handler {
	t.Helper()
	log := memory.New()
	execute, err := execution.NewExecuteAction(execution.ExecuteConfig{
		Admitter: admitted{}, Resolver: resolved{}, Provider: ambiguous{}, Journal: log,
		Now: func() time.Time { return time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	verify, _ := execution.NewVerifyAttempt(execution.VerifyConfig{
		Resolver: resolved{}, Observer: noObservations{}, Verifier: inconclusive{},
		Journal: log, Reader: log, Actions: log,
	})
	recoverUseCase, _ := execution.NewRecoverAttempt(execute, log)
	getAttempt, _ := execution.NewGetAttempt(log)
	scanner, _ := execution.NewScanIncompleteAttempts(log, log)
	handler, err := New(Config{
		Executor: execute, Verifier: verify, Recoverer: recoverUseCase, GetAttempt: getAttempt, Scanner: scanner,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

type admitted struct{}

func (admitted) Admit(context.Context, habio.Action) (habio.AdmissionStatus, error) {
	return habio.AdmissionAdmitted, nil
}

type resolved struct{}

func (resolved) Resolve(context.Context, habio.Action) (habio.ResolvedTarget, error) {
	return habio.NewResolvedTarget("fixture", nil)
}

type ambiguous struct{}

func (ambiguous) Dispatch(_ context.Context, attempt habio.ExecutionAttempt, _ habio.Action, _ habio.ResolvedTarget) (habio.DispatchResult, error) {
	result, _ := habio.NewDispatchResult(habio.DispatchResultSpec{Provider: "fixture", AttemptID: attempt.ID(), Status: habio.DispatchUnknown})
	return result, errors.New("fixture: timeout")
}

type noObservations struct{}

func (noObservations) Observe(context.Context, habio.Action, habio.ResolvedTarget) ([]habio.Observation, error) {
	return nil, nil
}

type inconclusive struct{}

func (inconclusive) Verify(_ context.Context, _ habio.Action, _ []habio.Observation, at time.Time) (habio.VerificationResult, error) {
	return habio.NewVerificationResult(habio.VerificationResultSpec{
		Status: habio.VerificationInconclusive, Verifier: "fixture", CheckedAt: at, Reason: "none",
	})
}
