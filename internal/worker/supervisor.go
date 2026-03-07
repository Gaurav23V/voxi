package worker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Gaurav23V/voxi/internal/audio"
	"github.com/Gaurav23V/voxi/internal/config"
	"github.com/Gaurav23V/voxi/internal/logging"
)

type Supervisor struct {
	cfg        config.Config
	socketPath string
	logger     logging.Logger

	mu     sync.Mutex
	cmd    *exec.Cmd
	client SocketClient
}

func NewSupervisor(cfg config.Config, socketPath string, logger logging.Logger) *Supervisor {
	return &Supervisor{
		cfg:        cfg,
		socketPath: socketPath,
		logger:     logger,
		client:     SocketClient{SocketPath: socketPath},
	}
}

func (s *Supervisor) Start(ctx context.Context) error {
	return s.ensureStarted(ctx)
}

func (s *Supervisor) Stop() error {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal worker: %w", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		stopped := s.cmd == nil
		s.mu.Unlock()
		if stopped {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := cmd.Process.Kill(); err != nil {
		return fmt.Errorf("kill worker after timeout: %w", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		stopped := s.cmd == nil
		s.mu.Unlock()
		if stopped {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("worker did not stop")
}

func (s *Supervisor) TranscribeAndClean(ctx context.Context, requestID string, capture audio.Capture) (Result, error) {
	if err := s.ensureStarted(ctx); err != nil {
		return Result{Stage: "startup", Code: "BOOT_DEP_MISSING", Message: err.Error()}, err
	}

	result, err := s.client.TranscribeAndClean(ctx, requestID, capture)
	if err == nil {
		return result, nil
	}

	s.logger.Log(logging.Event{
		Stage:     "worker",
		Result:    "retrying_after_error",
		RequestID: requestID,
		Message:   err.Error(),
	})

	restartCtx, cancel := context.WithTimeout(context.Background(), s.workerHealthTimeout())
	restartErr := s.restart(restartCtx)
	cancel()
	if restartErr != nil {
		if ctx.Err() != nil {
			return Result{}, err
		}
		return Result{Stage: "startup", Code: "BOOT_DEP_MISSING", Message: restartErr.Error()}, restartErr
	}

	if ctx.Err() != nil {
		return Result{}, err
	}

	return s.client.TranscribeAndClean(ctx, requestID, capture)
}

func (s *Supervisor) Health(ctx context.Context, requestID string) (Health, error) {
	if err := s.ensureStarted(ctx); err != nil {
		return Health{}, err
	}
	return s.client.Health(ctx, requestID)
}

func (s *Supervisor) restart(ctx context.Context) error {
	if err := s.Stop(); err != nil {
		s.logger.Log(logging.Event{Stage: "worker", Result: "stop_error", Message: err.Error()})
	}
	return s.ensureStarted(ctx)
}

func (s *Supervisor) ensureStarted(ctx context.Context) error {
	s.mu.Lock()
	if s.cmd != nil && s.cmd.Process != nil && s.cmd.ProcessState == nil {
		s.mu.Unlock()
		return nil
	}

	cmd := exec.CommandContext(context.Background(), s.cfg.WorkerPython, "-m", s.cfg.WorkerEntrypoint,
		"--socket", s.socketPath,
		"--asr-model", s.cfg.ASRModel,
		"--llm-model", s.cfg.LLMModel,
		"--ollama-url", s.cfg.OllamaURL,
	)
	cmd.Env = buildWorkerEnv(os.Environ())

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("capture worker stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("capture worker stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("start worker: %w", err)
	}

	s.cmd = cmd
	s.mu.Unlock()

	go s.capturePipe("stdout", stdout)
	go s.capturePipe("stderr", stderr)
	go s.awaitExit(cmd)

	return s.waitForHealth(ctx)
}

func (s *Supervisor) waitForHealth(ctx context.Context) error {
	deadline := time.Now().Add(s.workerHealthTimeout())
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("worker health check timed out")
		}

		healthCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		_, err := s.client.Health(healthCtx, fmt.Sprintf("health-%d", time.Now().UnixNano()))
		cancel()
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *Supervisor) workerHealthTimeout() time.Duration {
	timeout := time.Duration(s.cfg.WorkerHealthTimeout) * time.Millisecond
	if timeout <= 0 {
		return 1500 * time.Millisecond
	}
	return timeout
}

func (s *Supervisor) awaitExit(cmd *exec.Cmd) {
	err := cmd.Wait()

	s.mu.Lock()
	if s.cmd == cmd {
		s.cmd = nil
	}
	s.mu.Unlock()

	message := "worker exited cleanly"
	result := "exited"
	if err != nil {
		message = err.Error()
		result = "crashed"
	}

	s.logger.Log(logging.Event{
		Stage:   "worker",
		Result:  result,
		Message: message,
	})
}

func (s *Supervisor) capturePipe(stream string, reader io.ReadCloser) {
	defer reader.Close()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		s.logger.Log(logging.Event{
			Stage:   "worker_" + stream,
			Result:  "output",
			Message: scanner.Text(),
		})
	}
}

func buildWorkerEnv(existing []string) []string {
	env := append([]string{}, existing...)

	if workerPath := locateWorkerPythonPath(); workerPath != "" {
		env = append(env, "PYTHONPATH="+workerPath)
	}

	return env
}

func locateWorkerPythonPath() string {
	if override := os.Getenv("VOXI_WORKER_PYTHONPATH"); override != "" {
		return override
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return ""
	}

	dir := currentDir
	for {
		candidate := filepath.Join(dir, "worker", "voxi_worker")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, "worker")
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
