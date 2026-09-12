package habio

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidResolvedTarget = errors.New("habio: invalid resolved target")
	ErrInvalidReceipt        = errors.New("habio: invalid receipt")
	ErrInvalidDispatchResult = errors.New("habio: invalid dispatch result")
)

// ResolvedTarget is an immutable provider-specific endpoint derived from a
// logical Action target. Endpoint is opaque to the core.
type ResolvedTarget struct {
	provider string
	endpoint []byte
}

// NewResolvedTarget creates a provider-specific endpoint. Endpoint may be
// empty when the provider needs only the logical target carried by Action.
func NewResolvedTarget(provider string, endpoint []byte) (ResolvedTarget, error) {
	if err := validateText("provider", provider); err != nil {
		return ResolvedTarget{}, fmt.Errorf("%w: %v", ErrInvalidResolvedTarget, err)
	}
	return ResolvedTarget{provider: provider, endpoint: cloneBytes(endpoint)}, nil
}

// Provider returns the identity of the provider selected by resolution.
func (t ResolvedTarget) Provider() string { return t.provider }

// Endpoint returns a copy of the opaque provider-specific endpoint.
func (t ResolvedTarget) Endpoint() []byte { return cloneBytes(t.endpoint) }

// ReceiptSpec contains provider acknowledgement evidence for one attempt.
type ReceiptSpec struct {
	Provider   string
	AttemptID  AttemptID
	ReceivedAt time.Time
	Evidence   []byte
}

// Receipt is immutable evidence that a Provider acknowledged an operation. It
// does not establish a physical effect. The zero value is invalid.
type Receipt struct {
	provider   string
	attemptID  AttemptID
	receivedAt time.Time
	evidence   []byte
}

// NewReceipt validates and copies provider acknowledgement evidence.
func NewReceipt(spec ReceiptSpec) (Receipt, error) {
	if err := validateText("provider", spec.Provider); err != nil {
		return Receipt{}, fmt.Errorf("%w: %v", ErrInvalidReceipt, err)
	}
	if err := validateIdentity("attempt ID", string(spec.AttemptID)); err != nil {
		return Receipt{}, fmt.Errorf("%w: %v", ErrInvalidReceipt, err)
	}
	if spec.ReceivedAt.IsZero() {
		return Receipt{}, fmt.Errorf("%w: received time is required", ErrInvalidReceipt)
	}
	return Receipt{
		provider:   spec.Provider,
		attemptID:  spec.AttemptID,
		receivedAt: spec.ReceivedAt,
		evidence:   cloneBytes(spec.Evidence),
	}, nil
}

func (r Receipt) Provider() string { return r.provider }

func (r Receipt) AttemptID() AttemptID { return r.attemptID }

func (r Receipt) ReceivedAt() time.Time { return r.receivedAt }

func (r Receipt) Evidence() []byte { return cloneBytes(r.evidence) }

// DispatchResultSpec describes the evidence available when Provider.Dispatch
// returns. Receipt must be present exactly when Status is acknowledged.
type DispatchResultSpec struct {
	Provider  string
	AttemptID AttemptID
	Status    DispatchStatus
	Receipt   *Receipt
}

// DispatchResult is immutable dispatch knowledge. Its zero value means that no
// provider evidence was returned. It can be returned alongside a non-nil Go
// error so callers do not lose evidence.
type DispatchResult struct {
	provider   string
	attemptID  AttemptID
	status     DispatchStatus
	receipt    Receipt
	hasReceipt bool
}

// NewDispatchResult validates result identity and acknowledgement evidence.
func NewDispatchResult(spec DispatchResultSpec) (DispatchResult, error) {
	if err := validateText("provider", spec.Provider); err != nil {
		return DispatchResult{}, fmt.Errorf("%w: %v", ErrInvalidDispatchResult, err)
	}
	if err := validateIdentity("attempt ID", string(spec.AttemptID)); err != nil {
		return DispatchResult{}, fmt.Errorf("%w: %v", ErrInvalidDispatchResult, err)
	}
	if !spec.Status.valid() {
		return DispatchResult{}, fmt.Errorf("%w: dispatch status %d", ErrInvalidDispatchResult, spec.Status)
	}
	if spec.Status == DispatchAcknowledged && spec.Receipt == nil {
		return DispatchResult{}, fmt.Errorf("%w: acknowledged status requires a receipt", ErrInvalidDispatchResult)
	}
	if spec.Status != DispatchAcknowledged && spec.Receipt != nil {
		return DispatchResult{}, fmt.Errorf("%w: receipt requires acknowledged status", ErrInvalidDispatchResult)
	}

	result := DispatchResult{
		provider:  spec.Provider,
		attemptID: spec.AttemptID,
		status:    spec.Status,
	}
	if spec.Receipt != nil {
		if spec.Receipt.Provider() != spec.Provider {
			return DispatchResult{}, fmt.Errorf("%w: receipt provider does not match result", ErrInvalidDispatchResult)
		}
		if spec.Receipt.AttemptID() != spec.AttemptID {
			return DispatchResult{}, fmt.Errorf("%w: receipt attempt does not match result", ErrInvalidDispatchResult)
		}
		result.receipt = *spec.Receipt
		result.hasReceipt = true
	}
	return result, nil
}

func (r DispatchResult) Provider() string { return r.provider }

func (r DispatchResult) AttemptID() AttemptID { return r.attemptID }

func (r DispatchResult) Status() DispatchStatus { return r.status }

// Receipt returns provider acknowledgement evidence when Status is
// DispatchAcknowledged.
func (r DispatchResult) Receipt() (Receipt, bool) { return r.receipt, r.hasReceipt }

// Provider is the minimum required physical execution backend contract.
//
// A non-nil error describes the software path, not the physical outcome.
// DispatchResult must retain any evidence known when the call returns. Context
// cancellation does not imply that a physical operation was cancelled.
// Implementations must reject an attempt whose ActionID does not match the
// supplied Action and must validate that target was resolved for that Action
// before performing external I/O.
type Provider interface {
	Dispatch(ctx context.Context, attempt ExecutionAttempt, action Action, target ResolvedTarget) (DispatchResult, error)
}

// Resolver maps a logical Action target to an opaque Provider endpoint.
type Resolver interface {
	Resolve(ctx context.Context, action Action) (ResolvedTarget, error)
}

// Admitter supplies application or domain policy at Habio's enforcement point.
// AdmissionStatus and error remain separate for the same reason as Outcome and
// transport errors.
type Admitter interface {
	Admit(ctx context.Context, action Action) (AdmissionStatus, error)
}

// Observer obtains physical/provider evidence independently of dispatch.
// Implementations must validate that target was resolved for the supplied
// Action before obtaining or labeling evidence.
type Observer interface {
	Observe(ctx context.Context, action Action, target ResolvedTarget) ([]Observation, error)
}
