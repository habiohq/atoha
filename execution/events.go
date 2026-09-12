package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/habiohq/habio"
)

func newEvent(actionID habio.ActionID, attemptID habio.AttemptID, kind habio.EventKind, qualifier string, occurredAt, recordedAt time.Time, data []byte) (habio.ExecutionEvent, error) {
	id := deterministicEventID(actionID, attemptID, kind, qualifier)
	return habio.NewExecutionEvent(habio.ExecutionEventSpec{
		ID: id, ActionID: actionID, AttemptID: attemptID, Kind: kind,
		OccurredAt: occurredAt, RecordedAt: recordedAt, Data: data,
	})
}

func deterministicEventID(actionID habio.ActionID, attemptID habio.AttemptID, kind habio.EventKind, qualifier string) habio.EventID {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s", actionID, attemptID, kind, qualifier)))
	return habio.EventID("event-" + hex.EncodeToString(digest[:16]))
}

func actionRequestedEvent(action habio.Action, attemptID habio.AttemptID, recordedAt time.Time) (habio.ExecutionEvent, error) {
	data, err := marshalPayload(actionRequestedPayload{
		Schema: EventDataSchemaV1, Target: action.Target(), Name: action.Name(), Input: action.Input(), RequestedAt: action.RequestedAt(),
	})
	if err != nil {
		return habio.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attemptID, habio.EventActionRequested, string(attemptID), action.RequestedAt(), recordedAt, data)
}

func attemptStartedEvent(action habio.Action, attempt habio.ExecutionAttempt, recordedAt time.Time) (habio.ExecutionEvent, error) {
	recoveryOf, _ := attempt.RecoveryOf()
	data, err := marshalPayload(attemptStartedPayload{
		Schema: EventDataSchemaV1, StartedAt: attempt.StartedAt(), RecoveryOf: recoveryOf,
	})
	if err != nil {
		return habio.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attempt.ID(), habio.EventAttemptStarted, "", attempt.StartedAt(), recordedAt, data)
}

func recoveryAuthorizedEvent(action habio.Action, attempt habio.ExecutionAttempt, authorization RecoveryAuthorization, recordedAt time.Time) (habio.ExecutionEvent, error) {
	data, err := marshalPayload(recoveryAuthorizedPayload{
		Schema: EventDataSchemaV1, PreviousAttemptID: authorization.PreviousAttemptID(),
		AuthorizedBy: authorization.AuthorizedBy(), AuthorizedAt: authorization.AuthorizedAt(), Reason: authorization.Reason(),
	})
	if err != nil {
		return habio.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attempt.ID(), habio.EventRecoveryAuthorized, "", authorization.AuthorizedAt(), recordedAt, data)
}

func admissionEvent(action habio.Action, attempt habio.ExecutionAttempt, status habio.AdmissionStatus, operationErr error, recordedAt time.Time) (habio.ExecutionEvent, error) {
	kind := habio.EventActionAdmitted
	if status == habio.AdmissionRejected {
		kind = habio.EventActionRejected
	}
	data, err := marshalPayload(admissionPayload{Schema: EventDataSchemaV1, Status: status.String(), Error: errorText(operationErr)})
	if err != nil {
		return habio.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attempt.ID(), kind, "", recordedAt, recordedAt, data)
}

func notDispatchedEvent(action habio.Action, attempt habio.ExecutionAttempt, cause error, recordedAt time.Time) (habio.ExecutionEvent, error) {
	data, err := marshalPayload(dispatchPayload{Schema: EventDataSchemaV1, Status: habio.DispatchNotDispatched.String(), Error: errorText(cause)})
	if err != nil {
		return habio.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attempt.ID(), habio.EventNotDispatched, "", recordedAt, recordedAt, data)
}

func dispatchEvents(action habio.Action, attempt habio.ExecutionAttempt, target habio.ResolvedTarget, result habio.DispatchResult, operationErr error, recordedAt time.Time) ([]habio.ExecutionEvent, error) {
	kind := habio.EventDispatchUnknown
	switch result.Status() {
	case habio.DispatchNotDispatched:
		kind = habio.EventNotDispatched
	case habio.DispatchDispatched:
		kind = habio.EventActionDispatched
	case habio.DispatchAcknowledged:
		kind = habio.EventProviderAcknowledged
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
	events := []habio.ExecutionEvent{dispatchEvent}
	if result.Status() == habio.DispatchDispatched || result.Status() == habio.DispatchAcknowledged {
		effectEvent, err := newEvent(action.ID(), attempt.ID(), habio.EventEffectUnverified, "dispatch", recordedAt, recordedAt, nil)
		if err != nil {
			return nil, err
		}
		events = append(events, effectEvent)
	}
	return events, nil
}

func observationEvent(action habio.Action, attemptID habio.AttemptID, observation habio.Observation) (habio.ExecutionEvent, error) {
	data, err := marshalPayload(observationPayload{
		Schema: EventDataSchemaV1, ID: observation.ID(), Source: observation.Source(), Target: observation.Target(),
		Value: observation.Value(), Evidence: observation.Evidence(), ObservedAt: observation.ObservedAt(), RecordedAt: observation.RecordedAt(),
	})
	if err != nil {
		return habio.ExecutionEvent{}, err
	}
	return newEvent(action.ID(), attemptID, habio.EventObservationRecorded, string(observation.ID()), observation.ObservedAt(), observation.RecordedAt(), data)
}

func verificationEvents(action habio.Action, attemptID habio.AttemptID, result habio.VerificationResult, operationErr error, recordedAt time.Time) ([]habio.ExecutionEvent, error) {
	if result.Status() == habio.VerificationUnknown {
		return nil, nil
	}
	verificationKind := habio.EventVerificationInconclusive
	effectKind := habio.EventEffectUnverified
	switch result.Status() {
	case habio.VerificationVerified:
		verificationKind = habio.EventVerificationVerified
		effectKind = habio.EventEffectObservedSatisfied
	case habio.VerificationUnsatisfied:
		verificationKind = habio.EventVerificationUnsatisfied
		effectKind = habio.EventEffectObservedUnsatisfied
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
	return []habio.ExecutionEvent{effectEvent, verificationEvent}, nil
}
