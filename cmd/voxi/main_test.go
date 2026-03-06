package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
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
