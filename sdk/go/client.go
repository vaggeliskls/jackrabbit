package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("resource not found")
	ErrConflict = errors.New("resource conflict")
	ErrServer   = errors.New("server error")
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

type Option func(*Client)

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

func WithToken(token string) Option {
	return func(c *Client) {
		c.token = token
	}
}

func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

type RunnerConfig struct {
	Slug             string
	Name             string
	Tags             []string
	ConcurrencyLimit int
	GPUCapable       bool
}

func (c *Client) RegisterRunner(ctx context.Context, config RunnerConfig) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"slug":              config.Slug,
		"name":              config.Name,
		"tags":              config.Tags,
		"concurrency_limit": config.ConcurrencyLimit,
		"gpu_capable":       config.GPUCapable,
	}

	var runner map[string]interface{}
	if err := c.doRequest(ctx, "POST", "/api/v1/runners/register", body, &runner); err != nil {
		return nil, err
	}

	return runner, nil
}

func (c *Client) DeregisterRunner(ctx context.Context, slug string) error {
	return c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/v1/runners/%s", slug), nil, nil)
}

func (c *Client) Heartbeat(ctx context.Context, slug string) error {
	return c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/runners/%s/heartbeat", slug), nil, nil)
}

type RunnerFilter struct {
	Status string
	Tags   []string
}

func (c *Client) ListRunners(ctx context.Context, filter *RunnerFilter) ([]map[string]interface{}, error) {
	url := "/api/v1/runners"
	if filter != nil {
		params := make([]string, 0)
		if filter.Status != "" {
			params = append(params, fmt.Sprintf("status=%s", filter.Status))
		}
		for _, tag := range filter.Tags {
			params = append(params, fmt.Sprintf("tags=%s", tag))
		}
		if len(params) > 0 {
			url += "?" + strings.Join(params, "&")
		}
	}

	var runners []map[string]interface{}
	if err := c.doRequest(ctx, "GET", url, nil, &runners); err != nil {
		return nil, err
	}

	return runners, nil
}

func (c *Client) GetRunner(ctx context.Context, slug string) (map[string]interface{}, error) {
	var runner map[string]interface{}
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/runners/%s", slug), nil, &runner); err != nil {
		return nil, err
	}

	return runner, nil
}

type CommandRequest struct {
	TargetType  string
	TargetValue string
	Payload     map[string]interface{}
	MaxRetries  int
	TimeoutSecs int
	Deadline    *time.Time
}

func (c *Client) SendCommand(ctx context.Context, req CommandRequest) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"target_type":  req.TargetType,
		"target_value": req.TargetValue,
		"payload":      req.Payload,
		"max_retries":  req.MaxRetries,
		"timeout_secs": req.TimeoutSecs,
		"deadline":     req.Deadline,
	}

	var cmd map[string]interface{}
	if err := c.doRequest(ctx, "POST", "/api/v1/commands", body, &cmd); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (c *Client) GetCommand(ctx context.Context, id string) (map[string]interface{}, error) {
	var cmd map[string]interface{}
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/commands/%s", id), nil, &cmd); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (c *Client) KillCommand(ctx context.Context, id string) error {
	return c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/commands/%s/kill", id), nil, nil)
}

func (c *Client) ReportResult(ctx context.Context, slug, commandID string, exitCode int, errorMessage string) error {
	status := "success"
	if exitCode != 0 {
		status = "failed"
	}
	body := map[string]interface{}{
		"command_id":    commandID,
		"status":        status,
		"exit_code":     exitCode,
		"error_message": errorMessage,
	}
	return c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/runners/%s/result", slug), body, nil)
}

func (c *Client) BatchInsertLogs(ctx context.Context, slug string, logs []map[string]interface{}) error {
	body := map[string]interface{}{"logs": logs}
	return c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/runners/%s/logs", slug), body, nil)
}

func (c *Client) BatchInsertMetrics(ctx context.Context, slug string, metrics []map[string]interface{}) error {
	body := map[string]interface{}{"metrics": metrics}
	return c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/runners/%s/metrics", slug), body, nil)
}

func (c *Client) GetLogs(ctx context.Context, commandID string, page, pageSize int) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("/api/v1/commands/%s/logs?page=%d&page_size=%d", commandID, page, pageSize)
	var logs []map[string]interface{}
	if err := c.doRequest(ctx, "GET", url, nil, &logs); err != nil {
		return nil, err
	}

	return logs, nil
}

type MetricOpts struct {
	Resolution string
	Page       int
	PageSize   int
}

func (c *Client) GetMetrics(ctx context.Context, commandID string, opts *MetricOpts) ([]map[string]interface{}, error) {
	if opts == nil {
		opts = &MetricOpts{Resolution: "raw", Page: 1, PageSize: 100}
	}

	url := fmt.Sprintf("/api/v1/commands/%s/metrics?resolution=%s&page=%d&page_size=%d",
		commandID, opts.Resolution, opts.Page, opts.PageSize)

	var metrics []map[string]interface{}
	if err := c.doRequest(ctx, "GET", url, nil, &metrics); err != nil {
		return nil, err
	}

	return metrics, nil
}

type CommandEvent struct {
	Type string
	Data map[string]interface{}
}

func (c *Client) WatchCommands(ctx context.Context, slug string) (<-chan CommandEvent, error) {
	eventCh := make(chan CommandEvent, 10)

	go func() {
		defer close(eventCh)

		backoff := time.Second
		maxBackoff := 30 * time.Second

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if err := c.streamSSE(ctx, slug, eventCh); err != nil {
				if ctx.Err() != nil {
					return
				}

				time.Sleep(backoff)
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}

			backoff = time.Second
		}
	}()

	return eventCh, nil
}

func (c *Client) streamSSE(ctx context.Context, slug string, eventCh chan<- CommandEvent) error {
	url := fmt.Sprintf("%s/api/v1/runners/%s/sse", c.baseURL, slug)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE connection failed: %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventType string
	var eventData strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			eventData.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		} else if line == "" {
			if eventType != "" && eventData.Len() > 0 {
				var data map[string]interface{}
				if err := json.Unmarshal([]byte(eventData.String()), &data); err == nil {
					select {
					case eventCh <- CommandEvent{Type: eventType, Data: data}:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				eventType = ""
				eventData.Reset()
			}
		}
	}

	return scanner.Err()
}

func (c *Client) WatchLogs(ctx context.Context, commandID string) (<-chan map[string]interface{}, error) {
	logCh := make(chan map[string]interface{}, 100)

	go func() {
		defer close(logCh)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		lastSeq := int64(0)
		page := 1

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logs, err := c.GetLogs(ctx, commandID, page, 100)
				if err != nil {
					continue
				}

				for _, log := range logs {
					if seq, ok := log["seq"].(float64); ok && int64(seq) > lastSeq {
						select {
						case logCh <- log:
							lastSeq = int64(seq)
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
	}()

	return logCh, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body, result interface{}) error {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = strings.NewReader(string(jsonData))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			switch resp.StatusCode {
			case http.StatusNotFound:
				return ErrNotFound
			case http.StatusConflict:
				return ErrConflict
			default:
				if errMsg, ok := errResp["error"].(string); ok {
					return fmt.Errorf("%w: %s", ErrServer, errMsg)
				}
				return ErrServer
			}
		}
		return fmt.Errorf("request failed: %d", resp.StatusCode)
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return err
		}
	}

	return nil
}
