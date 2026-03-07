package audio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestIsExpectedStopErrorExitStatusOne(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 1").Run()
	if err == nil {
		t.Fatal("expected command error, got nil")
	}
	if !isExpectedStopError(err) {
		t.Fatalf("isExpectedStopError(%v) = false, want true", err)
	}
}

func TestIsExpectedStopErrorExitStatusTwo(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 2").Run()
	if err == nil {
		t.Fatal("expected command error, got nil")
	}
	if isExpectedStopError(err) {
		t.Fatalf("isExpectedStopError(%v) = true, want false", err)
	}
}

func TestPWRecorderStopAcceptsExitStatusOneWhenAudioExists(t *testing.T) {
	t.Setenv("VOXI_RUNTIME_DIR", t.TempDir())

	command := filepath.Join(t.TempDir(), "fake-pw-record.sh")
	script := `#!/usr/bin/env bash
set -euo pipefail
out="$3"
[ -n "${out}" ]
printf 'abcd' > "${out}"
trap 'exit 1' INT
while true; do
  sleep 1
done
`
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	recorder := &PWRecorder{Command: command}
	if err := recorder.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	capture, err := recorder.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(capture.Audio) == 0 {
		t.Fatal("capture audio is empty, want non-empty")
	}
	if capture.AudioFormat != "pcm_s16le" {
		t.Fatalf("AudioFormat = %q, want %q", capture.AudioFormat, "pcm_s16le")
	}
	if capture.SampleRateHz != 16000 {
		t.Fatalf("SampleRateHz = %d, want %d", capture.SampleRateHz, 16000)
	}
}
