package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Gaurav23V/voxi/internal/config"
)

type Check struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Detail    string `json:"detail"`
	Fatal     bool   `json:"fatal,omitempty"`
	Category  string `json:"category,omitempty"`
}

type Report struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
}

func Run(ctx context.Context, cfg config.Config) (Report, error) {
	checks := []Check{
		binaryCheck("wtype", true),
		binaryCheck("wl-copy", true),
		binaryCheck("notify-send", true),
		binaryCheck("pw-record", true),
		binaryCheck(cfg.WorkerPython, true),
		binaryCheck("ollama", false),
		ollamaCheck(ctx, cfg.OllamaURL),
		nvidiaCheck(),
		workerRuntimeCheck(cfg),
	}

	ok := true
	for _, check := range checks {
		if check.Fatal && check.Status != "ok" {
			ok = false
		}
	}

	return Report{OK: ok, Checks: checks}, nil
}

func Format(report Report) (string, error) {
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func binaryCheck(name string, fatal bool) Check {
	if _, err := exec.LookPath(name); err != nil {
		return Check{
			Name:     name,
			Status:   "missing",
			Detail:   "binary not found in PATH",
			Fatal:    fatal,
			Category: "binary",
		}
	}

	return Check{
		Name:     name,
		Status:   "ok",
		Detail:   "available",
		Fatal:    fatal,
		Category: "binary",
	}
}

func ollamaCheck(ctx context.Context, baseURL string) Check {
	requestCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return Check{Name: "ollama", Status: "error", Detail: err.Error(), Category: "service"}
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Check{Name: "ollama", Status: "unreachable", Detail: err.Error(), Fatal: true, Category: "service"}
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		return Check{Name: "ollama", Status: "error", Detail: fmt.Sprintf("unexpected HTTP %d", response.StatusCode), Fatal: true, Category: "service"}
	}

	return Check{Name: "ollama", Status: "ok", Detail: "reachable", Category: "service"}
}

func nvidiaCheck() Check {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return Check{Name: "nvidia", Status: "missing", Detail: "nvidia-smi unavailable; CPU fallback expected", Category: "gpu"}
	}

	cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Check{Name: "nvidia", Status: "error", Detail: strings.TrimSpace(string(output)), Category: "gpu"}
	}

	return Check{Name: "nvidia", Status: "ok", Detail: strings.TrimSpace(string(output)), Category: "gpu"}
}

func workerRuntimeCheck(cfg config.Config) Check {
	command := exec.Command(
		cfg.WorkerPython,
		"-c",
		"from voxi_worker.health import detect_device; print(detect_device())",
	)
	command.Env = buildWorkerEnv(os.Environ())

	output, err := command.CombinedOutput()
	if err != nil {
		return Check{
			Name:     "worker_runtime",
			Status:   "error",
			Detail:   strings.TrimSpace(string(output)),
			Fatal:    true,
			Category: "worker",
		}
	}

	device := strings.TrimSpace(string(output))
	if device == "" {
		device = "cpu"
	}
	detail := fmt.Sprintf("device=%s model=%s cleanup=%s", device, cfg.ASRModel, cfg.LLMModel)
	return Check{Name: "worker_runtime", Status: "ok", Detail: detail, Category: "worker"}
}

func buildWorkerEnv(existing []string) []string {
	env := append([]string{}, existing...)
	if workerPath := locateWorkerPythonPath(); workerPath != "" {
		env = append(env, "PYTHONPATH="+workerPath)
	}
	return env
}

func locateWorkerPythonPath() string {
	if override := os.Getenv("VOXI_WORKER_PYTHONPATH"); override != "" {
		return override
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return ""
	}

	dir := currentDir
	for {
		candidate := filepath.Join(dir, "worker", "voxi_worker")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, "worker")
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
