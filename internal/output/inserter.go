package output

import (
	"context"
	"fmt"
	"os/exec"
)

type Inserter interface {
	Insert(context.Context, string) error
}

type WTypeInserter struct {
	Command string
}

func NewWTypeInserter() *WTypeInserter {
	return &WTypeInserter{Command: "wtype"}
}

func (i *WTypeInserter) Insert(ctx context.Context, text string) error {
	command := i.Command
	if command == "" {
		command = "wtype"
	}

	cmd := exec.CommandContext(ctx, command, text)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wtype failed: %w: %s", err, string(output))
	}
	return nil
}
