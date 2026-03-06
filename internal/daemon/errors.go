package daemon

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Gaurav23V/voxi/internal/worker"
)

type StageError struct {
	Title  string
	Stage  string
	Code   string
	Reason string
}

func NewStageError(stage, code, reason string) StageError {
	title := "Action failed"
	if stage == "Speech recognition" {
		title = "Transcription failed"
	}

	return StageError{
		Title:  title,
		Stage:  stage,
		Code:   code,
		Reason: reason,
	}
}

func (e StageError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", e.Title, e.Stage, e.Reason)
}

func (e StageError) Body() string {
	return fmt.Sprintf("Stage: %s (%s)", e.Stage, e.Reason)
}

func AsStageError(err error) StageError {
	var stageErr StageError
	if errors.As(err, &stageErr) {
		return stageErr
	}
	return NewStageError("Startup", "BOOT_DEP_MISSING", shortReason(err))
}

func normalizeWorkerError(result worker.Result, err error) error {
	stage := mapStage(result.Stage)
	if stage == "" {
		stage = "Speech recognition"
	}
	code := result.Code
	if code == "" {
		code = "ASR_RUNTIME_FAILURE"
	}
	reason := result.Message
	if reason == "" {
		reason = shortReason(err)
	}
	return NewStageError(stage, code, reason)
}

func isRetryable(err error) bool {
	stageErr := AsStageError(err)
	if stageErr.Code == "REC_EMPTY_AUDIO" || stageErr.Code == "BOOT_DEP_MISSING" || strings.Contains(strings.ToLower(stageErr.Reason), "unavailable") {
		return false
	}
	return stageErr.Stage == "Speech recognition" || stageErr.Stage == "Text cleanup"
}

func mapStage(workerStage string) string {
	switch workerStage {
	case "speech_recognition":
		return "Speech recognition"
	case "text_cleanup":
		return "Text cleanup"
	case "startup":
		return "Startup"
	default:
		return ""
	}
}

func stageFromError(err error) string {
	return AsStageError(err).Stage
}

func codeFromError(err error) string {
	return AsStageError(err).Code
}

func shortReason(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "unknown error"
	}
	message = strings.ReplaceAll(message, "\n", " ")
	return message
}
