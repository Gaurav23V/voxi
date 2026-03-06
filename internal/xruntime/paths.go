package xruntime

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

const (
	socketDirName  = "voxi"
	logDirName     = "voxi"
	configDirName  = "voxi"
	configFileName = "config.yaml"
)

func UID() (int, error) {
	current, err := user.Current()
	if err != nil {
		return 0, fmt.Errorf("lookup current user: %w", err)
	}

	uid, err := strconv.Atoi(current.Uid)
	if err != nil {
		return 0, fmt.Errorf("parse uid %q: %w", current.Uid, err)
	}

	return uid, nil
}

func RuntimeDir() string {
	if dir := os.Getenv("VOXI_RUNTIME_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, socketDirName)
	}

	uid, err := UID()
	if err == nil {
		return filepath.Join("/run/user", strconv.Itoa(uid), socketDirName)
	}

	return filepath.Join(os.TempDir(), socketDirName)
}

func DaemonSocketPath() string {
	if path := os.Getenv("VOXI_DAEMON_SOCKET"); path != "" {
		return path
	}
	return filepath.Join(RuntimeDir(), "daemon.sock")
}

func WorkerSocketPath() string {
	if path := os.Getenv("VOXI_WORKER_SOCKET"); path != "" {
		return path
	}
	return filepath.Join(RuntimeDir(), "worker.sock")
}

func StateDir() (string, error) {
	if dir := os.Getenv("VOXI_STATE_DIR"); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, logDirName), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, ".local", "state", logDirName), nil
}

func LogFilePath() (string, error) {
	stateDir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "voxi.log"), nil
}

func ConfigFilePath() (string, error) {
	if path := os.Getenv("VOXI_CONFIG_PATH"); path != "" {
		return path, nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, configDirName, configFileName), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, ".config", configDirName, configFileName), nil
}

func EnsureRuntimeDir() error {
	return os.MkdirAll(RuntimeDir(), 0o755)
}

func EnsureStateDir() error {
	stateDir, err := StateDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(stateDir, 0o755)
}
