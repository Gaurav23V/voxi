package output

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Clipboard interface {
	Copy(context.Context, string) error
}

type WLCopyClipboard struct {
	Command string
}

const wlCopyStartupGracePeriod = 100 * time.Millisecond

func NewWLCopyClipboard() *WLCopyClipboard {
	command := os.Getenv("VOXI_WL_COPY_COMMAND")
	if command == "" {
		command = "wl-copy"
	}
	return &WLCopyClipboard{Command: command}
}

func (c *WLCopyClipboard) Copy(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	command := c.Command
	if command == "" {
		command = "wl-copy"
	}

	// wl-copy can remain alive after it has taken ownership of the clipboard,
	// so treat a still-running process as success after a short startup window.
	cmd := exec.Command(command)
	cmd.Stdin = strings.NewReader(text)
	cmd.Stdout = io.Discard

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("wl-copy failed: %w", err)
	}

	waitResult := make(chan error, 1)
	go func() {
		waitResult <- cmd.Wait()
	}()

	startupGrace := wlCopyStartupGracePeriod
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < startupGrace {
			startupGrace = remaining
		}
	}
	if startupGrace <= 0 {
		startupGrace = time.Millisecond
	}

	timer := time.NewTimer(startupGrace)
	defer timer.Stop()

	select {
	case err := <-waitResult:
		if err == nil {
			return nil
		}
		output := strings.TrimSpace(stderr.String())
		if output == "" {
			return fmt.Errorf("wl-copy failed: %w", err)
		}
		return fmt.Errorf("wl-copy failed: %w: %s", err, output)
	case <-timer.C:
		return nil
	}
}
