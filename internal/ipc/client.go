package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
)

func Call(ctx context.Context, socketPath string, request any, response any) error {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("dial %s: %w", socketPath, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(request); err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(response); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
