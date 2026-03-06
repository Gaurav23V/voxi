package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/Gaurav23V/voxi/internal/xruntime"
)

type PWRecorder struct {
	Command string

	mu       sync.Mutex
	cmd      *exec.Cmd
	filePath string
}

func NewPWRecorder() *PWRecorder {
	command := os.Getenv("VOXI_RECORD_COMMAND")
	if command == "" {
		command = "pw-record"
	}
	return &PWRecorder{Command: command}
}

func (r *PWRecorder) Start(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cmd != nil {
		return fmt.Errorf("recording already active")
	}

	if err := xruntime.EnsureRuntimeDir(); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(xruntime.RuntimeDir(), "recording-*.wav")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	cmd := exec.Command(r.Command, "--rate=16000", "--channels=1", tempFile.Name())
	if err := cmd.Start(); err != nil {
		_ = os.Remove(tempFile.Name())
		return fmt.Errorf("start recorder: %w", err)
	}
	r.cmd = cmd
	r.filePath = tempFile.Name()
	return nil
}

func (r *PWRecorder) Stop(ctx context.Context) (Capture, error) {
	r.mu.Lock()
	cmd := r.cmd
	filePath := r.filePath
	r.cmd = nil
	r.filePath = ""
	r.mu.Unlock()

	if cmd == nil {
		return Capture{}, errors.New("recording not active")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return Capture{}, fmt.Errorf("stop recorder process: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		if err != nil && !isExpectedStopError(err) {
			return Capture{}, fmt.Errorf("wait for recorder: %w", err)
		}
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return Capture{}, fmt.Errorf("recording stop timeout: %w", ctx.Err())
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return Capture{}, fmt.Errorf("read recording: %w", err)
	}
	_ = os.Remove(filePath)

	audioBytes, err := extractPCMData(content)
	if err != nil {
		return Capture{}, err
	}
	if len(audioBytes) == 0 {
		return Capture{}, errors.New("empty audio")
	}

	return Capture{
		Audio:        audioBytes,
		AudioFormat:  "pcm_s16le",
		SampleRateHz: 16000,
	}, nil
}

func extractPCMData(content []byte) ([]byte, error) {
	if len(content) < 12 || string(content[:4]) != "RIFF" || string(content[8:12]) != "WAVE" {
		return content, nil
	}

	offset := 12
	for offset+8 <= len(content) {
		chunkID := string(content[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(content[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := chunkStart + chunkSize
		if chunkEnd > len(content) {
			return nil, errors.New("invalid wav data chunk")
		}

		if chunkID == "data" {
			return content[chunkStart:chunkEnd], nil
		}

		offset = chunkEnd
		if chunkSize%2 == 1 {
			offset++
		}
	}

	return nil, errors.New("wav data chunk not found")
}

func isExpectedStopError(err error) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signal() == os.Interrupt {
			return true
		}
	}
	return strings.Contains(strings.ToLower(err.Error()), "interrupt")
}
