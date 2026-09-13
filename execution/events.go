package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/habiohq/atoha"
)

func newEvent(actionID atoha.ActionID, attemptID atoha.AttemptID, kind atoha.EventKind, qualifier string, occurredAt, recordedAt time.Time, data []byte) (atoha.ExecutionEvent, error) {
	id := deterministicEventID(actionID, attemptID, kind, qualifier)
	return atoha.NewExecutionEvent(atoha.ExecutionEventSpec{
		ID: id, ActionID: actionID, AttemptID: attemptID, Kind: kind,
		OccurredAt: occurredAt, RecordedAt: recordedAt, Data: data,
	})
}

func deterministicEventID(actionID atoha.ActionID, attemptID atoha.AttemptID, kind atoha.EventKind, qualifier string) atoha.EventID {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s", actionID, attemptID, kind, qualifier)))
	return atoha.EventID("event-" + hex.EncodeToString(digest[:16]))
}

func actionRequestedEvent(action atoha.Action, attemptID atoha.AttemptID, recordedAt time.Time) (atoha.ExecutionEvent, error) {
	data, err := marshalPayload(actionRequestedPayload{
		Schema: EventDataSchemaV1, Target: action.Target(), Name: action.Name(), Input: action.Input(), RequestedAt: action.RequestedAt(),
	})
	if err != nil {
		return atoha.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attemptID, atoha.EventActionRequested, string(attemptID), action.RequestedAt(), recordedAt, data)
}

func attemptStartedEvent(action atoha.Action, attempt atoha.ExecutionAttempt, recordedAt time.Time) (atoha.ExecutionEvent, error) {
	recoveryOf, _ := attempt.RecoveryOf()
	data, err := marshalPayload(attemptStartedPayload{
		Schema: EventDataSchemaV1, StartedAt: attempt.StartedAt(), RecoveryOf: recoveryOf,
	})
	if err != nil {
		return atoha.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attempt.ID(), atoha.EventAttemptStarted, "", attempt.StartedAt(), recordedAt, data)
}

func recoveryAuthorizedEvent(action atoha.Action, attempt atoha.ExecutionAttempt, authorization RecoveryAuthorization, recordedAt time.Time) (atoha.ExecutionEvent, error) {
	data, err := marshalPayload(recoveryAuthorizedPayload{
		Schema: EventDataSchemaV1, PreviousAttemptID: authorization.PreviousAttemptID(),
		AuthorizedBy: authorization.AuthorizedBy(), AuthorizedAt: authorization.AuthorizedAt(), Reason: authorization.Reason(),
	})
	if err != nil {
		return atoha.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attempt.ID(), atoha.EventRecoveryAuthorized, "", authorization.AuthorizedAt(), recordedAt, data)
}

func admissionEvent(action atoha.Action, attempt atoha.ExecutionAttempt, status atoha.AdmissionStatus, operationErr error, recordedAt time.Time) (atoha.ExecutionEvent, error) {
	kind := atoha.EventActionAdmitted
	if status == atoha.AdmissionRejected {
		kind = atoha.EventActionRejected
	}
	data, err := marshalPayload(admissionPayload{Schema: EventDataSchemaV1, Status: status.String(), Error: errorText(operationErr)})
	if err != nil {
		return atoha.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attempt.ID(), kind, "", recordedAt, recordedAt, data)
}

func notDispatchedEvent(action atoha.Action, attempt atoha.ExecutionAttempt, cause error, recordedAt time.Time) (atoha.ExecutionEvent, error) {
	data, err := marshalPayload(dispatchPayload{Schema: EventDataSchemaV1, Status: atoha.DispatchNotDispatched.String(), Error: errorText(cause)})
	if err != nil {
		return atoha.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attempt.ID(), atoha.EventNotDispatched, "", recordedAt, recordedAt, data)
}

func dispatchEvents(action atoha.Action, attempt atoha.ExecutionAttempt, target atoha.ResolvedTarget, result atoha.DispatchResult, operationErr error, recordedAt time.Time) ([]atoha.ExecutionEvent, error) {
	kind := atoha.EventDispatchUnknown
	switch result.Status() {
	case atoha.DispatchNotDispatched:
		kind = atoha.EventNotDispatched
	case atoha.DispatchDispatched:
		kind = atoha.EventActionDispatched
	case atoha.DispatchAcknowledged:
		kind = atoha.EventProviderAcknowledged
	}
	payload := dispatchPayload{
		Schema: EventDataSchemaV1, Provider: result.Provider(), Status: result.Status().String(),
		TargetProvider: target.Provider(), TargetEndpoint: target.Endpoint(), Error: errorText(operationErr),
	}
	if receipt, ok := result.Receipt(); ok {
		payload.Receipt = &receiptPayload{Provider: receipt.Provider(), ReceivedAt: receipt.ReceivedAt(), Evidence: receipt.Evidence()}
	}
	data, err := marshalPayload(payload)
	if err != nil {
		return nil, err
	}
	dispatchEvent, err := newEvent(action.ID(), attempt.ID(), kind, "", recordedAt, recordedAt, data)
	if err != nil {
		return nil, err
	}
	events := []atoha.ExecutionEvent{dispatchEvent}
	if result.Status() == atoha.DispatchDispatched || result.Status() == atoha.DispatchAcknowledged {
		effectEvent, err := newEvent(action.ID(), attempt.ID(), atoha.EventEffectUnverified, "dispatch", recordedAt, recordedAt, nil)
		if err != nil {
			return nil, err
		}
		events = append(events, effectEvent)
	}
	return events, nil
}

func observationEvent(action atoha.Action, attemptID atoha.AttemptID, observation atoha.Observation) (atoha.ExecutionEvent, error) {
	data, err := marshalPayload(observationPayload{
		Schema: EventDataSchemaV1, ID: observation.ID(), Source: observation.Source(), Target: observation.Target(),
		Value: observation.Value(), Evidence: observation.Evidence(), ObservedAt: observation.ObservedAt(), RecordedAt: observation.RecordedAt(),
	})
	if err != nil {
		return atoha.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attemptID, atoha.EventObservationRecorded, string(observation.ID()), observation.ObservedAt(), observation.RecordedAt(), data)
}

func verificationEvents(action atoha.Action, attemptID atoha.AttemptID, result atoha.VerificationResult, operationErr error, recordedAt time.Time) ([]atoha.ExecutionEvent, error) {
	if result.Status() == atoha.VerificationUnknown {
		return nil, nil
	}
	verificationKind := atoha.EventVerificationInconclusive
	effectKind := atoha.EventEffectUnverified
	switch result.Status() {
	case atoha.VerificationVerified:
		verificationKind = atoha.EventVerificationVerified
		effectKind = atoha.EventEffectObservedSatisfied
	case atoha.VerificationUnsatisfied:
		verificationKind = atoha.EventVerificationUnsatisfied
		effectKind = atoha.EventEffectObservedUnsatisfied
	}
	qualifier := result.Verifier() + "\x00" + result.CheckedAt().Format(time.RFC3339Nano)
	effectEvent, err := newEvent(action.ID(), attemptID, effectKind, qualifier, result.CheckedAt(), recordedAt, nil)
	if err != nil {
		return nil, err
	}
	data, err := marshalPayload(verificationPayload{
		Schema: EventDataSchemaV1, Status: result.Status().String(), Verifier: result.Verifier(), CheckedAt: result.CheckedAt(),
		ObservationIDs: result.ObservationIDs(), Reason: result.Reason(), Error: errorText(operationErr),
	})
	if err != nil {
		return nil, err
	}
	verificationEvent, err := newEvent(action.ID(), attemptID, verificationKind, qualifier, result.CheckedAt(), recordedAt, data)
	if err != nil {
		return nil, err
	}
	return []atoha.ExecutionEvent{effectEvent, verificationEvent}, nil
}
