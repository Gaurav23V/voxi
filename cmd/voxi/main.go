package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Gaurav23V/voxi/internal/audio"
	"github.com/Gaurav23V/voxi/internal/config"
	"github.com/Gaurav23V/voxi/internal/daemon"
	"github.com/Gaurav23V/voxi/internal/doctor"
	"github.com/Gaurav23V/voxi/internal/ipc"
	"github.com/Gaurav23V/voxi/internal/logging"
	"github.com/Gaurav23V/voxi/internal/output"
	"github.com/Gaurav23V/voxi/internal/worker"
	"github.com/Gaurav23V/voxi/internal/xruntime"
)

const (
	cliUsage          = "usage: voxi <daemon|toggle|status|doctor|restart|rebuild>"
	voxiServiceName   = "voxi.service"
	voxiSourceDirEnv  = "VOXI_SOURCE_DIR"
	voxiInstallRelBin = ".local/bin/voxi"
)

var (
	executeCommand = runExternalCommand
	resolveHomeDir = os.UserHomeDir
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(cliUsage)
	}

	switch args[0] {
	case "daemon":
		return runDaemon()
	case "toggle":
		return runToggle()
	case "status":
		return runStatus()
	case "doctor":
		return runDoctor()
	case "restart":
		return runRestart()
	case "rebuild":
		return runRebuild()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runDaemon() error {
	cfg, _, err := config.Load()
	if err != nil {
		return err
	}
	if err := xruntime.EnsureRuntimeDir(); err != nil {
		return err
	}

	logger, err := logging.New()
	if err != nil {
		return err
	}
	defer logger.Close()

	workerSupervisor := worker.NewSupervisor(cfg, xruntime.WorkerSocketPath(), logger)
	startCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.WorkerHealthTimeout)*time.Millisecond)
	if err := workerSupervisor.Start(startCtx); err != nil {
		logger.Log(logging.Event{Stage: "Startup", Result: "worker_degraded", Message: err.Error()})
	}
	cancel()

	service := daemon.NewService(
		cfg,
		audio.NewPWRecorder(),
		workerSupervisor,
		output.NewWTypeInserter(),
		output.NewWLCopyClipboard(),
		output.NewNotifySend(cfg.NotificationTimeout),
		logger,
	)

	server := ipc.NewServer(xruntime.DaemonSocketPath(), service.HandleRPC)
	if err := server.Start(); err != nil {
		return err
	}
	defer server.Close()

	logger.Log(logging.Event{Stage: "Startup", State: "Idle", Result: "ready"})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	return workerSupervisor.Stop()
}

func runToggle() error {
	response, err := callDaemon("toggle")
	if err != nil {
		return fmt.Errorf("toggle daemon: %w", err)
	}
	if !response.OK {
		return errors.New(response.Message)
	}
	fmt.Println(response.State)
	return nil
}

func runStatus() error {
	response, err := callDaemon("status")
	if err != nil {
		return fmt.Errorf("status daemon: %w", err)
	}
	if !response.OK {
		return errors.New(response.Message)
	}
	fmt.Printf("{\"state\":%q}\n", response.State)
	return nil
}

func runDoctor() error {
	cfg, _, err := config.Load()
	if err != nil {
		return err
	}

	report, err := doctor.Run(context.Background(), cfg)
	if err != nil {
		return err
	}

	formatted, err := doctor.Format(report)
	if err != nil {
		return err
	}

	fmt.Println(formatted)
	if !report.OK {
		return errors.New("doctor found fatal readiness issues")
	}

	return nil
}

func runRestart() error {
	if err := executeNamedCommand(context.Background(), "", "systemctl", "--user", "restart", voxiServiceName); err != nil {
		return fmt.Errorf("restart service: %w", err)
	}
	return nil
}

func runRebuild() error {
	sourceDir, err := resolveSourceDir()
	if err != nil {
		return err
	}

	homeDir, err := resolveHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "voxi-rebuild-*")
	if err != nil {
		return fmt.Errorf("create rebuild temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempBinary := filepath.Join(tempDir, "voxi")
	if err := executeNamedCommand(context.Background(), sourceDir, "go", "build", "-o", tempBinary, "./cmd/voxi"); err != nil {
		return fmt.Errorf("build binary: %w", err)
	}

	installPath := filepath.Join(homeDir, voxiInstallRelBin)
	if err := installBinary(tempBinary, installPath); err != nil {
		return err
	}

	if err := runRestart(); err != nil {
		return err
	}

	return nil
}

func callDaemon(op string) (ipc.DaemonResponse, error) {
	timeout := daemonRPCTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	request := ipc.DaemonRequest{
		ID: fmt.Sprintf("cli-%d", os.Getpid()),
		Op: op,
	}

	var response ipc.DaemonResponse
	if err := ipc.Call(ctx, xruntime.DaemonSocketPath(), request, &response); err != nil {
		if isRPCTimeout(err) {
			return ipc.DaemonResponse{}, fmt.Errorf("daemon request timed out after %s", timeout)
		}
		return ipc.DaemonResponse{}, err
	}
	return response, nil
}

func daemonRPCTimeout() time.Duration {
	const defaultTimeout = 2 * time.Second

	raw := os.Getenv("VOXI_DAEMON_RPC_TIMEOUT_MS")
	if raw == "" {
		return defaultTimeout
	}

	timeoutMS, err := strconv.Atoi(raw)
	if err != nil || timeoutMS <= 0 {
		return defaultTimeout
	}

	return time.Duration(timeoutMS) * time.Millisecond
}

func isRPCTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

func executeNamedCommand(ctx context.Context, dir, name string, args ...string) error {
	return executeCommand(ctx, dir, name, args...)
}

func runExternalCommand(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	trimmed := strings.TrimSpace(string(output))
	if errors.Is(err, exec.ErrNotFound) {
		if trimmed == "" {
			return fmt.Errorf("%s not found in PATH", name)
		}
		return fmt.Errorf("%s not found in PATH: %s", name, trimmed)
	}
	if trimmed == "" {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return fmt.Errorf("%s failed: %w: %s", name, err, trimmed)
}

func resolveSourceDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv(voxiSourceDirEnv)); override != "" {
		resolved, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", voxiSourceDirEnv, err)
		}
		if ok, err := isVoxiSourceDir(resolved); err != nil {
			return "", err
		} else if !ok {
			return "", fmt.Errorf("%s=%q is not a Voxi source directory", voxiSourceDirEnv, override)
		}
		return resolved, nil
	}

	startDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return findSourceDir(startDir)
}

func findSourceDir(startDir string) (string, error) {
	current, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve source search path: %w", err)
	}

	for {
		ok, err := isVoxiSourceDir(current)
		if err != nil {
			return "", err
		}
		if ok {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", errors.New("could not locate Voxi source directory; run `voxi rebuild` from the repo or set VOXI_SOURCE_DIR")
}

func isVoxiSourceDir(dir string) (bool, error) {
	requiredPaths := []string{
		filepath.Join(dir, "go.mod"),
		filepath.Join(dir, "cmd", "voxi", "main.go"),
	}

	for _, path := range requiredPaths {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("check source path %q: %w", path, err)
		}
		if info.IsDir() {
			return false, nil
		}
	}

	return true, nil
}

func installBinary(sourcePath, installPath string) error {
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		return fmt.Errorf("ensure install directory: %w", err)
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open rebuilt binary: %w", err)
	}
	defer sourceFile.Close()

	tempFile, err := os.CreateTemp(filepath.Dir(installPath), ".voxi-install-*")
	if err != nil {
		return fmt.Errorf("create install temp file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(tempFile, sourceFile); err != nil {
		tempFile.Close()
		return fmt.Errorf("copy rebuilt binary: %w", err)
	}
	if err := tempFile.Chmod(0o755); err != nil {
		tempFile.Close()
		return fmt.Errorf("chmod rebuilt binary: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close install temp file: %w", err)
	}
	if err := os.Rename(tempPath, installPath); err != nil {
		return fmt.Errorf("install rebuilt binary: %w", err)
	}

	cleanup = false
	return nil
}
