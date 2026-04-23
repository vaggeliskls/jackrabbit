package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Executor runs commands and manages their lifecycle
type Executor struct {
	logger zerolog.Logger
	mu     sync.Mutex
	active map[string]*execution
}

type execution struct {
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	stdoutPipe io.ReadCloser
	stderrPipe io.ReadCloser
	startedAt  time.Time
}

// Result contains the outcome of a command execution
type Result struct {
	ExitCode     int
	ErrorMessage string
	StartedAt    time.Time
	FinishedAt   time.Time
}

// LogLine represents a single line of output
type LogLine struct {
	Source string // "stdout" or "stderr"
	Line   string
	Time   time.Time
}

func NewExecutor(logger zerolog.Logger) *Executor {
	return &Executor{
		logger: logger.With().Str("component", "executor").Logger(),
		active: make(map[string]*execution),
	}
}

// Execute runs a command and streams its output
func (e *Executor) Execute(ctx context.Context, commandID string, cmdStr string, timeout time.Duration) (<-chan LogLine, <-chan Result, error) {
	e.mu.Lock()
	if _, exists := e.active[commandID]; exists {
		e.mu.Unlock()
		return nil, nil, fmt.Errorf("command already running: %s", commandID)
	}
	e.mu.Unlock()

	logCh := make(chan LogLine, 100)
	resultCh := make(chan Result, 1)

	// Create cancellable context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)

	// Parse command (simple shell execution)
	cmd := exec.CommandContext(execCtx, "sh", "-c", cmdStr)
	
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	exec := &execution{
		cmd:        cmd,
		cancel:     cancel,
		stdoutPipe: stdoutPipe,
		stderrPipe: stderrPipe,
		startedAt:  time.Now(),
	}

	e.mu.Lock()
	e.active[commandID] = exec
	e.mu.Unlock()

	// Start the command
	if err := cmd.Start(); err != nil {
		e.mu.Lock()
		delete(e.active, commandID)
		e.mu.Unlock()
		cancel()
		return nil, nil, fmt.Errorf("failed to start command: %w", err)
	}

	e.logger.Info().
		Str("command_id", commandID).
		Str("command", cmdStr).
		Msg("command started")

	// Stream output
	go e.streamOutput(commandID, stdoutPipe, "stdout", logCh)
	go e.streamOutput(commandID, stderrPipe, "stderr", logCh)

	// Wait for completion
	go func() {
		defer close(logCh)
		defer close(resultCh)
		defer cancel()

		result := Result{
			StartedAt:  exec.startedAt,
			FinishedAt: time.Now(),
		}

		err := cmd.Wait()
		
		e.mu.Lock()
		delete(e.active, commandID)
		e.mu.Unlock()

		result.FinishedAt = time.Now()

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				result.ExitCode = 124 // timeout exit code
				result.ErrorMessage = "command timed out"
			} else if ctx.Err() == context.Canceled {
				result.ExitCode = 130 // SIGINT exit code
				result.ErrorMessage = "command cancelled"
			} else {
				// Try to extract exit code from error string
				result.ExitCode = -1
				result.ErrorMessage = err.Error()
				
				// Check if we can get exit code from process state
				if cmd.ProcessState != nil {
					result.ExitCode = cmd.ProcessState.ExitCode()
					if result.ExitCode != 0 {
						result.ErrorMessage = fmt.Sprintf("command exited with code %d", result.ExitCode)
					}
				}
			}
		} else {
			result.ExitCode = 0
		}

		e.logger.Info().
			Str("command_id", commandID).
			Int("exit_code", result.ExitCode).
			Dur("duration", result.FinishedAt.Sub(result.StartedAt)).
			Msg("command completed")

		resultCh <- result
	}()

	return logCh, resultCh, nil
}

// Kill terminates a running command
func (e *Executor) Kill(commandID string) error {
	e.mu.Lock()
	exec, exists := e.active[commandID]
	e.mu.Unlock()

	if !exists {
		return fmt.Errorf("command not found: %s", commandID)
	}

	e.logger.Info().Str("command_id", commandID).Msg("killing command")

	// Cancel context to trigger graceful shutdown
	exec.cancel()

	// Give it a moment, then force kill if needed
	time.AfterFunc(2*time.Second, func() {
		if exec.cmd.Process != nil {
			exec.cmd.Process.Signal(os.Kill)
		}
	})

	return nil
}

// streamOutput reads from a pipe and sends lines to the log channel
func (e *Executor) streamOutput(commandID string, pipe io.ReadCloser, source string, logCh chan<- LogLine) {
	defer pipe.Close()

	buf := make([]byte, 4096)
	var incomplete []byte

	for {
		n, err := pipe.Read(buf)
		if n > 0 {
			data := append(incomplete, buf[:n]...)
			lines := splitLines(data)

			for i, line := range lines {
				// Last element might be incomplete
				if i == len(lines)-1 && !endsWithNewline(data) {
					incomplete = []byte(line)
					break
				}

				if line != "" {
					select {
					case logCh <- LogLine{
						Source: source,
						Line:   line,
						Time:   time.Now(),
					}:
					default:
						// Channel full, drop log
						e.logger.Warn().
							Str("command_id", commandID).
							Msg("log channel full, dropping output")
					}
				}
			}
		}

		if err != nil {
			if err != io.EOF {
				e.logger.Error().
					Err(err).
					Str("command_id", commandID).
					Str("source", source).
					Msg("error reading output")
			}
			break
		}
	}

	// Send any remaining incomplete line
	if len(incomplete) > 0 {
		logCh <- LogLine{
			Source: source,
			Line:   string(incomplete),
			Time:   time.Now(),
		}
	}
}

// splitLines splits data on newlines
func splitLines(data []byte) []string {
	var lines []string
	start := 0

	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}

	// Add remaining data (might be incomplete line)
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}

	return lines
}

// endsWithNewline checks if data ends with a newline
func endsWithNewline(data []byte) bool {
	return len(data) > 0 && data[len(data)-1] == '\n'
}

// IsRunning checks if a command is currently executing
func (e *Executor) IsRunning(commandID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, exists := e.active[commandID]
	return exists
}

// ActiveCount returns the number of currently executing commands
func (e *Executor) ActiveCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.active)
}

// GetPID returns the OS PID of a running command's process.
func (e *Executor) GetPID(commandID string) (int32, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ex, exists := e.active[commandID]
	if !exists || ex.cmd.Process == nil {
		return 0, false
	}
	return int32(ex.cmd.Process.Pid), true
}
