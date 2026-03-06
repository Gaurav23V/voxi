package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Gaurav23V/voxi/internal/xruntime"
)

type Event struct {
	Timestamp  string `json:"timestamp"`
	Stage      string `json:"stage,omitempty"`
	State      string `json:"state,omitempty"`
	Result     string `json:"result,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	RetryCount int    `json:"retry_count,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Message    string `json:"message,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
}

type Logger interface {
	Log(Event)
	Close() error
}

type JSONLogger struct {
	mu     sync.Mutex
	writer io.Writer
	closer io.Closer
}

func New() (*JSONLogger, error) {
	if err := xruntime.EnsureStateDir(); err != nil {
		return nil, fmt.Errorf("ensure state dir: %w", err)
	}

	logPath, err := xruntime.LogFilePath()
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return &JSONLogger{
		writer: io.MultiWriter(os.Stdout, file),
		closer: file,
	}, nil
}

func NewForWriter(writer io.Writer) *JSONLogger {
	return &JSONLogger{writer: writer}
}

func (l *JSONLogger) Log(event Event) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		return
	}

	_, _ = fmt.Fprintln(l.writer, string(encoded))
}

func (l *JSONLogger) Close() error {
	if l.closer == nil {
		return nil
	}
	return l.closer.Close()
}
