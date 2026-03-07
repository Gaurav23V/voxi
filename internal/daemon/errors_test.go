package daemon

import (
	"errors"
	"testing"

	"github.com/Gaurav23V/voxi/internal/worker"
)

func TestNormalizeWorkerError_IPCTimeoutMapsToASRTimeout(t *testing.T) {
	result := worker.Result{} // empty: IPC timeout before worker responded
	err := errors.New("decode response: read unix /tmp/voxi.sock: i/o timeout")

	got := normalizeWorkerError(result, err)
	stageErr := AsStageError(got)

	if stageErr.Code != "ASR_TIMEOUT" {
		t.Errorf("Code = %q, want ASR_TIMEOUT", stageErr.Code)
	}
	if stageErr.Reason != "inference exceeded timeout" {
		t.Errorf("Reason = %q, want inference exceeded timeout", stageErr.Reason)
	}
	if stageErr.Stage != "Speech recognition" {
		t.Errorf("Stage = %q, want Speech recognition", stageErr.Stage)
	}
}

func TestNormalizeWorkerError_WorkerASRTimeoutPreserved(t *testing.T) {
	result := worker.Result{
		Stage:   "speech_recognition",
		Code:    "ASR_TIMEOUT",
		Message: "inference exceeded timeout",
	}
	err := errors.New("worker error")

	got := normalizeWorkerError(result, err)
	stageErr := AsStageError(got)

	if stageErr.Code != "ASR_TIMEOUT" {
		t.Errorf("Code = %q, want ASR_TIMEOUT", stageErr.Code)
	}
	if stageErr.Reason != "inference exceeded timeout" {
		t.Errorf("Reason = %q, want inference exceeded timeout", stageErr.Reason)
	}
}

func TestIsRetryable_IPCTimeoutIsRetryable(t *testing.T) {
	result := worker.Result{}
	err := errors.New("decode response: read unix /tmp/voxi.sock: i/o timeout")
	normalized := normalizeWorkerError(result, err)

	if !isRetryable(normalized) {
		t.Error("IPC timeout should be retryable (Speech recognition stage)")
	}
}
