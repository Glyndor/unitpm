// Package transport implements the Inter-Process Communication transport layer.
package transport

import (
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/jsonx"
)

// ResponseDecoder handles decoding of responses.
type ResponseDecoder struct {
	decoder jsonx.Decoder
}

// NewResponseDecoder creates a new decoder.
func NewResponseDecoder(decoder jsonx.Decoder) *ResponseDecoder {
	return &ResponseDecoder{
		decoder: decoder,
	}
}

// Decode decodes a response.
func (d *ResponseDecoder) Decode(resp *protocol.Response) error {
	return d.decoder.Decode(resp)
}
