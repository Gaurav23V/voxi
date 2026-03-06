package config

import (
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/Gaurav23V/voxi/internal/xruntime"
)

type Config struct {
	HotkeyCommand        string `yaml:"hotkey_command"`
	ASRModel             string `yaml:"asr_model"`
	LLMRuntime           string `yaml:"llm_runtime"`
	LLMModel             string `yaml:"llm_model"`
	InsertMethod         string `yaml:"insert_method"`
	NotificationTimeout  int    `yaml:"notification_timeout_ms"`
	ASRTimeout           int    `yaml:"asr_timeout_ms"`
	LLMTimeout           int    `yaml:"llm_timeout_ms"`
	InsertionTimeout     int    `yaml:"insertion_timeout_ms"`
	OllamaURL            string `yaml:"ollama_url"`
	WorkerPython         string `yaml:"worker_python"`
	WorkerEntrypoint     string `yaml:"worker_entrypoint"`
	WorkerHealthTimeout  int    `yaml:"worker_health_timeout_ms"`
	WorkerShutdownSignal string `yaml:"worker_shutdown_signal"`
}

func Default() Config {
	return Config{
		HotkeyCommand:        "voxi toggle",
		ASRModel:             "nvidia/parakeet-tdt-0.6b-v2",
		LLMRuntime:           "ollama",
		LLMModel:             "gemma3:4b",
		InsertMethod:         "wtype",
		NotificationTimeout:  2200,
		ASRTimeout:           1500,
		LLMTimeout:           1200,
		InsertionTimeout:     200,
		OllamaURL:            "http://127.0.0.1:11434",
		WorkerPython:         "python3",
		WorkerEntrypoint:     "voxi_worker",
		WorkerHealthTimeout:  1500,
		WorkerShutdownSignal: "term",
	}
}

func Load() (Config, string, error) {
	path, err := xruntime.ConfigFilePath()
	if err != nil {
		return Config{}, "", err
	}
	return LoadFromPath(path)
}

func LoadFromPath(path string) (Config, string, error) {
	cfg := Default()

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnvironmentOverrides(&cfg)
			return cfg, path, nil
		}
		return Config{}, path, fmt.Errorf("read config: %w", err)
	}

	decoder := yaml.NewDecoder(strings.NewReader(string(content)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, path, fmt.Errorf("decode config: %w", err)
	}

	applyEnvironmentOverrides(&cfg)
	return cfg, path, nil
}

func applyEnvironmentOverrides(cfg *Config) {
	if value := os.Getenv("VOXI_WORKER_PYTHON"); value != "" {
		cfg.WorkerPython = value
	}
	if value := os.Getenv("VOXI_WORKER_ENTRYPOINT"); value != "" {
		cfg.WorkerEntrypoint = value
	}
	if value := os.Getenv("VOXI_OLLAMA_URL"); value != "" {
		cfg.OllamaURL = value
	}
}
