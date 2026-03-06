package daemon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Gaurav23V/voxi/internal/audio"
	"github.com/Gaurav23V/voxi/internal/config"
	"github.com/Gaurav23V/voxi/internal/logging"
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
