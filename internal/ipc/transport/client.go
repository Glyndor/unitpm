// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/google/uuid"
)

// Client handles communication with the daemon.
type Client struct {
	conn    net.Conn
	scanner *bufio.Scanner
	encoder jsonx.Encoder
}

// NewClient establishes a connection to the daemon.
func NewClient() (IPCClient, error) {
	path, err := GetSocketPath()
	if err != nil {
		return nil, err
	}

	conn, err := dial(path, 5*time.Second)
	if err != nil {
		return nil, daemonUnreachable(path, err)
	}

	scanner := bufio.NewScanner(conn)
	// Enforce MaxMsgSize for responses too
	scanner.Buffer(make([]byte, 4096), MaxMsgSize)

	return &Client{
		conn:    conn,
		scanner: scanner,
		encoder: jsonx.NewEncoder(conn),
	}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Call sends a request and waits for a response.
func (c *Client) Call(command string, params any, result any) error {
	reqID := generateID()

	if err := c.sendRequest(reqID, command, params); err != nil {
		return err
	}

	return c.readResponse(reqID, result)
}

func (c *Client) sendRequest(reqID string, command string, params any) error {
	// Marshal params
	var paramBytes jsonx.RawMessage
	if params != nil {
		b, err := jsonx.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params error: %w", err)
		}
		paramBytes = b
	}

	req := protocol.Request{
		Version: protocol.Version,
		ID:      reqID,
		Command: command,
		Params:  paramBytes,
	}

	// Set write deadline
	if err := c.conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		return fmt.Errorf("set write deadline error: %w", err)
	}

	if err := c.encoder.Encode(req); err != nil {
		return fmt.Errorf("send error: %w", err)
	}
	return nil
}

func (c *Client) readResponse(reqID string, result any) error {
	// Read response
	// Set read deadline
	if err := c.conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		return fmt.Errorf("set read deadline error: %w", err)
	}

	if !c.scanner.Scan() {
		if err := c.scanner.Err(); err != nil {
			return fmt.Errorf("receive error: %w", err)
		}
		return errors.New("connection closed by server")
	}

	var resp protocol.Response
	if err := jsonx.Unmarshal(c.scanner.Bytes(), &resp); err != nil {
		return fmt.Errorf("receive error (invalid json): %w", err)
	}

	if resp.ID != reqID {
		return fmt.Errorf("response ID mismatch: got %s, want %s", resp.ID, reqID)
	}

	if err := c.checkStatus(&resp); err != nil {
		return err
	}

	if result != nil && resp.Result != nil {
		if err := jsonx.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("result unmarshal error: %w", err)
		}
	}

	return nil
}

func (c *Client) checkStatus(resp *protocol.Response) error {
	if resp.Status != protocol.StatusError {
		return nil
	}

	if resp.Error == nil {
		return errors.New("unknown ipc error")
	}

	errData := resp.Error.Data

	// Attempt to decode ProtocolMismatchData if the code matches
	if resp.Error.Code == "PROTOCOL_MISMATCH" {
		// The Data field is likely a map[string]interface{} (from json decoding into any)
		// We need to re-encode and decode it into the struct to be safe and clean.
		if dataBytes, err := jsonx.Marshal(resp.Error.Data); err == nil {
			var mismatchData protocol.MismatchData
			if err := jsonx.Unmarshal(dataBytes, &mismatchData); err == nil {
				errData = mismatchData
			}
		}
	}

	return &protocol.RemoteError{
		Code:    resp.Error.Code,
		Message: resp.Error.Message,
		Data:    errData,
	}
}

func generateID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// daemonUnreachable replaces the raw Unix-socket error with a message that
// tells the user how to start lynxd. Falls through to the original error for
// unrelated failures.
func daemonUnreachable(path string, err error) error {
	msg := err.Error()
	connectionRefused := strings.Contains(msg, "connection refused")
	noSocket := strings.Contains(msg, "no such file or directory")
	if !connectionRefused && !noSocket {
		return err
	}

	userMode := strings.Contains(path, "/run/user/") || strings.Contains(path, os.Getenv("XDG_RUNTIME_DIR"))
	var hint string
	if userMode {
		hint = "Start the daemon in the background:\n    lynxd &\n  Or enable user-mode startup:\n    lynx startup"
	} else {
		hint = "Start the system daemon:\n    sudo systemctl start lynx.lynxd\n  If you just installed, also run:\n    sudo lynx startup"
	}

	return fmt.Errorf(
		"cannot reach the Lynx daemon at %s\n  %s\n\n  Original error: %s",
		path, hint, msg,
	)
}
