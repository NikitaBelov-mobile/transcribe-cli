package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Client talks to local daemon API.
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) Health() error {
	resp, err := c.http.Get(c.baseURL + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("daemon returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Status  string `json:"status"`
		Service string `json:"service"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("invalid health payload: %w", err)
	}
	if payload.Status != "ok" {
		return errors.New("daemon health status is not ok")
	}
	if strings.TrimSpace(payload.Service) != "transcribe-cli" {
		return fmt.Errorf("unexpected health service: %q", payload.Service)
	}
	return nil
}

func (c *Client) AddJob(req AddJobRequest) (*Job, error) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(req); err != nil {
		return nil, err
	}
	resp, err := c.http.Post(c.baseURL+"/v1/jobs", "application/json", &body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, readError(resp.Body, resp.StatusCode)
	}
	var job Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) ListJobs() ([]*Job, error) {
	resp, err := c.http.Get(c.baseURL + "/v1/jobs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, readError(resp.Body, resp.StatusCode)
	}
	var payload struct {
		Jobs []*Job `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Jobs, nil
}

func (c *Client) GetJob(id string) (*Job, error) {
	resp, err := c.http.Get(c.url("/v1/jobs/", id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, readError(resp.Body, resp.StatusCode)
	}
	var job Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) CancelJob(id string) (*Job, error) {
	return c.postJobAction(id, "cancel")
}

func (c *Client) RetryJob(id string) (*Job, error) {
	return c.postJobAction(id, "retry")
}

func (c *Client) postJobAction(id, action string) (*Job, error) {
	resp, err := c.http.Post(c.url("/v1/jobs/", id, action), "application/json", http.NoBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, readError(resp.Body, resp.StatusCode)
	}
	var job Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) url(parts ...string) string {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return c.baseURL
	}
	u.Path = path.Join(append([]string{u.Path}, parts...)...)
	return u.String()
}

func readError(body io.Reader, code int) error {
	var payload map[string]any
	if err := json.NewDecoder(body).Decode(&payload); err == nil {
		if msg, ok := payload["error"].(string); ok && msg != "" {
			return fmt.Errorf("HTTP %d: %s", code, msg)
		}
	}
	return fmt.Errorf("HTTP %d", code)
}
