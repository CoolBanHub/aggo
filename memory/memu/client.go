package memu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client for the memu memory service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new memu Client. If httpClient is nil, a default client
// with a 30-second timeout is used. The baseURL trailing slash is stripped.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

// Retrieve sends a retrieval request to the memu /retrieve endpoint.
func (c *Client) Retrieve(ctx context.Context, req RetrieveRequest) (RetrieveResponse, error) {
	var resp RetrieveResponse
	if err := c.postJSON(ctx, "/retrieve", req, &resp); err != nil {
		return RetrieveResponse{}, err
	}
	return resp, nil
}

// Memorize sends a memorization request to the memu /memorize endpoint.
func (c *Client) Memorize(ctx context.Context, req MemorizeRequest) (MemorizeResponse, error) {
	var resp MemorizeResponse
	if err := c.postJSON(ctx, "/memorize", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// postJSON is a helper that sends a JSON POST request and decodes the response.
func (c *Client) postJSON(ctx context.Context, path string, requestBody any, responseBody any) error {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal %s request: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send %s request: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%s returned status %s", path, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(responseBody); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}

	return nil
}
