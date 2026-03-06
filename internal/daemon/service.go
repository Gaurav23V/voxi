package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Gaurav23V/voxi/internal/audio"
	"github.com/Gaurav23V/voxi/internal/config"
	"github.com/Gaurav23V/voxi/internal/ipc"
	"github.com/Gaurav23V/voxi/internal/logging"
	"github.com/Gaurav23V/voxi/internal/output"
	"github.com/Gaurav23V/voxi/internal/state"
	"github.com/Gaurav23V/voxi/internal/worker"
)

type Service struct {
	cfg       config.Config
	recorder  audio.Recorder
	worker    worker.Client
	inserter  output.Inserter
	clipboard output.Clipboard
	notifier  output.Notifier
	logger    logging.Logger

	mu           sync.Mutex
	machine      *state.Machine
	currentJobID string
}

func NewService(cfg config.Config, recorder audio.Recorder, workerClient worker.Client, inserter output.Inserter, clipboard output.Clipboard, notifier output.Notifier, logger logging.Logger) *Service {
	return &Service{
		cfg:       cfg,
		recorder:  recorder,
		worker:    workerClient,
		inserter:  inserter,
		clipboard: clipboard,
		notifier:  notifier,
		logger:    logger,
		machine:   state.New(),
	}
}

func (s *Service) HandleRPC(ctx context.Context, payload json.RawMessage) (any, error) {
	var request ipc.DaemonRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return ipc.DaemonResponse{OK: false, Message: fmt.Sprintf("invalid request: %v", err)}, nil
	}

	switch request.Op {
	case "toggle":
		nextState, err := s.Toggle(ctx)
		if err != nil {
			return ipc.DaemonResponse{ID: request.ID, OK: false, State: string(nextState), Message: err.Error()}, nil
		}
		return ipc.DaemonResponse{ID: request.ID, OK: true, State: string(nextState)}, nil
	case "status":
		return ipc.DaemonResponse{ID: request.ID, OK: true, State: string(s.Status())}, nil
	default:
		return ipc.DaemonResponse{ID: request.ID, OK: false, State: string(s.Status()), Message: "unsupported operation"}, nil
	}
}

func (s *Service) Toggle(ctx context.Context) (state.Value, error) {
	s.mu.Lock()
	current := s.machine.Current()

	switch current {
	case state.Idle:
		if err := s.recorder.Start(ctx); err != nil {
			s.mu.Unlock()
			stageErr := NewStageError("Recording", "REC_MIC_UNAVAILABLE", shortReason(err))
			s.fail(context.Background(), "", stageErr)
			return state.Idle, stageErr
		}

		next := s.machine.Toggle()
		s.mu.Unlock()

		s.logger.Log(logging.Event{Stage: "Recording", State: string(next), Result: "started"})
		s.notify(context.Background(), "Listening...", "")
		return next, nil
	case state.Recording:
		capture, err := s.recorder.Stop(ctx)
		if err != nil {
			s.machine.Reset()
			s.mu.Unlock()
			stageErr := NewStageError("Audio finalize", "REC_EMPTY_AUDIO", shortReason(err))
			s.fail(context.Background(), "", stageErr)
			return state.Idle, stageErr
		}

		next := s.machine.Toggle()
		jobID := nextRequestID()
		s.currentJobID = jobID
		s.mu.Unlock()

		s.logger.Log(logging.Event{Stage: "Audio finalize", State: string(next), Result: "captured", RequestID: jobID})
		s.notify(context.Background(), "Processing speech...", "")
		go s.runPipeline(jobID, capture)
		return next, nil
	default:
		s.mu.Unlock()
		if current == state.Processing {
			s.notify(context.Background(), "Processing speech...", "")
		}
		s.logger.Log(logging.Event{Stage: "toggle", State: string(current), Result: "ignored"})
		return current, nil
	}
}

func (s *Service) Status() state.Value {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.machine.Current()
}

func (s *Service) runPipeline(jobID string, capture audio.Capture) {
	started := time.Now()

	result, retryCount, err := s.transcribeWithRetry(jobID, capture)
	if err != nil {
		s.fail(context.Background(), jobID, err)
		return
	}

	if !s.beginInsert(jobID) {
		return
	}

	text := strings.TrimSpace(result.Cleaned)
	if text == "" {
		text = strings.TrimSpace(result.Transcript)
	}

	if text == "" {
		s.fail(context.Background(), jobID, NewStageError("Text cleanup", "LLM_EMPTY_OUTPUT", "empty output"))
		return
	}

	insertStart := time.Now()
	if err := s.insertWithFallback(context.Background(), jobID, text); err != nil {
		s.fail(context.Background(), jobID, err)
		return
	}

	s.resetToIdle(jobID)
	s.logger.Log(logging.Event{
		Stage:      "pipeline",
		State:      string(state.Idle),
		Result:     "completed",
		RequestID:  jobID,
		RetryCount: retryCount,
		DurationMS: time.Since(started).Milliseconds() + time.Since(insertStart).Milliseconds(),
	})
}

func (s *Service) transcribeWithRetry(jobID string, capture audio.Capture) (worker.Result, int, error) {
	var lastError error

	for attempt := 0; attempt < 2; attempt++ {
		attemptCtx, cancel := context.WithTimeout(context.Background(), time.Duration(s.cfg.ASRTimeout+s.cfg.LLMTimeout)*time.Millisecond)
		started := time.Now()
		result, err := s.worker.TranscribeAndClean(attemptCtx, jobID, capture)
		cancel()

		if err == nil {
			s.logger.Log(logging.Event{
				Stage:      "Speech recognition",
				Result:     "ok",
				RequestID:  jobID,
				RetryCount: attempt,
				DurationMS: time.Since(started).Milliseconds(),
			})
			return result, attempt, nil
		}

		lastError = normalizeWorkerError(result, err)
		if !isRetryable(lastError) || attempt == 1 {
			return worker.Result{}, attempt, lastError
		}

		s.logger.Log(logging.Event{
			Stage:      stageFromError(lastError),
			Result:     "retrying",
			RequestID:  jobID,
			ErrorCode:  codeFromError(lastError),
			RetryCount: attempt + 1,
			DurationMS: time.Since(started).Milliseconds(),
		})
	}

	return worker.Result{}, 1, lastError
}

func (s *Service) insertWithFallback(ctx context.Context, jobID, text string) error {
	var lastError error

	for attempt := 0; attempt < 2; attempt++ {
		insertCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.InsertionTimeout)*time.Millisecond)
		started := time.Now()
		err := s.inserter.Insert(insertCtx, text)
		cancel()
		if err == nil {
			s.notify(context.Background(), "Inserted", "")
			s.logger.Log(logging.Event{
				Stage:      "Text insertion",
				Result:     "inserted",
				RequestID:  jobID,
				RetryCount: attempt,
				DurationMS: time.Since(started).Milliseconds(),
			})
			return nil
		}

		lastError = NewStageError("Text insertion", "INS_WTYPE_FAILED", shortReason(err))
		if attempt == 1 {
			break
		}

		s.logger.Log(logging.Event{
			Stage:      "Text insertion",
			Result:     "retrying",
			RequestID:  jobID,
			ErrorCode:  "INS_WTYPE_FAILED",
			RetryCount: attempt + 1,
			DurationMS: time.Since(started).Milliseconds(),
		})
	}

	clipboardCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.InsertionTimeout)*time.Millisecond)
	defer cancel()
	if err := s.clipboard.Copy(clipboardCtx, text); err != nil {
		return NewStageError("Text insertion", "INS_CLIPBOARD_FAILED", shortReason(err))
	}

	s.notify(context.Background(), "Copied to clipboard", "")
	s.logger.Log(logging.Event{
		Stage:     "Text insertion",
		Result:    "clipboard_fallback",
		RequestID: jobID,
		ErrorCode: codeFromError(lastError),
	})
	return nil
}

func (s *Service) beginInsert(jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentJobID != jobID || s.machine.Current() != state.Processing {
		return false
	}

	s.machine.BeginInsert()
	return true
}

func (s *Service) resetToIdle(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentJobID == jobID {
		s.machine.Reset()
		s.currentJobID = ""
	}
}

func (s *Service) fail(ctx context.Context, jobID string, err error) {
	stageErr := AsStageError(err)
	s.notify(ctx, stageErr.Title, stageErr.Body())
	s.logger.Log(logging.Event{
		Stage:     stageErr.Stage,
		State:     string(state.Idle),
		Result:    "error",
		RequestID: jobID,
		ErrorCode: stageErr.Code,
		Message:   stageErr.Reason,
	})
	s.resetToIdle(jobID)
}

func (s *Service) notify(ctx context.Context, title, body string) {
	if s.notifier == nil {
		return
	}
	_ = s.notifier.Notify(ctx, title, body)
}

var requestCounter uint64

func nextRequestID() string {
	counter := atomic.AddUint64(&requestCounter, 1)
	return fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), counter)
}
