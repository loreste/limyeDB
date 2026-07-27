package limyedb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	host       string
	authToken  string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithAuthToken sends the given bearer token on every request. Required to
// reach a server started with -auth-token; omit it only for a server running
// with -allow-anonymous.
func WithAuthToken(token string) Option {
	return func(c *Client) { c.authToken = token }
}

// WithHTTPClient overrides the default HTTP client (e.g. to set a custom
// timeout or transport).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

func NewClient(host string, opts ...Option) *Client {
	if host == "" {
		host = "http://localhost:8080"
	}
	c := &Client{
		host: host,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) request(method, path string, payload interface{}, target interface{}) error {
	var bodyReader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		// Exclude nulls by recreating without defaults (Go json does this well natively except for omitted maps).
		// Standard REST endpoints process standard omitempty structs flawlessly.
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.host+path, bodyReader)
	if err != nil {
		return err
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) CreateCollection(config CollectionConfig) error {
	if config.Metric == "" {
		config.Metric = "cosine"
	}
	return c.request(http.MethodPost, "/collections", config, nil)
}

func (c *Client) DeleteCollection(name string) error {
	return c.request(http.MethodDelete, "/collections/"+name, nil, nil)
}

func (c *Client) Upsert(collectionName string, points []Point) error {
	payload := map[string]interface{}{
		"points": points,
	}
	return c.request(http.MethodPut, "/collections/"+collectionName+"/points", payload, nil)
}

func (c *Client) Search(collectionName string, vector []float32, limit int) ([]Match, error) {
	if limit <= 0 {
		limit = 10
	}
	payload := map[string]interface{}{
		"vector": vector,
		"limit":  limit,
	}

	var result struct {
		Result []Match `json:"result"`
	}
	if err := c.request(http.MethodPost, "/collections/"+collectionName+"/search", payload, &result); err != nil {
		return nil, err
	}
	return result.Result, nil
}

func (c *Client) Discover(collectionName string, params DiscoverParams) ([]Match, error) {
	if params.Limit <= 0 {
		params.Limit = 10
	}

	var result struct {
		Points []Match `json:"points"`
	}
	if err := c.request(http.MethodPost, "/collections/"+collectionName+"/discover", params, &result); err != nil {
		return nil, err
	}
	return result.Points, nil
}
