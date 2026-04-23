package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// SSEListener listens for command dispatch events via Server-Sent Events
type SSEListener struct {
	serverURL   string
	runnerSlug  string
	apiToken    string
	logger      zerolog.Logger
	httpClient  *http.Client
	eventCh     chan CommandEvent
	reconnectCh chan struct{}
}

// CommandEvent represents a command dispatch event
type CommandEvent struct {
	CommandID   string
	Payload     map[string]interface{}
	TimeoutSecs int
}

func NewSSEListener(serverURL, runnerSlug, apiToken string, logger zerolog.Logger) *SSEListener {
	return &SSEListener{
		serverURL:   serverURL,
		runnerSlug:  runnerSlug,
		apiToken:    apiToken,
		logger:      logger.With().Str("component", "sse").Logger(),
		httpClient:  &http.Client{Timeout: 0}, // No timeout for SSE
		eventCh:     make(chan CommandEvent, 10),
		reconnectCh: make(chan struct{}, 1),
	}
}

// Start begins listening for SSE events
func (s *SSEListener) Start(ctx context.Context) <-chan CommandEvent {
	go s.listen(ctx)
	return s.eventCh
}

// Reconnect triggers a manual reconnection
func (s *SSEListener) Reconnect() {
	select {
	case s.reconnectCh <- struct{}{}:
	default:
	}
}

func (s *SSEListener) listen(ctx context.Context) {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			close(s.eventCh)
			return
		default:
		}

		err := s.connectAndListen(ctx)
		if err != nil {
			if ctx.Err() != nil {
				close(s.eventCh)
				return
			}

			s.logger.Error().Err(err).Msg("SSE connection error, reconnecting")

			select {
			case <-time.After(backoff):
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			case <-ctx.Done():
				close(s.eventCh)
				return
			}
		} else {
			backoff = time.Second // Reset backoff on successful connection
		}
	}
}

func (s *SSEListener) connectAndListen(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/runners/%s/sse", s.serverURL, s.runnerSlug)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if s.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiToken)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect to SSE: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE connection failed: %d", resp.StatusCode)
	}

	s.logger.Info().Msg("SSE connection established")

	return s.readEvents(ctx, resp.Body)
}

func (s *SSEListener) readEvents(ctx context.Context, body io.ReadCloser) error {
	scanner := bufio.NewScanner(body)
	var eventType string
	var eventData strings.Builder

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.reconnectCh:
			return fmt.Errorf("manual reconnect requested")
		default:
		}

		line := scanner.Text()

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			eventData.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		} else if line == "" {
			if eventType == "command_dispatch" && eventData.Len() > 0 {
				event, err := s.parseCommandEvent(eventData.String())
				if err != nil {
					s.logger.Error().Err(err).Msg("failed to parse command event")
				} else {
					select {
					case s.eventCh <- event:
						s.logger.Info().
							Str("command_id", event.CommandID).
							Msg("received command dispatch")
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			} else if eventType == "ping" {
				s.logger.Debug().Msg("received ping")
			}
			eventType = ""
			eventData.Reset()
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return fmt.Errorf("SSE connection closed")
}

func (s *SSEListener) parseCommandEvent(data string) (CommandEvent, error) {
	var event struct {
		CommandID   string                 `json:"command_id"`
		Payload     map[string]interface{} `json:"payload"`
		TimeoutSecs int                    `json:"timeout_secs"`
	}

	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return CommandEvent{}, fmt.Errorf("unmarshal event: %w", err)
	}

	return CommandEvent{
		CommandID:   event.CommandID,
		Payload:     event.Payload,
		TimeoutSecs: event.TimeoutSecs,
	}, nil
}
