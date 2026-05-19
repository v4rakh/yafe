package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"git.myservermanager.com/varakh/yafe/internal/auth"
)

const (
	defaultTimeout = 30 * time.Second
)

// Job represents a job in the queue.
type Job struct {
	ID        string            `json:"id"`
	Flow      string            `json:"flow"`
	Inputs    map[string]string `json:"inputs,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	StartedAt *time.Time        `json:"started_at,omitempty"`
	EndedAt   *time.Time        `json:"ended_at,omitempty"`
	Error     string            `json:"error,omitempty"`
	ExitCode  *int              `json:"exit_code,omitempty"`
}

// JobWithStatus includes the job's current status.
type JobWithStatus struct {
	Job
	Status string `json:"status"`
}

// FlowResponse contains flow details.
type FlowResponse struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// ErrorResponse is returned by the API on errors.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Client communicates with the yafe daemon.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// NewUnixSocketClient creates a client that connects via Unix socket.
func NewUnixSocketClient(socketPath, apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
			Timeout: defaultTimeout,
		},
		baseURL: "http://localhost", // Host is ignored for Unix socket
		apiKey:  apiKey,
	}
}

// NewHTTPClient creates a client that connects via HTTP.
func NewHTTPClient(baseURL, apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

// do execute the request with API key header if set.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	if c.apiKey != "" {
		req.Header.Set(auth.HeaderAPIKey, c.apiKey)
	}
	return c.httpClient.Do(req) //nolint:gosec
}

// ListJobs returns jobs filtered by status.
func (c *Client) ListJobs(statuses []string) ([]*JobWithStatus, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/jobs")
	if err != nil {
		return nil, err
	}

	if len(statuses) > 0 {
		q := u.Query()
		for _, s := range statuses {
			q.Add("status", s)
		}
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var jobs []*JobWithStatus
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return jobs, nil
}

// GetJob retrieves a job by ID.
func (c *Client) GetJob(id string) (*JobWithStatus, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL+"/api/v1/jobs/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var job JobWithStatus
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &job, nil
}

// GetJobLogs retrieves a job's logs by ID.
func (c *Client) GetJobLogs(id string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL+"/api/v1/jobs/"+url.PathEscape(id)+"/logs", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.do(req)
	if err != nil {
		return "", c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", c.parseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	return string(body), nil
}

// EnqueueJob creates a new job for the given flow.
func (c *Client) EnqueueJob(flow string, inputs map[string]string) (string, error) {
	body := map[string]any{
		"flow":   flow,
		"inputs": inputs,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.baseURL+"/api/v1/jobs", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return "", c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return "", c.parseError(resp)
	}

	var result struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return result.JobID, nil
}

// DeleteJob removes a job from the queue.
func (c *Client) DeleteJob(id string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, c.baseURL+"/api/v1/jobs/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

// ListFlows returns all flow names.
func (c *Client) ListFlows() ([]string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL+"/api/v1/flows", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var flows []string
	if err := json.NewDecoder(resp.Body).Decode(&flows); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return flows, nil
}

// GetFlow retrieves a flow by name.
func (c *Client) GetFlow(name string) (*FlowResponse, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL+"/api/v1/flows/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var flow FlowResponse
	if err := json.NewDecoder(resp.Body).Decode(&flow); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &flow, nil
}

// AddFlow creates or updates a flow.
func (c *Client) AddFlow(name string, content []byte) error {
	body := map[string]string{
		"content": string(content),
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, c.baseURL+"/api/v1/flows/"+url.PathEscape(name), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return c.parseError(resp)
	}

	return nil
}

// RenameFlow renames a flow.
func (c *Client) RenameFlow(oldName, newName string) error {
	reqBody := map[string]string{"new_name": newName}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.baseURL+"/api/v1/flows/"+url.PathEscape(oldName)+"/rename", bytes.NewReader(body))
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

// DeleteFlow removes a flow.
func (c *Client) DeleteFlow(name string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, c.baseURL+"/api/v1/flows/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
		return errors.New(errResp.Error)
	}

	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}

func (c *Client) wrapConnectionError(err error) error {
	// Check for connection refused or socket not found
	var netErr *net.OpError
	if errors.As(err, &netErr) && netErr.Op == "dial" {
		return fmt.Errorf("daemon not running: %w", err)
	}
	return fmt.Errorf("daemon not running: %w", err)
}

// Schedule represents a scheduled job trigger.
type Schedule struct {
	Name       string            `json:"name"`
	Flow       string            `json:"flow"`
	Type       string            `json:"type"`
	Expression string            `json:"expression"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	Enabled    bool              `json:"enabled"`
	CreatedAt  time.Time         `json:"created_at"`
	LastRunAt  *time.Time        `json:"last_run_at,omitempty"`
	NextRunAt  *time.Time        `json:"next_run_at,omitempty"`
}

// CreateScheduleRequest is used to create a new schedule.
type CreateScheduleRequest struct {
	Name       string            `json:"name"`
	Flow       string            `json:"flow"`
	Type       string            `json:"type"`
	Expression string            `json:"expression"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	Enabled    bool              `json:"enabled"`
}

// UpdateScheduleRequest is used to update an existing schedule.
type UpdateScheduleRequest struct {
	Flow       string            `json:"flow"`
	Type       string            `json:"type"`
	Expression string            `json:"expression"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	Enabled    bool              `json:"enabled"`
}

// ListSchedules returns all schedules.
func (c *Client) ListSchedules() ([]*Schedule, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL+"/api/v1/schedules", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var schedules []*Schedule
	if err := json.NewDecoder(resp.Body).Decode(&schedules); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return schedules, nil
}

// GetSchedule retrieves a schedule by name.
func (c *Client) GetSchedule(name string) (*Schedule, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL+"/api/v1/schedules/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var schedule Schedule
	if err := json.NewDecoder(resp.Body).Decode(&schedule); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &schedule, nil
}

// CreateSchedule creates a new schedule.
func (c *Client) CreateSchedule(req CreateScheduleRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.baseURL+"/api/v1/schedules", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.do(httpReq)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return c.parseError(resp)
	}

	return nil
}

// UpdateSchedule updates an existing schedule.
func (c *Client) UpdateSchedule(name string, req UpdateScheduleRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPut, c.baseURL+"/api/v1/schedules/"+url.PathEscape(name), bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.do(httpReq)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

// DeleteSchedule removes a schedule.
func (c *Client) DeleteSchedule(name string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, c.baseURL+"/api/v1/schedules/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

// EnableSchedule enables a schedule.
func (c *Client) EnableSchedule(name string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.baseURL+"/api/v1/schedules/"+url.PathEscape(name)+"/enable", nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

// DisableSchedule disables a schedule.
func (c *Client) DisableSchedule(name string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.baseURL+"/api/v1/schedules/"+url.PathEscape(name)+"/disable", nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}
