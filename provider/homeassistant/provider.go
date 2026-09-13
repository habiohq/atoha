// Package homeassistant provides a REST proof-of-concept anti-corruption layer.
// Provider is the Atoha-facing facade; REST transport and translation stay in
// separate implementation files so Home Assistant concepts do not leak inward.
package homeassistant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/habiohq/atoha"
)

const ProviderID = "homeassistant"

var (
	ErrInvalidConfig    = errors.New("homeassistant: invalid config")
	ErrTargetNotFound   = errors.New("homeassistant: logical target not found")
	ErrInvalidTarget    = errors.New("homeassistant: invalid resolved target")
	ErrTargetMismatch   = errors.New("homeassistant: resolved target does not match action")
	ErrAttemptMismatch  = errors.New("homeassistant: execution attempt does not match action")
	ErrInvalidAction    = errors.New("homeassistant: invalid action")
	ErrUnexpectedStatus = errors.New("homeassistant: unexpected HTTP status")
	ErrResponseTooLarge = errors.New("homeassistant: response too large")
)

type Config struct {
	BaseURL  string
	Token    string
	Bindings map[string]string
	Client   *http.Client
	Now      func() time.Time
}

// Provider is the Atoha-facing facade over translation and REST transport.
type Provider struct {
	translator translator
	client     haClient
	now        func() time.Time
}

func New(config Config) (*Provider, error) {
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return nil, fmt.Errorf("%w: base URL must be absolute HTTP(S)", ErrInvalidConfig)
	}
	if baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("%w: base URL cannot contain query or fragment", ErrInvalidConfig)
	}
	if strings.TrimSpace(config.Token) == "" {
		return nil, fmt.Errorf("%w: token is required", ErrInvalidConfig)
	}
	translation, err := newTranslator(config.Bindings)
	if err != nil {
		return nil, err
	}
	httpClient := config.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/")
	return &Provider{
		translator: translation,
		client:     &restClient{baseURL: baseURL, token: config.Token, client: httpClient},
		now:        now,
	}, nil
}

func (p *Provider) Resolve(_ context.Context, action atoha.Action) (atoha.ResolvedTarget, error) {
	return p.translator.resolve(action)
}

// Dispatch translates a Atoha Action and delegates exactly one REST call.
func (p *Provider) Dispatch(ctx context.Context, attempt atoha.ExecutionAttempt, action atoha.Action, target atoha.ResolvedTarget) (atoha.DispatchResult, error) {
	if attempt.ActionID() != action.ID() {
		return p.result(attempt.ID(), atoha.DispatchNotDispatched, nil, fmt.Errorf("%w: attempt action %q, action %q", ErrAttemptMismatch, attempt.ActionID(), action.ID()))
	}
	request, err := p.translator.dispatchRequest(action, target)
	if err != nil {
		return p.result(attempt.ID(), atoha.DispatchNotDispatched, nil, err)
	}
	response, err := p.client.callService(ctx, request)
	if err != nil && !response.received {
		return p.result(attempt.ID(), atoha.DispatchUnknown, nil, err)
	}
	if response.statusCode < http.StatusOK || response.statusCode >= http.StatusMultipleChoices {
		statusErr := fmt.Errorf("%w: %s", ErrUnexpectedStatus, response.status)
		return p.result(attempt.ID(), atoha.DispatchDispatched, nil, errors.Join(statusErr, err))
	}
	receipt, receiptErr := atoha.NewReceipt(atoha.ReceiptSpec{
		Provider: ProviderID, AttemptID: attempt.ID(), ReceivedAt: p.now(), Evidence: response.body,
	})
	if receiptErr != nil {
		return atoha.DispatchResult{}, receiptErr
	}
	return p.result(attempt.ID(), atoha.DispatchAcknowledged, &receipt, err)
}

func (p *Provider) Observe(ctx context.Context, action atoha.Action, target atoha.ResolvedTarget) ([]atoha.Observation, error) {
	entityID, err := p.translator.entityFor(action, target)
	if err != nil {
		return nil, err
	}
	state, err := p.client.getState(ctx, entityID)
	if err != nil {
		return nil, err
	}
	if state.EntityID != entityID {
		return nil, fmt.Errorf("homeassistant: state entity %q does not match %q", state.EntityID, entityID)
	}
	if state.LastUpdated.IsZero() {
		return nil, errors.New("homeassistant: state has no last_updated time")
	}
	hash := sha256.Sum256(append([]byte(entityID+"\x00"), state.Raw...))
	observation, err := atoha.NewObservation(atoha.ObservationSpec{
		ID: atoha.ObservationID("ha-" + hex.EncodeToString(hash[:])), Source: ProviderID + "/rest",
		Target: action.Target(), Value: state.Raw, ObservedAt: state.LastUpdated, RecordedAt: p.now(),
	})
	if err != nil {
		return nil, err
	}
	return []atoha.Observation{observation}, nil
}

func (p *Provider) result(attemptID atoha.AttemptID, status atoha.DispatchStatus, receipt *atoha.Receipt, operationErr error) (atoha.DispatchResult, error) {
	result, err := atoha.NewDispatchResult(atoha.DispatchResultSpec{
		Provider: ProviderID, AttemptID: attemptID, Status: status, Receipt: receipt,
	})
	if err != nil {
		return atoha.DispatchResult{}, err
	}
	return result, operationErr
}

var (
	_ atoha.Provider = (*Provider)(nil)
	_ atoha.Resolver = (*Provider)(nil)
	_ atoha.Observer = (*Provider)(nil)
)
