package worker

import (
	"context"
	"path/filepath"
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

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
