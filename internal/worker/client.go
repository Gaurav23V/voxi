package worker

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/Gaurav23V/voxi/internal/audio"
	"github.com/Gaurav23V/voxi/internal/ipc"
)

type Result struct {
	Transcript string
	Cleaned    string
	Stage      string
	Code       string
	Message    string
}

type Health struct {
	Device   string
	ASRModel string
	LLMModel string
}

type Client interface {
	TranscribeAndClean(context.Context, string, audio.Capture) (Result, error)
	Health(context.Context, string) (Health, error)
}

type SocketClient struct {
	SocketPath string
}

func (c SocketClient) TranscribeAndClean(ctx context.Context, requestID string, capture audio.Capture) (Result, error) {
	request := ipc.WorkerRequest{
		ID:           requestID,
		Op:           "transcribe_and_clean",
		AudioFormat:  capture.AudioFormat,
		SampleRateHz: capture.SampleRateHz,
		AudioB64:     base64.StdEncoding.EncodeToString(capture.Audio),
	}

	var response ipc.WorkerResponse
	if err := ipc.Call(ctx, c.SocketPath, request, &response); err != nil {
		return Result{}, err
	}

	if !response.OK {
		return Result{
			Stage:   response.Stage,
			Code:    response.Code,
			Message: response.Message,
		}, fmt.Errorf("worker %s: %s", response.Code, response.Message)
	}

	return Result{
		Transcript: response.Transcript,
		Cleaned:    response.Cleaned,
	}, nil
}

func (c SocketClient) Health(ctx context.Context, requestID string) (Health, error) {
	request := ipc.WorkerRequest{
		ID: requestID,
		Op: "health",
	}

	var response ipc.WorkerResponse
	if err := ipc.Call(ctx, c.SocketPath, request, &response); err != nil {
		return Health{}, err
	}
	if !response.OK {
		return Health{}, fmt.Errorf("worker %s: %s", response.Code, response.Message)
	}

	return Health{
		Device:   response.Device,
		ASRModel: response.ASRModel,
		LLMModel: response.LLMModel,
	}, nil
}

type UnavailableClient struct{}

func (UnavailableClient) TranscribeAndClean(context.Context, string, audio.Capture) (Result, error) {
	return Result{Stage: "startup", Code: "BOOT_DEP_MISSING", Message: "worker unavailable"}, fmt.Errorf("worker unavailable")
}

func (UnavailableClient) Health(context.Context, string) (Health, error) {
	return Health{}, fmt.Errorf("worker unavailable")
}
