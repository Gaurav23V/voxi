package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
)

type Handler func(context.Context, json.RawMessage) (any, error)

type Server struct {
	socketPath string
	handler    Handler

	listener net.Listener
	wg       sync.WaitGroup
}

func NewServer(socketPath string, handler Handler) *Server {
	return &Server{
		socketPath: socketPath,
		handler:    handler,
	}
}

func (s *Server) Start() error {
	if err := os.RemoveAll(s.socketPath); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socketPath, err)
	}
	s.listener = listener

	s.wg.Add(1)
	go s.serve()
	return nil
}

func (s *Server) Close() error {
	if s.listener == nil {
		return nil
	}
	err := s.listener.Close()
	s.wg.Wait()
	removeErr := os.Remove(s.socketPath)
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return errors.Join(err, removeErr)
	}
	return err
}

func (s *Server) serve() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()

			decoder := json.NewDecoder(conn)
			var payload json.RawMessage
			if err := decoder.Decode(&payload); err != nil {
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"ok":      false,
					"message": fmt.Sprintf("invalid request: %v", err),
				})
				return
			}

			response, err := s.handler(context.Background(), payload)
			if err != nil {
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"ok":      false,
					"message": err.Error(),
				})
				return
			}

			_ = json.NewEncoder(conn).Encode(response)
		}()
	}
}
