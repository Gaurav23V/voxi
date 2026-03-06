package ipc

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestServerStartRemovesStaleSocketOnly(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "daemon.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	server := NewServer(socketPath, func(context.Context, json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Close()
}

func TestServerStartRejectsRegularFileAtSocketPath(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	if err := os.WriteFile(socketPath, []byte("not a socket"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	server := NewServer(socketPath, func(context.Context, json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	if err := server.Start(); err == nil {
		t.Fatal("Start() error = nil, want refusal for non-socket path")
	}
}
