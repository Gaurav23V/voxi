package output

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

type Notifier interface {
	Notify(ctx context.Context, title, body string) error
}

type NotifySend struct {
	Command   string
	TimeoutMS int
}

func NewNotifySend(timeoutMS int) *NotifySend {
	return &NotifySend{
		Command:   "notify-send",
		TimeoutMS: timeoutMS,
	}
}

func (n *NotifySend) Notify(ctx context.Context, title, body string) error {
	command := n.Command
	if command == "" {
		command = "notify-send"
	}

	args := []string{"-t", strconv.Itoa(n.TimeoutMS), title}
	if body != "" {
		args = append(args, body)
	}

	cmd := exec.CommandContext(ctx, command, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("notify-send failed: %w: %s", err, string(output))
	}

	return nil
}
