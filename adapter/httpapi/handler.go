// Package httpapi exposes Habio application use cases without changing their
// uncertainty semantics.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/habiohq/habio"
	"github.com/habiohq/habio/execution"
)

const maxRequestSize = 1 << 20

// Executor runs one initial attempt.
type Executor interface {
	Execute(context.Context, execution.ExecuteInput) (execution.ExecuteResult, error)
}

// Verifier observes and verifies an existing attempt.
type Verifier interface {
	Verify(context.Context, execution.VerifyInput) (execution.VerifyResult, error)
}

// Recoverer runs an explicitly authorized recovery attempt.
type Recoverer interface {
	Recover(context.Context, execution.RecoverInput) (execution.ExecuteResult, error)
}

// AttemptGetter reads a projected attempt view.
type AttemptGetter interface {
	Get(context.Context, habio.AttemptID) (execution.AttemptView, error)
}

// IncompleteScanner reports attempts that require inspection.
type IncompleteScanner interface {
	Scan(context.Context) ([]execution.IncompleteAttempt, error)
}

// Config supplies application ports; transport code owns no concrete provider or storage.
type Config struct {
	Executor   Executor
	Verifier   Verifier
	Recoverer  Recoverer
	GetAttempt AttemptGetter
	Scanner    IncompleteScanner
	APIToken   string
}

// Handler implements the reference Habio HTTP API.
type Handler struct {
	executor   Executor
	verifier   Verifier
	recoverer  Recoverer
	getAttempt AttemptGetter
	scanner    IncompleteScanner
	apiToken   string
	mux        *http.ServeMux
}

// New constructs a strict HTTP handler from application-level ports.
func New(config Config) (*Handler, error) {
	if config.Executor == nil || config.Verifier == nil || config.Recoverer == nil || config.GetAttempt == nil || config.Scanner == nil {
		return nil, errors.New("habio http api: all use cases are required")
	}
	h := &Handler{
		executor: config.Executor, verifier: config.Verifier, recoverer: config.Recoverer,
		getAttempt: config.GetAttempt, scanner: config.Scanner, apiToken: config.APIToken,
		mux: http.NewServeMux(),
	}
	h.mux.HandleFunc("POST /v1/actions/execute", h.execute)
	h.mux.HandleFunc("POST /v1/actions/verify", h.verify)
	h.mux.HandleFunc("POST /v1/actions/recover", h.recover)
	h.mux.HandleFunc("GET /v1/attempts/{attempt_id}", h.get)
	h.mux.HandleFunc("GET /v1/recovery/incomplete", h.scan)
	h.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.apiToken != "" && r.URL.Path != "/healthz" && !sameToken(r.Header.Get("Authorization"), "Bearer "+h.apiToken) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}
	h.mux.ServeHTTP(w, r)
}

type actionRequest struct {
	ID          habio.ActionID  `json:"id"`
	Target      string          `json:"target"`
	Name        string          `json:"name"`
	Input       json.RawMessage `json:"input,omitempty"`
	RequestedAt time.Time       `json:"requested_at"`
}

func (r actionRequest) action() (habio.Action, error) {
	return habio.NewAction(habio.ActionSpec{
		ID: r.ID, Target: r.Target, Name: r.Name, Input: r.Input, RequestedAt: r.RequestedAt,
	})
}

type executeRequest struct {
	Action    actionRequest   `json:"action"`
	AttemptID habio.AttemptID `json:"attempt_id"`
}

type recoverRequest struct {
	Action       actionRequest   `json:"action"`
	AttemptID    habio.AttemptID `json:"attempt_id"`
	Previous     habio.AttemptID `json:"previous_attempt_id"`
	AuthorizedBy string          `json:"authorized_by"`
	AuthorizedAt time.Time       `json:"authorized_at"`
	Reason       string          `json:"reason"`
}

type executeResponse struct {
	ActionID       habio.ActionID  `json:"action_id"`
	AttemptID      habio.AttemptID `json:"attempt_id"`
	RecoveryOf     habio.AttemptID `json:"recovery_of,omitempty"`
	Admission      string          `json:"admission"`
	Dispatch       string          `json:"dispatch"`
	Effect         string          `json:"effect"`
	Provider       string          `json:"provider,omitempty"`
	OperationError string          `json:"operation_error,omitempty"`
}

type verifyResponse struct {
	ActionID       habio.ActionID        `json:"action_id"`
	AttemptID      habio.AttemptID       `json:"attempt_id"`
	Status         string                `json:"status"`
	Verifier       string                `json:"verifier,omitempty"`
	ObservationIDs []habio.ObservationID `json:"observation_ids,omitempty"`
	Reason         string                `json:"reason,omitempty"`
	OperationError string                `json:"operation_error,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) execute(w http.ResponseWriter, r *http.Request) {
	var request executeRequest
	if !decode(w, r, &request) {
		return
	}
	action, err := request.Action.action()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	result, operationErr := h.executor.Execute(r.Context(), execution.ExecuteInput{Action: action, AttemptID: request.AttemptID})
	writeExecution(w, result, operationErr)
}

func (h *Handler) recover(w http.ResponseWriter, r *http.Request) {
	var request recoverRequest
	if !decode(w, r, &request) {
		return
	}
	action, err := request.Action.action()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	authorization, err := execution.NewRecoveryAuthorization(execution.RecoveryAuthorizationSpec{
		ActionID: action.ID(), PreviousAttemptID: request.Previous, AuthorizedBy: request.AuthorizedBy,
		AuthorizedAt: request.AuthorizedAt, Reason: request.Reason,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	result, operationErr := h.recoverer.Recover(r.Context(), execution.RecoverInput{
		Action: action, AttemptID: request.AttemptID, Authorization: authorization,
	})
	writeExecution(w, result, operationErr)
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	var request executeRequest
	if !decode(w, r, &request) {
		return
	}
	action, err := request.Action.action()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	result, operationErr := h.verifier.Verify(r.Context(), execution.VerifyInput{Action: action, AttemptID: request.AttemptID})
	if result.AttemptID == "" || errors.Is(operationErr, habio.ErrActionIdentityConflict) {
		writeUseCaseError(w, operationErr)
		return
	}
	response := verifyResponse{
		ActionID: result.ActionID, AttemptID: result.AttemptID,
		Status: result.Verification.Status().String(), Verifier: result.Verification.Verifier(),
		ObservationIDs: result.Verification.ObservationIDs(), Reason: result.Verification.Reason(),
	}
	if operationErr != nil {
		response.OperationError = operationErr.Error()
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	view, err := h.getAttempt.Get(r.Context(), habio.AttemptID(r.PathValue("attempt_id")))
	if err != nil {
		writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"action_id": view.ActionID, "attempt_id": view.AttemptID,
		"admission": view.Outcome.Admission().String(), "dispatch": view.Outcome.Dispatch().String(),
		"effect": view.Outcome.Effect().String(), "verification": view.Verification.String(),
		"conflicts": map[string]bool{
			"admission": view.AdmissionConflicted, "dispatch": view.DispatchConflicted, "effect": view.EffectConflicted,
		},
	})
}

func (h *Handler) scan(w http.ResponseWriter, r *http.Request) {
	items, err := h.scanner.Scan(r.Context())
	if err != nil {
		writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempts": items})
}

func writeExecution(w http.ResponseWriter, result execution.ExecuteResult, err error) {
	if result.Attempt.ID() == "" || errors.Is(err, execution.ErrAttemptAlreadyStarted) ||
		errors.Is(err, execution.ErrRecoveryNotAuthorized) || errors.Is(err, execution.ErrActionMismatch) ||
		errors.Is(err, habio.ErrActionIdentityConflict) {
		writeUseCaseError(w, err)
		return
	}
	response := executeResponse{
		ActionID: result.Action.ID(), AttemptID: result.Attempt.ID(),
		Admission: result.Outcome.Admission().String(), Dispatch: result.Outcome.Dispatch().String(),
		Effect: result.Outcome.Effect().String(), Provider: result.Dispatch.Provider(),
	}
	if previous, ok := result.Attempt.RecoveryOf(); ok {
		response.RecoveryOf = previous
	}
	if err != nil {
		response.OperationError = err.Error()
	}
	writeJSON(w, http.StatusOK, response)
}

func writeUseCaseError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, execution.ErrInvalidInput), errors.Is(err, execution.ErrRecoveryNotAuthorized), errors.Is(err, habio.ErrInvalidAction):
		status = http.StatusBadRequest
	case errors.Is(err, execution.ErrAttemptNotFound):
		status = http.StatusNotFound
	case errors.Is(err, execution.ErrAttemptAlreadyStarted), errors.Is(err, execution.ErrActionMismatch), errors.Is(err, habio.ErrActionIdentityConflict):
		status = http.StatusConflict
	}
	message := "internal error"
	if err != nil && status != http.StatusInternalServerError {
		message = err.Error()
	}
	writeJSON(w, status, errorResponse{Error: message})
}

func decode(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("invalid JSON request: %v", err)})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "request must contain one JSON value"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func sameToken(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
