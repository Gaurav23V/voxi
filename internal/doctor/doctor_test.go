package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Gaurav23V/voxi/internal/config"
)

func TestRunReportsMissingBinaryAndWorkerRuntime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()

	pathDir := t.TempDir()
	for _, name := range []string{"wl-copy", "notify-send", "pw-record", "python3", "ollama"} {
		makeExecutable(t, filepath.Join(pathDir, name))
	}

	t.Setenv("PATH", pathDir)
	t.Setenv("VOXI_WORKER_PYTHONPATH", filepath.Join("..", "..", "worker"))

	cfg := config.Default()
	cfg.OllamaURL = server.URL
	report, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if report.OK {
		t.Fatalf("report.OK = true, want false because wtype is missing")
	}

	missingWType := false
	workerRuntimeSeen := false
	for _, check := range report.Checks {
		if check.Name == "wtype" && check.Status == "missing" {
			missingWType = true
		}
		if check.Name == "worker_runtime" {
			workerRuntimeSeen = true
		}
	}

	if !missingWType {
		t.Fatalf("expected missing wtype check in %+v", report.Checks)
	}
	if !workerRuntimeSeen {
		t.Fatalf("expected worker_runtime check in %+v", report.Checks)
	}
}

func makeExecutable(t *testing.T, path string) {
	t.Helper()
	content := "#!/usr/bin/env bash\n"
	if filepath.Base(path) == "python3" {
		content += "echo cpu\n"
	} else {
		content += "exit 0\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
