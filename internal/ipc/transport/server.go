// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/jsonx"
)

// Identity represents the authenticated identity of an IPC client.
type Identity struct {
	UID string // User ID
	GID string // Group ID
	PID int    // Process ID
}

type contextKey string

// ContextKeyIdentity is the context key for the client identity.
const ContextKeyIdentity contextKey = "identity"

// CommandHandler is a function that handles an IPC command.
type CommandHandler func(ctx context.Context, params jsonx.RawMessage) (jsonx.RawMessage, error)

const (
	statusError   = "error"
	statusSuccess = "success"
)

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

	identity, err := validateIdentity(conn)
	if err != nil {
		return
	}

	// Create context with identity
	ctx := context.WithValue(context.Background(), ContextKeyIdentity, identity)

	// Use bufio.Scanner to enforce newline-delimited messages and size limits
	scanner := bufio.NewScanner(conn)
	// Explicitly set buffer size and max token size (MaxMsgSize)
	buf := make([]byte, 4096)
	scanner.Buffer(buf, MaxMsgSize)

	encoder := jsonx.NewEncoder(conn)

	for {
		if !s.handleRequest(ctx, conn, scanner, encoder) {
			return
		}
	}
}

func (s *Server) handleRequest(
	ctx context.Context,
	conn net.Conn,
	scanner *bufio.Scanner,
	encoder jsonx.Encoder,
) bool {
	// Set read deadline per request
	if err := conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		return false
	}

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			if errors.Is(err, bufio.ErrTooLong) {
				s.sendError(encoder, "ERR_LIMITS", "Message too large")
			} else if errors.Is(err, os.ErrDeadlineExceeded) {
				s.sendError(encoder, "ERR_TIMEOUT", "Read timed out")
			}
		}
		return false
	}

	// Decode into UniversalRequest to determine type
	var univReq UniversalRequest
	if err := jsonx.Unmarshal(scanner.Bytes(), &univReq); err != nil {
		s.sendError(encoder, "ERR_BAD_REQUEST", "Invalid JSON")
		return false
	}

	var resp any

	if univReq.Type == "start" {
		resp = s.dispatchStart(ctx, &univReq)
	} else {
		req := &protocol.Request{
			Version: univReq.Version,
			ID:      univReq.ID,
			Command: univReq.Command,
			Params:  univReq.Params,
		}
		resp = s.dispatch(ctx, req)
	}

	// Set write deadline
	if err := conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		return false
	}

	if err := encoder.Encode(resp); err != nil {
		return false
	}

	return true
}

func (s *Server) sendError(encoder jsonx.Encoder, code, message string) {
	resp := &protocol.Response{
		Status: statusError,
		Error: &protocol.Error{
			Code:    code,
			Message: message,
		},
	}
	if err := encoder.Encode(resp); err != nil {
		// Connection likely broken or closed
		return
	}
}

func (s *Server) dispatchStart(ctx context.Context, req *UniversalRequest) *protocol.StartResponse {
	resp := &protocol.StartResponse{
		ProtocolVersion: protocol.Version,
		Type:            "start_result",
		RequestID:       req.RequestID,
	}

	// Validate protocol version
	if req.ProtocolVersion != protocol.Version {
		resp.Ok = false
		resp.Error = &protocol.StartError{
			Code: "PROTOCOL_MISMATCH",
			Message: fmt.Sprintf(
				"Protocol mismatch: server v%d, client v%d",
				protocol.Version,
				req.ProtocolVersion,
			),
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

	res, err := handler(ctx, req.Spec)
	if err != nil {
		resp.Ok = false
		resp.Error = &protocol.StartError{
			Code:    "INTERNAL_ERROR",
			Message: err.Error(),
		}
	} else {
		resp.Ok = true
		var data protocol.StartResponseData
		if err := jsonx.Unmarshal(res, &data); err != nil {
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

func (s *Server) dispatch(ctx context.Context, req *protocol.Request) *protocol.Response {
	resp := &protocol.Response{
		ID: req.ID,
	}

	// Validate protocol version
	if req.Version != protocol.Version {
		resp.Status = statusError
		resp.Error = &protocol.Error{
			Code: "PROTOCOL_MISMATCH",
			Message: fmt.Sprintf(
				"Protocol mismatch: server v%d, client v%d",
				protocol.Version,
				req.Version,
			),
			Data: protocol.MismatchData{
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
		resp.Status = statusError
		resp.Error = &protocol.Error{
			Code:    "UNKNOWN_COMMAND",
			Message: "Command not found",
		}
		return resp
	}

	res, err := handler(ctx, req.Params)
	if err != nil {
		resp.Status = statusError
		resp.Error = &protocol.Error{
			Code:    "INTERNAL_ERROR",
			Message: err.Error(),
		}
	} else {
		resp.Status = statusSuccess
		resp.Result = res
	}

	return resp
}
