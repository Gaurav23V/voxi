package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCallDaemonTimesOutWhenServerStopsResponding(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	t.Setenv("VOXI_DAEMON_SOCKET", socketPath)
	t.Setenv("VOXI_DAEMON_RPC_TIMEOUT_MS", "50")

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		time.Sleep(200 * time.Millisecond)
	}()

	_, err = callDaemon("status")
	if err == nil {
		t.Fatal("callDaemon() error = nil, want timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("callDaemon() error = %q, want timeout message", err)
	}

	<-done
}

func TestDaemonRPCTimeoutDefaultsOnInvalidOverride(t *testing.T) {
	t.Setenv("VOXI_DAEMON_RPC_TIMEOUT_MS", "invalid")
	if got := daemonRPCTimeout(); got != 2*time.Second {
		t.Fatalf("daemonRPCTimeout() = %s, want %s", got, 2*time.Second)
	}
}

func TestDaemonRPCTimeoutUsesEnvironmentOverride(t *testing.T) {
	t.Setenv("VOXI_DAEMON_RPC_TIMEOUT_MS", "125")
	if got := daemonRPCTimeout(); got != 125*time.Millisecond {
		t.Fatalf("daemonRPCTimeout() = %s, want %s", got, 125*time.Millisecond)
	}
}

func TestRunStatusUsesConfiguredSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	t.Setenv("VOXI_DAEMON_SOCKET", socketPath)

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte(fmt.Sprintf("{\"id\":\"cli-%d\",\"ok\":true,\"state\":\"Idle\"}\n", os.Getpid())))
	}()

	if response, err := callDaemon("status"); err != nil || response.State != "Idle" {
		t.Fatalf("callDaemon(status) = %+v, %v, want Idle,nil", response, err)
	}

	<-done
}

func TestRunRestartUsesSystemctlUserService(t *testing.T) {
	var calls []commandCall
	stubCommandExecution(t, func(_ context.Context, dir, name string, args ...string) error {
		calls = append(calls, commandCall{dir: dir, name: name, args: append([]string(nil), args...)})
		return nil
	})

	if err := run([]string{"restart"}); err != nil {
		t.Fatalf("run(restart) error = %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("command calls = %d, want 1", len(calls))
	}
	if calls[0].dir != "" {
		t.Fatalf("restart command dir = %q, want empty", calls[0].dir)
	}
	if calls[0].name != "systemctl" {
		t.Fatalf("restart command name = %q, want systemctl", calls[0].name)
	}
	wantArgs := []string{"--user", "restart", voxiServiceName}
	if !slices.Equal(calls[0].args, wantArgs) {
		t.Fatalf("restart command args = %v, want %v", calls[0].args, wantArgs)
	}
}

func TestRunRebuildBuildsInstallsAndRestartsService(t *testing.T) {
	sourceDir := createFakeSourceTree(t)
	homeDir := t.TempDir()

	t.Setenv(voxiSourceDirEnv, sourceDir)
	stubHomeDir(t, homeDir)

	var calls []commandCall
	stubCommandExecution(t, func(_ context.Context, dir, name string, args ...string) error {
		calls = append(calls, commandCall{dir: dir, name: name, args: append([]string(nil), args...)})

		if name == "go" {
			outputIndex := slices.Index(args, "-o")
			if outputIndex < 0 || outputIndex+1 >= len(args) {
				t.Fatalf("go build args missing -o output path: %v", args)
			}
			if err := os.WriteFile(args[outputIndex+1], []byte("rebuilt binary"), 0o755); err != nil {
				t.Fatalf("WriteFile(rebuilt binary): %v", err)
			}
		}
		return nil
	})

	if err := run([]string{"rebuild"}); err != nil {
		t.Fatalf("run(rebuild) error = %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("command calls = %d, want 2", len(calls))
	}
	if calls[0].name != "go" {
		t.Fatalf("first command = %q, want go", calls[0].name)
	}
	if calls[0].dir != sourceDir {
		t.Fatalf("go build dir = %q, want %q", calls[0].dir, sourceDir)
	}
	wantBuildArgsPrefix := []string{"build", "-o"}
	if len(calls[0].args) < 4 || !slices.Equal(calls[0].args[:2], wantBuildArgsPrefix) || calls[0].args[3] != "./cmd/voxi" {
		t.Fatalf("go build args = %v, want build -o <temp> ./cmd/voxi", calls[0].args)
	}

	if calls[1].name != "systemctl" {
		t.Fatalf("second command = %q, want systemctl", calls[1].name)
	}
	wantRestartArgs := []string{"--user", "restart", voxiServiceName}
	if !slices.Equal(calls[1].args, wantRestartArgs) {
		t.Fatalf("restart command args = %v, want %v", calls[1].args, wantRestartArgs)
	}

	installedBinary := filepath.Join(homeDir, voxiInstallRelBin)
	content, err := os.ReadFile(installedBinary)
	if err != nil {
		t.Fatalf("ReadFile(installed binary) error = %v", err)
	}
	if string(content) != "rebuilt binary" {
		t.Fatalf("installed binary content = %q, want %q", string(content), "rebuilt binary")
	}
}

func TestFindSourceDirWalksUpTree(t *testing.T) {
	sourceDir := createFakeSourceTree(t)
	startDir := filepath.Join(sourceDir, "nested", "deeper")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(startDir) error = %v", err)
	}

	got, err := findSourceDir(startDir)
	if err != nil {
		t.Fatalf("findSourceDir() error = %v", err)
	}
	if got != sourceDir {
		t.Fatalf("findSourceDir() = %q, want %q", got, sourceDir)
	}
}

func TestFindSourceDirReturnsHelpfulErrorWhenMissing(t *testing.T) {
	_, err := findSourceDir(t.TempDir())
	if err == nil {
		t.Fatal("findSourceDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "VOXI_SOURCE_DIR") {
		t.Fatalf("findSourceDir() error = %q, want source-dir guidance", err)
	}
}

type commandCall struct {
	dir  string
	name string
	args []string
}

func stubCommandExecution(t *testing.T, stub func(context.Context, string, string, ...string) error) {
	t.Helper()

	previous := executeCommand
	executeCommand = stub
	t.Cleanup(func() {
		executeCommand = previous
	})
}

func stubHomeDir(t *testing.T, homeDir string) {
	t.Helper()

	previous := resolveHomeDir
	resolveHomeDir = func() (string, error) {
		return homeDir, nil
	}
	t.Cleanup(func() {
		resolveHomeDir = previous
	})
}

func createFakeSourceTree(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/voxi\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "voxi"), 0o755); err != nil {
		t.Fatalf("MkdirAll(cmd/voxi) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "voxi", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(cmd/voxi/main.go) error = %v", err)
	}
	return root
}
