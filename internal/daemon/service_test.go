package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Gaurav23V/voxi/internal/audio"
	"github.com/Gaurav23V/voxi/internal/config"
	"github.com/Gaurav23V/voxi/internal/logging"
	"github.com/Gaurav23V/voxi/internal/output"
	"github.com/Gaurav23V/voxi/internal/state"
	"github.com/Gaurav23V/voxi/internal/worker"
)

type fakeRecorder struct {
	startCalled int
	stopCalled  int
	capture     audio.Capture
	startErr    error
	stopErr     error
}

func (f *fakeRecorder) Start(context.Context) error {
	f.startCalled++
	return f.startErr
}

func (f *fakeRecorder) Stop(context.Context) (audio.Capture, error) {
	f.stopCalled++
	return f.capture, f.stopErr
}

type fakeWorker struct {
	result worker.Result
	err    error
}

func (f fakeWorker) TranscribeAndClean(context.Context, string, audio.Capture) (worker.Result, error) {
	return f.result, f.err
}

func (f fakeWorker) Health(context.Context, string) (worker.Health, error) {
	return worker.Health{}, nil
}

type sequenceWorker struct {
	mu      sync.Mutex
	results []worker.Result
	errors  []error
	calls   int
}

func (s *sequenceWorker) TranscribeAndClean(context.Context, string, audio.Capture) (worker.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.calls
	s.calls++

	var result worker.Result
	if index < len(s.results) {
		result = s.results[index]
	}

	var err error
	if index < len(s.errors) {
		err = s.errors[index]
	}

	return result, err
}

func (s *sequenceWorker) Health(context.Context, string) (worker.Health, error) {
	return worker.Health{}, nil
}

type fakeInserter struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeInserter) Insert(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.err
}

type fakeClipboard struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeClipboard) Copy(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.err
}

type fakeNotifier struct {
	mu       sync.Mutex
	messages []string
}

func (f *fakeNotifier) Notify(_ context.Context, title, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, title+"|"+body)
	return nil
}

func (f *fakeNotifier) contains(target string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, message := range f.messages {
		if message == target {
			return true
		}
	}
	return false
}

func TestToggleStartsRecording(t *testing.T) {
	recorder := &fakeRecorder{}
	notifier := &fakeNotifier{}

	service := NewService(
		config.Default(),
		recorder,
		fakeWorker{},
		&fakeInserter{},
		&fakeClipboard{},
		notifier,
		logging.NewForWriter(testWriter{t}),
	)

	got, err := service.Toggle(context.Background())
	if err != nil {
		t.Fatalf("Toggle() error = %v", err)
	}
	if got != state.Recording {
		t.Fatalf("Toggle() state = %s, want %s", got, state.Recording)
	}
	if recorder.startCalled != 1 {
		t.Fatalf("Start called %d times, want 1", recorder.startCalled)
	}
}

func TestToggleIgnoredWhileProcessing(t *testing.T) {
	service := NewService(
		config.Default(),
		&fakeRecorder{},
		fakeWorker{},
		&fakeInserter{},
		&fakeClipboard{},
		&fakeNotifier{},
		logging.NewForWriter(testWriter{t}),
	)
	service.machine.Toggle()
	service.machine.Toggle()

	got, err := service.Toggle(context.Background())
	if err != nil {
		t.Fatalf("Toggle() error = %v", err)
	}
	if got != state.Processing {
		t.Fatalf("Toggle() state = %s, want %s", got, state.Processing)
	}
}

func TestRecordingStopTriggersPipelineAndResetsIdle(t *testing.T) {
	recorder := &fakeRecorder{
		capture: audio.Capture{
			Audio:        []byte("test"),
			AudioFormat:  "pcm_s16le",
			SampleRateHz: 16000,
		},
	}
	inserter := &fakeInserter{}
	service := NewService(
		config.Default(),
		recorder,
		fakeWorker{result: worker.Result{Transcript: "hello", Cleaned: "Hello."}},
		inserter,
		&fakeClipboard{},
		&fakeNotifier{},
		logging.NewForWriter(testWriter{t}),
	)

	if _, err := service.Toggle(context.Background()); err != nil {
		t.Fatalf("first Toggle() error = %v", err)
	}
	if _, err := service.Toggle(context.Background()); err != nil {
		t.Fatalf("second Toggle() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if service.Status() == state.Idle {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("service did not return to Idle, current=%s insertCalls=%d", service.Status(), inserter.calls)
}

func TestASRTransientFailureRetriesOnce(t *testing.T) {
	recorder := &fakeRecorder{
		capture: audio.Capture{
			Audio:        []byte("test"),
			AudioFormat:  "pcm_s16le",
			SampleRateHz: 16000,
		},
	}
	notifier := &fakeNotifier{}
	workerClient := &sequenceWorker{
		results: []worker.Result{
			{Stage: "speech_recognition", Code: "ASR_TIMEOUT", Message: "inference exceeded timeout"},
			{Transcript: "hello", Cleaned: "Hello."},
		},
		errors: []error{assertErr("timeout"), nil},
	}

	service := NewService(
		config.Default(),
		recorder,
		workerClient,
		&fakeInserter{},
		&fakeClipboard{},
		notifier,
		logging.NewForWriter(testWriter{t}),
	)

	if _, err := service.Toggle(context.Background()); err != nil {
		t.Fatalf("first Toggle() error = %v", err)
	}
	if _, err := service.Toggle(context.Background()); err != nil {
		t.Fatalf("second Toggle() error = %v", err)
	}

	waitForIdle(t, service)
	if workerClient.calls != 2 {
		t.Fatalf("worker calls = %d, want 2", workerClient.calls)
	}
}

func TestPermanentASRFailureSurfacesTranscriptionFailed(t *testing.T) {
	recorder := &fakeRecorder{
		capture: audio.Capture{
			Audio:        []byte("test"),
			AudioFormat:  "pcm_s16le",
			SampleRateHz: 16000,
		},
	}
	notifier := &fakeNotifier{}
	workerClient := &sequenceWorker{
		results: []worker.Result{
			{Stage: "speech_recognition", Code: "ASR_TIMEOUT", Message: "inference exceeded timeout"},
			{Stage: "speech_recognition", Code: "ASR_TIMEOUT", Message: "inference exceeded timeout"},
		},
		errors: []error{assertErr("timeout"), assertErr("timeout")},
	}

	service := NewService(
		config.Default(),
		recorder,
		workerClient,
		&fakeInserter{},
		&fakeClipboard{},
		notifier,
		logging.NewForWriter(testWriter{t}),
	)

	if _, err := service.Toggle(context.Background()); err != nil {
		t.Fatalf("first Toggle() error = %v", err)
	}
	if _, err := service.Toggle(context.Background()); err != nil {
		t.Fatalf("second Toggle() error = %v", err)
	}

	waitForIdle(t, service)
	if !notifier.contains("Transcription failed|Stage: Speech recognition (inference exceeded timeout)") {
		t.Fatalf("expected transcription failure notification, got %v", notifier.messages)
	}
}

func TestCleanupTransientFailureRetriesOnce(t *testing.T) {
	recorder := &fakeRecorder{
		capture: audio.Capture{
			Audio:        []byte("test"),
			AudioFormat:  "pcm_s16le",
			SampleRateHz: 16000,
		},
	}
	workerClient := &sequenceWorker{
		results: []worker.Result{
			{Stage: "text_cleanup", Code: "LLM_TIMEOUT", Message: "request exceeded timeout"},
			{Transcript: "hello", Cleaned: "Hello."},
		},
		errors: []error{assertErr("timeout"), nil},
	}

	service := NewService(
		config.Default(),
		recorder,
		workerClient,
		&fakeInserter{},
		&fakeClipboard{},
		&fakeNotifier{},
		logging.NewForWriter(testWriter{t}),
	)

	if _, err := service.Toggle(context.Background()); err != nil {
		t.Fatalf("first Toggle() error = %v", err)
	}
	if _, err := service.Toggle(context.Background()); err != nil {
		t.Fatalf("second Toggle() error = %v", err)
	}

	waitForIdle(t, service)
	if workerClient.calls != 2 {
		t.Fatalf("worker calls = %d, want 2", workerClient.calls)
	}
}

func TestInsertWithFallbackSkipsRetryWhenWTypeUnsupported(t *testing.T) {
	inserter := &fakeInserter{
		err: errors.New("wtype failed: exit status 1: Compositor does not support the virtual keyboard protocol"),
	}
	clipboard := &fakeClipboard{}
	notifier := &fakeNotifier{}
	service := NewService(
		config.Default(),
		&fakeRecorder{},
		fakeWorker{},
		inserter,
		clipboard,
		notifier,
		logging.NewForWriter(testWriter{t}),
	)

	if err := service.insertWithFallback(context.Background(), "job-unsupported", "hello world"); err != nil {
		t.Fatalf("insertWithFallback() error = %v", err)
	}

	if inserter.calls != 1 {
		t.Fatalf("wtype insert calls = %d, want 1 (skip retry on non-retryable error)", inserter.calls)
	}
	if clipboard.calls != 1 {
		t.Fatalf("clipboard copy calls = %d, want 1", clipboard.calls)
	}
	if !notifier.contains("Copied to clipboard|Direct typing is unavailable on this compositor. Press Ctrl+Shift+V to paste.") {
		t.Fatalf("expected compositor-specific clipboard notification, got %v", notifier.messages)
	}
}

func TestRecordingStopReturnsIdleWhenClipboardHelperStaysAlive(t *testing.T) {
	recorder := &fakeRecorder{
		capture: audio.Capture{
			Audio:        []byte("test"),
			AudioFormat:  "pcm_s16le",
			SampleRateHz: 16000,
		},
	}
	inserter := &fakeInserter{
		err: errors.New("wtype failed: exit status 1: Compositor does not support the virtual keyboard protocol"),
	}
	notifier := &fakeNotifier{}
	clipboardOutput := filepath.Join(t.TempDir(), "clipboard.txt")
	clipboardCommand := writeClipboardHelperScript(t, clipboardOutput, 1200*time.Millisecond)

	service := NewService(
		config.Default(),
		recorder,
		fakeWorker{result: worker.Result{Transcript: "hello", Cleaned: "Hello."}},
		inserter,
		&output.WLCopyClipboard{Command: clipboardCommand},
		notifier,
		logging.NewForWriter(testWriter{t}),
	)

	if _, err := service.Toggle(context.Background()); err != nil {
		t.Fatalf("first Toggle() error = %v", err)
	}
	if _, err := service.Toggle(context.Background()); err != nil {
		t.Fatalf("second Toggle() error = %v", err)
	}

	started := time.Now()
	deadline := started.Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if service.Status() == state.Idle {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if service.Status() != state.Idle {
		t.Fatalf("service did not return to Idle while clipboard helper was still running, current=%s", service.Status())
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("service returned to Idle after %s, want well before clipboard helper exit", elapsed)
	}
	if !notifier.contains("Copied to clipboard|Direct typing is unavailable on this compositor. Press Ctrl+Shift+V to paste.") {
		t.Fatalf("expected clipboard fallback notification, got %v", notifier.messages)
	}

	fileDeadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(fileDeadline) {
		content, err := os.ReadFile(clipboardOutput)
		if err == nil && string(content) == "Hello." {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	content, err := os.ReadFile(clipboardOutput)
	if err != nil {
		t.Fatalf("clipboard helper did not write output: %v", err)
	}
	t.Fatalf("clipboard helper output = %q, want %q", string(content), "Hello.")
}

func TestNotifyReturnsWhenNotifierBlocks(t *testing.T) {
	service := NewService(
		config.Default(),
		&fakeRecorder{},
		fakeWorker{},
		&fakeInserter{},
		&fakeClipboard{},
		&blockingNotifier{},
		logging.NewForWriter(testWriter{t}),
	)

	started := time.Now()
	service.notify(context.Background(), "title", "body")
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("notify() elapsed = %s, want <= 3s", elapsed)
	}
}

func TestIsNonRetryableWTypeError(t *testing.T) {
	if !isNonRetryableWTypeError(errors.New("Compositor does not support the virtual keyboard protocol")) {
		t.Fatalf("expected non-retryable classification for compositor protocol error")
	}
	if isNonRetryableWTypeError(errors.New("wtype failed: transient focus error")) {
		t.Fatalf("did not expect non-retryable classification for generic wtype failure")
	}
}

type blockingNotifier struct{}

func (b *blockingNotifier) Notify(ctx context.Context, _ string, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}

func waitForIdle(t *testing.T, service *Service) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if service.Status() == state.Idle {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("service did not return to Idle, current=%s", service.Status())
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}

func writeClipboardHelperScript(t *testing.T, outputPath string, sleepFor time.Duration) string {
	t.Helper()

	scriptPath := filepath.Join(t.TempDir(), "fake-wl-copy.sh")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
cat > %q
sleep %.3f
`, outputPath, sleepFor.Seconds())

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}
	return scriptPath
}

// delayedWorker sleeps then returns success; used to verify timeout floors.
type delayedWorker struct {
	delay time.Duration
}

func (d delayedWorker) TranscribeAndClean(ctx context.Context, _ string, _ audio.Capture) (worker.Result, error) {
	select {
	case <-time.After(d.delay):
		return worker.Result{Transcript: "ok", Cleaned: "ok"}, nil
	case <-ctx.Done():
		return worker.Result{}, ctx.Err()
	}
}

func (d delayedWorker) Health(context.Context, string) (worker.Health, error) {
	return worker.Health{}, nil
}

func TestTranscribeWithRetry_ColdStartFloorAllowsVerySlowFirstCall(t *testing.T) {
	cfg := config.Default()
	cfg.ASRTimeout = 1000
	cfg.LLMTimeout = 1000
	// Without a cold-start floor this would timeout quickly. The first call should
	// tolerate a long model warm-up.
	worker := delayedWorker{delay: 10 * time.Second}

	service := NewService(
		cfg,
		&fakeRecorder{capture: audio.Capture{Audio: []byte("x"), AudioFormat: "pcm_s16le", SampleRateHz: 16000}},
		worker,
		&fakeInserter{},
		&fakeClipboard{},
		nil,
		logging.NewForWriter(testWriter{t}),
	)

	_, err := service.Toggle(context.Background())
	if err != nil {
		t.Fatalf("first Toggle() error = %v", err)
	}
	_, err = service.Toggle(context.Background())
	if err != nil {
		t.Fatalf("second Toggle() error = %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if service.Status() == state.Idle {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("service did not return to Idle after 20s, current=%s", service.Status())
}
