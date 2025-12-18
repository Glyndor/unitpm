package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Jaro-c/Lynx/internal/version"
)

const (
	// MaxConnections is the maximum number of concurrent connections allowed.
	MaxConnections = 100
	// ReadTimeout is the timeout for reading from a connection.
	ReadTimeout = 5 * time.Second
	// WriteTimeout is the timeout for writing to a connection.
	WriteTimeout = 2 * time.Second
	// MaxMsgSize is the maximum size of a message.
	MaxMsgSize = 1024 * 1024 // 1MB
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
	buf := make([]byte, 4096)
	scanner.Buffer(buf, MaxMsgSize)

	encoder := json.NewEncoder(conn)

	for {
		// Set read deadline for the next request
		// If the client keeps the connection open but sends nothing, we shouldn't timeout immediately
		// unless we want to enforce an idle timeout.
		// "Read/write deadlines" usually implies per-operation.
		// If we want a persistent connection, we might set a long idle timeout.
		// Let's set an idle timeout of 60 seconds.
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			return
		}

		if !scanner.Scan() {
			return
		}

		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			return
		}

		resp := s.dispatch(&req)

		// Set write deadline
		if err := conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
			return
		}

		if err := encoder.Encode(resp); err != nil {
			return
		}
	}
}

func (s *Server) dispatch(req *Request) *Response {
	resp := &Response{
		ID: req.ID,
	}

	// Validate protocol version
	if req.Version != version.ProtocolVersion {
		resp.Status = statusError
		resp.Error = &Error{
			Code: "PROTOCOL_MISMATCH",
			Message: fmt.Sprintf(
				"Protocol mismatch: server v%d, client v%d",
				version.ProtocolVersion,
				req.Version,
			),
			Data: ProtocolMismatchData{
				Supported: version.ProtocolVersion,
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
		resp.Error = &Error{
			Code:    "UNKNOWN_COMMAND",
			Message: "Command not found",
		}
		return resp
	}

	res, err := handler(req.Params)
	if err != nil {
		resp.Status = statusError
		resp.Error = &Error{
			Code:    "INTERNAL_ERROR",
			Message: err.Error(),
		}
	} else {
		resp.Status = "success"
		resp.Result = res
	}

	return resp
}
