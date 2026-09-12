// Package homeassistant provides a REST proof-of-concept anti-corruption layer.
// Provider is the Habio-facing facade; REST transport and translation stay in
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

	"github.com/habiohq/habio"
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

// Provider is the Habio-facing facade over translation and REST transport.
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

func (p *Provider) Resolve(_ context.Context, action habio.Action) (habio.ResolvedTarget, error) {
	return p.translator.resolve(action)
}

// Dispatch translates a Habio Action and delegates exactly one REST call.
func (p *Provider) Dispatch(ctx context.Context, attempt habio.ExecutionAttempt, action habio.Action, target habio.ResolvedTarget) (habio.DispatchResult, error) {
	if attempt.ActionID() != action.ID() {
		return p.result(attempt.ID(), habio.DispatchNotDispatched, nil, fmt.Errorf("%w: attempt action %q, action %q", ErrAttemptMismatch, attempt.ActionID(), action.ID()))
	}
	request, err := p.translator.dispatchRequest(action, target)
	if err != nil {
		return p.result(attempt.ID(), habio.DispatchNotDispatched, nil, err)
	}
	response, err := p.client.callService(ctx, request)
	if err != nil && !response.received {
		return p.result(attempt.ID(), habio.DispatchUnknown, nil, err)
	}
	if response.statusCode < http.StatusOK || response.statusCode >= http.StatusMultipleChoices {
		statusErr := fmt.Errorf("%w: %s", ErrUnexpectedStatus, response.status)
		return p.result(attempt.ID(), habio.DispatchDispatched, nil, errors.Join(statusErr, err))
	}
	receipt, receiptErr := habio.NewReceipt(habio.ReceiptSpec{
		Provider: ProviderID, AttemptID: attempt.ID(), ReceivedAt: p.now(), Evidence: response.body,
	})
	if receiptErr != nil {
		return habio.DispatchResult{}, receiptErr
	}
	return p.result(attempt.ID(), habio.DispatchAcknowledged, &receipt, err)
}

func (p *Provider) Observe(ctx context.Context, action habio.Action, target habio.ResolvedTarget) ([]habio.Observation, error) {
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
	observation, err := habio.NewObservation(habio.ObservationSpec{
		ID: habio.ObservationID("ha-" + hex.EncodeToString(hash[:])), Source: ProviderID + "/rest",
		Target: action.Target(), Value: state.Raw, ObservedAt: state.LastUpdated, RecordedAt: p.now(),
	})
	if err != nil {
		return nil, err
	}
	return []habio.Observation{observation}, nil
}

func (p *Provider) result(attemptID habio.AttemptID, status habio.DispatchStatus, receipt *habio.Receipt, operationErr error) (habio.DispatchResult, error) {
	result, err := habio.NewDispatchResult(habio.DispatchResultSpec{
		Provider: ProviderID, AttemptID: attemptID, Status: status, Receipt: receipt,
	})
	if err != nil {
		return habio.DispatchResult{}, err
	}
	return result, operationErr
}

var (
	_ habio.Provider = (*Provider)(nil)
	_ habio.Resolver = (*Provider)(nil)
	_ habio.Observer = (*Provider)(nil)
)
