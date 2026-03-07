package worker

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gaurav23V/voxi/internal/audio"
	"github.com/Gaurav23V/voxi/internal/config"
	"github.com/Gaurav23V/voxi/internal/logging"
)

func TestSupervisorRestartsWorkerAfterCrash(t *testing.T) {
	t.Setenv("VOXI_WORKER_MODE", "fake")
	t.Setenv("VOXI_FAKE_ASR_TRANSCRIPT", "hello test")
	t.Setenv("VOXI_FAKE_CLEANUP_TEXT", "Hello test.")

	cfg := config.Default()
	cfg.WorkerHealthTimeout = 5000

	supervisor := NewSupervisor(cfg, filepath.Join(t.TempDir(), "worker.sock"), logging.NewForWriter(testWriter{t}))
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = supervisor.Stop()
	}()

	supervisor.mu.Lock()
	cmd := supervisor.cmd
	supervisor.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		t.Fatalf("worker process not started")
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		supervisor.mu.Lock()
		stopped := supervisor.cmd == nil
		supervisor.mu.Unlock()
		if stopped {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	result, err := supervisor.TranscribeAndClean(context.Background(), "job-1", audio.Capture{
		Audio:        []byte("abcd"),
		AudioFormat:  "pcm_s16le",
		SampleRateHz: 16000,
	})
	if err != nil {
		t.Fatalf("TranscribeAndClean() error = %v", err)
	}

	if result.Transcript != "hello test" {
		t.Fatalf("Transcript = %q, want %q", result.Transcript, "hello test")
	}
	if result.Cleaned != "Hello test." {
		t.Fatalf("Cleaned = %q, want %q", result.Cleaned, "Hello test.")
	}
}

func TestTranscribeAndCleanExpiredContextDoesNotMaskWithHealthTimeout(t *testing.T) {
	t.Setenv("VOXI_WORKER_MODE", "fake")
	t.Setenv("VOXI_FAKE_ASR_TRANSCRIPT", "hello test")
	t.Setenv("VOXI_FAKE_CLEANUP_TEXT", "Hello test.")

	cfg := config.Default()
	cfg.WorkerHealthTimeout = 3000

	supervisor := NewSupervisor(cfg, filepath.Join(t.TempDir(), "worker.sock"), logging.NewForWriter(testWriter{t}))
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = supervisor.Stop()
	}()

	beforePID := workerPID(supervisor)
	if beforePID == 0 {
		t.Fatalf("worker PID before timeout = 0")
	}

	callCtx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	time.Sleep(time.Millisecond)
	cancel()

	_, err := supervisor.TranscribeAndClean(callCtx, "job-expired-context", audio.Capture{
		Audio:        []byte("abcd"),
		AudioFormat:  "pcm_s16le",
		SampleRateHz: 16000,
	})
	if err == nil {
		t.Fatalf("TranscribeAndClean() error = nil, want timeout-related error")
	}

	if strings.Contains(strings.ToLower(err.Error()), "worker health check timed out") {
		t.Fatalf("TranscribeAndClean() error = %q, should preserve original request timeout", err.Error())
	}

	time.Sleep(150 * time.Millisecond)
	afterPID := workerPID(supervisor)
	if afterPID != beforePID {
		t.Fatalf("worker restarted on timeout; PID before=%d after=%d", beforePID, afterPID)
	}

	healthCtx, healthCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer healthCancel()
	if _, err := supervisor.Health(healthCtx, "health-after-expired-context"); err != nil {
		t.Fatalf("Health() error after expired context = %v", err)
	}
}

func TestTranscribeAndCleanWorkerErrorDoesNotRestartWorker(t *testing.T) {
	t.Setenv("VOXI_WORKER_MODE", "fake")
	t.Setenv("VOXI_FAKE_ASR_BEHAVIOR", "unavailable")

	cfg := config.Default()
	cfg.WorkerHealthTimeout = 3000

	supervisor := NewSupervisor(cfg, filepath.Join(t.TempDir(), "worker.sock"), logging.NewForWriter(testWriter{t}))
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = supervisor.Stop()
	}()

	beforePID := workerPID(supervisor)
	if beforePID == 0 {
		t.Fatalf("worker PID before request = 0")
	}

	result, err := supervisor.TranscribeAndClean(context.Background(), "job-worker-error", audio.Capture{
		Audio:        []byte("abcd"),
		AudioFormat:  "pcm_s16le",
		SampleRateHz: 16000,
	})
	if err == nil {
		t.Fatalf("TranscribeAndClean() error = nil, want worker response error")
	}
	if result.Code != "ASR_MODEL_UNAVAILABLE" {
		t.Fatalf("result.Code = %q, want ASR_MODEL_UNAVAILABLE", result.Code)
	}

	time.Sleep(150 * time.Millisecond)
	afterPID := workerPID(supervisor)
	if afterPID != beforePID {
		t.Fatalf("worker restarted on worker response error; PID before=%d after=%d", beforePID, afterPID)
	}
}

func workerPID(supervisor *Supervisor) int {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	if supervisor.cmd == nil || supervisor.cmd.Process == nil {
		return 0
	}
	return supervisor.cmd.Process.Pid
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
