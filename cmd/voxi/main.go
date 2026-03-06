package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
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

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: voxi <daemon|toggle|status|doctor>")
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

func callDaemon(op string) (ipc.DaemonResponse, error) {
	request := ipc.DaemonRequest{
		ID: fmt.Sprintf("cli-%d", os.Getpid()),
		Op: op,
	}

	var response ipc.DaemonResponse
	if err := ipc.Call(context.Background(), xruntime.DaemonSocketPath(), request, &response); err != nil {
		return ipc.DaemonResponse{}, err
	}
	return response, nil
}
