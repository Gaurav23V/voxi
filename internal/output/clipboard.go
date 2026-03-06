package output

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Clipboard interface {
	Copy(context.Context, string) error
}

type WLCopyClipboard struct {
	Command string
}

func NewWLCopyClipboard() *WLCopyClipboard {
	command := os.Getenv("VOXI_WL_COPY_COMMAND")
	if command == "" {
		command = "wl-copy"
	}
	return &WLCopyClipboard{Command: command}
}

func (c *WLCopyClipboard) Copy(ctx context.Context, text string) error {
	command := c.Command
	if command == "" {
		command = "wl-copy"
	}

	cmd := exec.CommandContext(ctx, command)
	cmd.Stdin = strings.NewReader(text)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wl-copy failed: %w: %s", err, string(output))
	}
	return nil
}
