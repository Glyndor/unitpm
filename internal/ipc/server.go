package ipc

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
	"time"
)

const (
	MaxConnections = 100
	ReadTimeout    = 5 * time.Second
	WriteTimeout   = 2 * time.Second
	MaxMsgSize     = 1024 * 1024 // 1MB
)

type CommandHandler func(params json.RawMessage) (json.RawMessage, error)

type Server struct {
	handlers map[string]CommandHandler
	mu       sync.RWMutex
	listener net.Listener
	sem      chan struct{} // semaphore for connection limiting
}

func NewServer() *Server {
	return &Server{
		handlers: make(map[string]CommandHandler),
		sem:      make(chan struct{}, MaxConnections),
	}
}

func (s *Server) Register(command string, handler CommandHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[command] = handler
}

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

	s.mu.RLock()
	handler, ok := s.handlers[req.Command]
	s.mu.RUnlock()

	if !ok {
		resp.Status = "error"
		resp.Error = &Error{Code: "UNKNOWN_COMMAND", Message: "Command not found"}
		return resp
	}

	res, err := handler(req.Params)
	if err != nil {
		resp.Status = "error"
		resp.Error = &Error{Code: "INTERNAL_ERROR", Message: err.Error()}
	} else {
		resp.Status = "success"
		resp.Result = res
	}

	return resp
}
