// Package ipc implements the Inter-Process Communication between lynx CLI and daemon.
package ipc

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Client handles communication with the daemon.
type Client struct {
	conn    net.Conn
	scanner *bufio.Scanner
	encoder *json.Encoder
}

// NewClient establishes a connection to the daemon.
func NewClient() (*Client, error) {
	path, err := GetSocketPath()
	if err != nil {
		return nil, err
	}

	conn, err := dial(path, 5*time.Second)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(conn)
	// Enforce 1MB max message size for responses too
	scanner.Buffer(make([]byte, 4096), 1024*1024)

	return &Client{
		conn:    conn,
		scanner: scanner,
		encoder: json.NewEncoder(conn),
	}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Call sends a request and waits for a response
func (c *Client) Call(command string, params interface{}, result interface{}) error {
	reqID := generateID()

	// Marshal params
	var paramBytes json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params error: %w", err)
		}
		paramBytes = b
	}

	req := Request{
		Version:   Version,
		ID:        reqID,
		Command:   command,
		Params:    paramBytes,
		Timestamp: time.Now().Unix(),
	}

	// Set write deadline
	if err := c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return fmt.Errorf("set write deadline error: %w", err)
	}

	if err := c.encoder.Encode(req); err != nil {
		return fmt.Errorf("send error: %w", err)
	}

	// Read response
	// Set read deadline
	if err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set read deadline error: %w", err)
	}

	if !c.scanner.Scan() {
		if err := c.scanner.Err(); err != nil {
			return fmt.Errorf("receive error: %w", err)
		}
		return fmt.Errorf("connection closed by server")
	}

	var resp Response
	if err := json.Unmarshal(c.scanner.Bytes(), &resp); err != nil {
		return fmt.Errorf("receive error (invalid json): %w", err)
	}

	if resp.ID != req.ID {
		return fmt.Errorf("response ID mismatch: got %s, want %s", resp.ID, req.ID)
	}

	if resp.Status == "error" {
		if resp.Error != nil {
			return fmt.Errorf("ipc error: [%s] %s", resp.Error.Code, resp.Error.Message)
		}
		return fmt.Errorf("unknown ipc error")
	}

	if result != nil && resp.Result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("result unmarshal error: %w", err)
		}
	}

	return nil
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
