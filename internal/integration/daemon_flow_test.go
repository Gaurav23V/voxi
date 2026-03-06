package integration

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Gaurav23V/voxi/internal/audio"
	"github.com/Gaurav23V/voxi/internal/config"
	"github.com/Gaurav23V/voxi/internal/daemon"
	"github.com/Gaurav23V/voxi/internal/ipc"
	"github.com/Gaurav23V/voxi/internal/logging"
	"github.com/Gaurav23V/voxi/internal/state"
	"github.com/Gaurav23V/voxi/internal/worker"
)

func TestCLIDaemonContractToggleAndStatus(t *testing.T) {
	missingSocketPath := filepath.Join(t.TempDir(), "missing.sock")
	var toggleResponse ipc.DaemonResponse
	if err := ipc.Call(context.Background(), missingSocketPath, ipc.DaemonRequest{}, &toggleResponse); err == nil {
		t.Fatalf("expected missing socket error")
	}

	service := daemon.NewService(
		config.Default(),
		&fakeRecorder{},
		fakeWorker{result: worker.Result{Transcript: "hello", Cleaned: "Hello."}},
		&fakeInserter{},
		&fakeClipboard{},
		&fakeNotifier{},
		logging.NewForWriter(testWriter{t}),
	)

	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	server := ipc.NewServer(socketPath, service.HandleRPC)
	if err := server.Start(); err != nil {
		t.Fatalf("server.Start() error = %v", err)
	}
	defer server.Close()

	if err := ipc.Call(context.Background(), socketPath, ipc.DaemonRequest{ID: "1", Op: "toggle"}, &toggleResponse); err != nil {
		t.Fatalf("ipc.Call(toggle) error = %v", err)
	}
	if !toggleResponse.OK || toggleResponse.State != string(state.Recording) {
		t.Fatalf("toggle response = %+v", toggleResponse)
	}

	var statusResponse ipc.DaemonResponse
	if err := ipc.Call(context.Background(), socketPath, ipc.DaemonRequest{ID: "2", Op: "status"}, &statusResponse); err != nil {
		t.Fatalf("ipc.Call(status) error = %v", err)
	}
	if !statusResponse.OK || statusResponse.State != string(state.Recording) {
		t.Fatalf("status response = %+v", statusResponse)
	}
}

func TestInsertionFallbackBehavior(t *testing.T) {
	recorder := &fakeRecorder{
		capture: audio.Capture{Audio: []byte("1234"), AudioFormat: "pcm_s16le", SampleRateHz: 16000},
	}
	inserter := &fakeInserter{err: context.DeadlineExceeded}
	clipboard := &fakeClipboard{}
	notifier := &fakeNotifier{}

	service := daemon.NewService(
		config.Default(),
		recorder,
		fakeWorker{result: worker.Result{Transcript: "hello there", Cleaned: "Hello there."}},
		inserter,
		clipboard,
		notifier,
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
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if inserter.calls != 2 {
		t.Fatalf("Insert calls = %d, want 2", inserter.calls)
	}
	if clipboard.calls != 1 {
		t.Fatalf("Clipboard calls = %d, want 1", clipboard.calls)
	}
	if !notifier.contains("Copied to clipboard") {
		t.Fatalf("expected clipboard fallback notification, got %v", notifier.messages)
	}
}

func TestInsertionDoubleFailureSurfacesActionFailed(t *testing.T) {
	recorder := &fakeRecorder{
		capture: audio.Capture{Audio: []byte("1234"), AudioFormat: "pcm_s16le", SampleRateHz: 16000},
	}
	inserter := &fakeInserter{err: integrationErr("wtype failed")}
	clipboard := &fakeClipboard{err: integrationErr("clipboard failed")}
	notifier := &fakeNotifier{}

	service := daemon.NewService(
		config.Default(),
		recorder,
		fakeWorker{result: worker.Result{Transcript: "hello there", Cleaned: "Hello there."}},
		inserter,
		clipboard,
		notifier,
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
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if inserter.calls != 2 {
		t.Fatalf("Insert calls = %d, want 2", inserter.calls)
	}
	if clipboard.calls != 1 {
		t.Fatalf("Clipboard calls = %d, want 1", clipboard.calls)
	}
	if !notifier.contains("Action failed|Stage: Text insertion (clipboard failed)") {
		t.Fatalf("expected final insertion failure notification, got %v", notifier.messages)
	}
}

type fakeRecorder struct {
	capture audio.Capture
}

func (f *fakeRecorder) Start(context.Context) error {
	return nil
}

func (f *fakeRecorder) Stop(context.Context) (audio.Capture, error) {
	return f.capture, nil
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

func (f *fakeNotifier) contains(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, message := range f.messages {
		if len(message) >= len(prefix) && message[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

type integrationErr string

func (e integrationErr) Error() string {
	return string(e)
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
