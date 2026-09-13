package execution

import (
	"encoding/json"
	"time"

	"github.com/habiohq/atoha"
)

// EventDataSchemaV1 identifies the JSON event payload schema emitted by this
// application package. EventKind remains the authoritative semantic fact.
const EventDataSchemaV1 = "atoha.execution.event/v1"

type actionRequestedPayload struct {
	Schema      string    `json:"schema"`
	Target      string    `json:"target"`
	Name        string    `json:"name"`
	Input       []byte    `json:"input,omitempty"`
	RequestedAt time.Time `json:"requested_at"`
}

type attemptStartedPayload struct {
	Schema     string          `json:"schema"`
	StartedAt  time.Time       `json:"started_at"`
	RecoveryOf atoha.AttemptID `json:"recovery_of,omitempty"`
}

type recoveryAuthorizedPayload struct {
	Schema            string          `json:"schema"`
	PreviousAttemptID atoha.AttemptID `json:"previous_attempt_id"`
	AuthorizedBy      string          `json:"authorized_by"`
	AuthorizedAt      time.Time       `json:"authorized_at"`
	Reason            string          `json:"reason"`
}

type admissionPayload struct {
	Schema string `json:"schema"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type dispatchPayload struct {
	Schema         string          `json:"schema"`
	Provider       string          `json:"provider,omitempty"`
	Status         string          `json:"status"`
	TargetProvider string          `json:"target_provider,omitempty"`
	TargetEndpoint []byte          `json:"target_endpoint,omitempty"`
	Receipt        *receiptPayload `json:"receipt,omitempty"`
	Error          string          `json:"error,omitempty"`
}

type receiptPayload struct {
	Provider   string    `json:"provider"`
	ReceivedAt time.Time `json:"received_at"`
	Evidence   []byte    `json:"evidence,omitempty"`
}

type observationPayload struct {
	Schema     string              `json:"schema"`
	ID         atoha.ObservationID `json:"id"`
	Source     string              `json:"source"`
	Target     string              `json:"target"`
	Value      []byte              `json:"value,omitempty"`
	Evidence   []byte              `json:"evidence,omitempty"`
	ObservedAt time.Time           `json:"observed_at"`
	RecordedAt time.Time           `json:"recorded_at"`
}

type verificationPayload struct {
	Schema         string                `json:"schema"`
	Status         string                `json:"status"`
	Verifier       string                `json:"verifier"`
	CheckedAt      time.Time             `json:"checked_at"`
	ObservationIDs []atoha.ObservationID `json:"observation_ids,omitempty"`
	Reason         string                `json:"reason,omitempty"`
	Error          string                `json:"error,omitempty"`
}

func marshalPayload(value any) ([]byte, error) { return json.Marshal(value) }

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
