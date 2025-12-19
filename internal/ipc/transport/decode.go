package transport

import "encoding/json"

// UniversalRequest helps determines the request type.
type UniversalRequest struct {
	Version         int             `json:"version"`
	ProtocolVersion int             `json:"protocol_version"`
	ID              string          `json:"id"`
	RequestID       string          `json:"request_id"`
	Command         string          `json:"command"`
	Type            string          `json:"type"`
	Params          json.RawMessage `json:"params"`
	Spec            json.RawMessage `json:"spec"`
}
