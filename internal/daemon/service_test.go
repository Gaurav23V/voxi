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

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
