package transport

import "github.com/Jaro-c/Lynx/internal/jsonx"

// UniversalRequest helps determines the request type.
type UniversalRequest struct {
	Version         int               `json:"version"`
	ProtocolVersion int               `json:"protocol_version"`
	ID              string            `json:"id"`
	RequestID       string            `json:"request_id"`
	Command         string            `json:"command"`
	Type            string            `json:"type"`
	Params          jsonx.RawMessage  `json:"params"`
	Spec            jsonx.RawMessage  `json:"spec"`
}
