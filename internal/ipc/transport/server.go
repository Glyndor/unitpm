// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// CommandHandler is a function that handles an IPC command.
type CommandHandler func(params json.RawMessage) (json.RawMessage, error)

// Server accepts connections and dispatches commands.
type Server struct {
	handlers map[string]CommandHandler
	mu       sync.RWMutex
	listener net.Listener
	sem      chan struct{} // semaphore for connection limiting
}

// NewServer creates a new IPC server.
func NewServer() *Server {
	return &Server{
		handlers: make(map[string]CommandHandler),
		sem:      make(chan struct{}, MaxConnections),
	}
}

// Register registers a handler for a command.
func (s *Server) Register(command string, handler CommandHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[command] = handler
}

// Start begins listening for connections.
func (s *Server) Start() error {
	path, err := GetSocketPath()
	if err != nil {
		return err
	}

	l, err := listen(path)
	if err != nil {
		return err
	}
	s.listener = l

	go s.acceptLoop()
	return nil
}

// Close stops the server and closes the listener.
func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Listener closed or error
			return
		}

		// Acquire semaphore
		select {
		case s.sem <- struct{}{}:
			go s.handleConnection(conn)
		default:
			// Too many connections
			_ = conn.Close()
		}
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		<-s.sem // Release semaphore
		_ = conn.Close()
	}()

	if err := validateIdentity(conn); err != nil {
		return
	}

	// Use bufio.Scanner to enforce newline-delimited messages and size limits
	scanner := bufio.NewScanner(conn)
	// Explicitly set buffer size and max token size (MaxMsgSize)
	buf := make([]byte, 4096)
	scanner.Buffer(buf, MaxMsgSize)

	encoder := json.NewEncoder(conn)

	for {
		// Set read deadline per request
		if err := conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
			return
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				if errors.Is(err, bufio.ErrTooLong) {
					s.sendError(encoder, "ERR_LIMITS", "Message too large")
				} else if errors.Is(err, os.ErrDeadlineExceeded) {
					s.sendError(encoder, "ERR_TIMEOUT", "Read timed out")
				}
			}
			return
		}

		// Decode into UniversalRequest to determine type
		var univReq UniversalRequest
		if err := json.Unmarshal(scanner.Bytes(), &univReq); err != nil {
			s.sendError(encoder, "ERR_BAD_REQUEST", "Invalid JSON")
			return
		}

		var resp any

		if univReq.Type == "start" {
			resp = s.dispatchStart(&univReq)
		} else {
			req := &protocol.Request{
				Version: univReq.Version,
				ID:      univReq.ID,
				Command: univReq.Command,
				Params:  univReq.Params,
			}
			resp = s.dispatch(req)
		}

		// Set write deadline
		if err := conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
			return
		}

		if err := encoder.Encode(resp); err != nil {
			return
		}
	}
}

func (s *Server) sendError(encoder *json.Encoder, code, message string) {
	resp := &protocol.Response{
		Status: "error",
		Error: &protocol.Error{
			Code:    code,
			Message: message,
		},
	}
	_ = encoder.Encode(resp)
}

func (s *Server) dispatchStart(req *UniversalRequest) *protocol.StartResponse {
	resp := &protocol.StartResponse{
		ProtocolVersion: protocol.Version,
		Type:            "start_result",
		RequestID:       req.RequestID,
	}

	// Validate protocol version
	if req.ProtocolVersion != protocol.Version {
		resp.Ok = false
		resp.Error = &protocol.StartError{
			Code:    "PROTOCOL_MISMATCH",
			Message: fmt.Sprintf("Protocol mismatch: server v%d, client v%d", protocol.Version, req.ProtocolVersion),
		}
		return resp
	}

	s.mu.RLock()
	handler, ok := s.handlers["start"]
	s.mu.RUnlock()

	if !ok {
		resp.Ok = false
		resp.Error = &protocol.StartError{
			Code:    "UNKNOWN_COMMAND",
			Message: "Command start not found",
		}
		return resp
	}

	res, err := handler(req.Spec)
	if err != nil {
		resp.Ok = false
		resp.Error = &protocol.StartError{
			Code:    "INTERNAL_ERROR",
			Message: err.Error(),
		}
	} else {
		resp.Ok = true
		var data protocol.StartResponseData
		if err := json.Unmarshal(res, &data); err != nil {
			resp.Ok = false
			resp.Error = &protocol.StartError{
				Code:    "INTERNAL_ERROR",
				Message: "Failed to encode response data",
			}
		} else {
			resp.Data = &data
		}
	}

	return resp
}

func (s *Server) dispatch(req *protocol.Request) *protocol.Response {
	resp := &protocol.Response{
		ID: req.ID,
	}

	// Validate protocol version
	if req.Version != protocol.Version {
		resp.Status = "error"
		resp.Error = &protocol.Error{
			Code: "PROTOCOL_MISMATCH",
			Message: fmt.Sprintf(
				"Protocol mismatch: server v%d, client v%d",
				protocol.Version,
				req.Version,
			),
			Data: protocol.ProtocolMismatchData{
				Supported: protocol.Version,
				Received:  req.Version,
			},
		}
		return resp
	}

	s.mu.RLock()
	handler, ok := s.handlers[req.Command]
	s.mu.RUnlock()

	if !ok {
		resp.Status = "error"
		resp.Error = &protocol.Error{
			Code:    "UNKNOWN_COMMAND",
			Message: "Command not found",
		}
		return resp
	}

	res, err := handler(req.Params)
	if err != nil {
		resp.Status = "error"
		resp.Error = &protocol.Error{
			Code:    "INTERNAL_ERROR",
			Message: err.Error(),
		}
	} else {
		resp.Status = "success"
		resp.Result = res
	}

	return resp
}
