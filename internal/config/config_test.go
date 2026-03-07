package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromPathAppliesMissingDefaults(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := `
hotkey_command: "voxi toggle"
asr_model: "nvidia/parakeet-tdt-0.6b-v2"
llm_runtime: "ollama"
llm_model: "gemma3:4b"
insert_method: "wtype"
notification_timeout_ms: 2200
asr_timeout_ms: 1500
llm_timeout_ms: 1200
insertion_timeout_ms: 200
worker_python: "/tmp/worker-python"
worker_entrypoint: "voxi_worker"
ollama_url: "http://127.0.0.1:11434"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, _, err := LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadFromPath() error = %v", err)
	}

	if cfg.WorkerHealthTimeout != Default().WorkerHealthTimeout {
		t.Fatalf("WorkerHealthTimeout = %d, want %d", cfg.WorkerHealthTimeout, Default().WorkerHealthTimeout)
	}
	if cfg.WorkerShutdownSignal != Default().WorkerShutdownSignal {
		t.Fatalf("WorkerShutdownSignal = %q, want %q", cfg.WorkerShutdownSignal, Default().WorkerShutdownSignal)
	}
}

func TestLoadFromPathAppliesDefaultsForNonPositiveTimeouts(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := `
asr_timeout_ms: 0
llm_timeout_ms: -10
insertion_timeout_ms: 0
worker_health_timeout_ms: 0
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, _, err := LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadFromPath() error = %v", err)
	}

	defaults := Default()
	if cfg.ASRTimeout != defaults.ASRTimeout {
		t.Fatalf("ASRTimeout = %d, want %d", cfg.ASRTimeout, defaults.ASRTimeout)
	}
	if cfg.LLMTimeout != defaults.LLMTimeout {
		t.Fatalf("LLMTimeout = %d, want %d", cfg.LLMTimeout, defaults.LLMTimeout)
	}
	if cfg.InsertionTimeout != defaults.InsertionTimeout {
		t.Fatalf("InsertionTimeout = %d, want %d", cfg.InsertionTimeout, defaults.InsertionTimeout)
	}
	if cfg.WorkerHealthTimeout != defaults.WorkerHealthTimeout {
		t.Fatalf("WorkerHealthTimeout = %d, want %d", cfg.WorkerHealthTimeout, defaults.WorkerHealthTimeout)
	}
}
