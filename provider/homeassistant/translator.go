package homeassistant

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/habiohq/habio"
)

// translator is the anti-corruption layer between Habio and Home Assistant.
type translator struct{ bindings map[string]string }

func newTranslator(source map[string]string) (translator, error) {
	bindings := make(map[string]string, len(source))
	for logical, entityID := range source {
		if strings.TrimSpace(logical) == "" || logical != strings.TrimSpace(logical) {
			return translator{}, fmt.Errorf("%w: invalid logical target %q", ErrInvalidConfig, logical)
		}
		if _, _, err := splitEntityID(entityID); err != nil {
			return translator{}, fmt.Errorf("%w: binding %q: %v", ErrInvalidConfig, logical, err)
		}
		bindings[logical] = entityID
	}
	return translator{bindings: bindings}, nil
}

func (t translator) resolve(action habio.Action) (habio.ResolvedTarget, error) {
	entityID, ok := t.bindings[action.Target()]
	if !ok {
		return habio.ResolvedTarget{}, fmt.Errorf("%w: %s", ErrTargetNotFound, action.Target())
	}
	return habio.NewResolvedTarget(ProviderID, []byte(entityID))
}

func (t translator) entityFor(action habio.Action, target habio.ResolvedTarget) (string, error) {
	if target.Provider() != ProviderID {
		return "", fmt.Errorf("%w: provider %q", ErrInvalidTarget, target.Provider())
	}
	entityID := string(target.Endpoint())
	if _, _, err := splitEntityID(entityID); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidTarget, err)
	}
	expected, ok := t.bindings[action.Target()]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrTargetNotFound, action.Target())
	}
	if entityID != expected {
		return "", fmt.Errorf("%w: target %q resolves to %q, got %q", ErrTargetMismatch, action.Target(), expected, entityID)
	}
	return entityID, nil
}

func (t translator) dispatchRequest(action habio.Action, target habio.ResolvedTarget) (serviceRequest, error) {
	entityID, err := t.entityFor(action, target)
	if err != nil {
		return serviceRequest{}, err
	}
	domain, _, _ := splitEntityID(entityID)
	if !validSegment(action.Name()) {
		return serviceRequest{}, fmt.Errorf("%w: invalid service action %q", ErrInvalidAction, action.Name())
	}
	body, err := serviceData(action.Input(), entityID)
	if err != nil {
		return serviceRequest{}, err
	}
	return serviceRequest{Domain: domain, Service: action.Name(), Body: body}, nil
}

func splitEntityID(entityID string) (string, string, error) {
	domain, objectID, ok := strings.Cut(entityID, ".")
	if !ok || !validSegment(domain) || !validSegment(objectID) {
		return "", "", fmt.Errorf("invalid entity ID %q", entityID)
	}
	return domain, objectID, nil
}

func validSegment(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func serviceData(input []byte, entityID string) ([]byte, error) {
	data := make(map[string]json.RawMessage)
	if len(bytes.TrimSpace(input)) != 0 {
		if err := json.Unmarshal(input, &data); err != nil {
			return nil, fmt.Errorf("%w: input must be a JSON object: %v", ErrInvalidAction, err)
		}
		if data == nil {
			return nil, fmt.Errorf("%w: input must be a JSON object", ErrInvalidAction)
		}
	}
	if _, exists := data["entity_id"]; exists {
		return nil, fmt.Errorf("%w: input cannot override resolved entity_id", ErrInvalidAction)
	}
	encodedEntity, _ := json.Marshal(entityID)
	data["entity_id"] = encodedEntity
	return json.Marshal(data)
}
