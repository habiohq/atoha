package homeassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseSize = 1 << 20

type serviceRequest struct {
	Domain  string
	Service string
	Body    []byte
}

type serviceResponse struct {
	received   bool
	statusCode int
	status     string
	body       []byte
}

type externalState struct {
	EntityID    string
	LastUpdated time.Time
	Raw         []byte
}

// haClient is the provider-owned external-system port and exposes no Habio types.
type haClient interface {
	callService(context.Context, serviceRequest) (serviceResponse, error)
	getState(context.Context, string) (externalState, error)
}

type restClient struct {
	baseURL *url.URL
	token   string
	client  *http.Client
}

func (c *restClient) callService(ctx context.Context, request serviceRequest) (serviceResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL("services", request.Domain, request.Service), bytes.NewReader(request.Body))
	if err != nil {
		return serviceResponse{}, err
	}
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return serviceResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := readLimited(resp.Body)
	return serviceResponse{received: true, statusCode: resp.StatusCode, status: resp.Status, body: body}, readErr
}

func (c *restClient) getState(ctx context.Context, entityID string) (externalState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL("states", entityID), nil)
	if err != nil {
		return externalState{}, err
	}
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return externalState{}, err
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return externalState{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return externalState{}, fmt.Errorf("%w: %s", ErrUnexpectedStatus, resp.Status)
	}
	var wire struct {
		EntityID    string    `json:"entity_id"`
		LastUpdated time.Time `json:"last_updated"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return externalState{}, fmt.Errorf("homeassistant: decode state: %w", err)
	}
	return externalState{EntityID: wire.EntityID, LastUpdated: wire.LastUpdated, Raw: body}, nil
}

func (c *restClient) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
}

func (c *restClient) apiURL(parts ...string) string {
	u := *c.baseURL
	escaped := make([]string, len(parts))
	for i, part := range parts {
		escaped[i] = url.PathEscape(part)
	}
	u.Path += "/api/" + strings.Join(escaped, "/")
	return u.String()
}

func readLimited(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxResponseSize+1))
	if err != nil {
		return body, err
	}
	if len(body) > maxResponseSize {
		return body[:maxResponseSize], ErrResponseTooLarge
	}
	return body, nil
}
