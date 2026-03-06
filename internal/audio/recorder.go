package audio

import (
	"context"
	"errors"
)

type Capture struct {
	Audio        []byte
	AudioFormat  string
	SampleRateHz int
}

type Recorder interface {
	Start(context.Context) error
	Stop(context.Context) (Capture, error)
}

var ErrUnavailable = errors.New("audio recorder unavailable")

type UnavailableRecorder struct{}

func (UnavailableRecorder) Start(context.Context) error {
	return ErrUnavailable
}

func (UnavailableRecorder) Stop(context.Context) (Capture, error) {
	return Capture{}, ErrUnavailable
}
