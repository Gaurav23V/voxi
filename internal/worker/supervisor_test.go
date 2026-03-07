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

	healthCtx, healthCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer healthCancel()
	if _, err := supervisor.Health(healthCtx, "health-after-expired-context"); err != nil {
		t.Fatalf("Health() error after expired context = %v", err)
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
