// Package jsonx provides JSON encoding and decoding utilities using sonic.
package jsonx

import (
	"io"

	"github.com/bytedance/sonic"
)

// Marshal returns the JSON encoding of v.
func Marshal(v interface{}) ([]byte, error) {
	return sonic.Marshal(v)
}

// Unmarshal parses the JSON-encoded data and stores the result
// in the value pointed to by v.
func Unmarshal(data []byte, v interface{}) error {
	return sonic.Unmarshal(data, v)
}

// MarshalIndent is like Marshal but applies Indent to format the output.
func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return sonic.ConfigStd.MarshalIndent(v, prefix, indent)
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) Encoder {
	return sonic.ConfigStd.NewEncoder(w)
}

// Encoder is an interface that matches json.Encoder's Encode method.
type Encoder interface {
	Encode(v interface{}) error
}

// NewDecoder returns a new decoder that reads from r.
func NewDecoder(r io.Reader) Decoder {
	return sonic.ConfigStd.NewDecoder(r)
}

// Decoder is an interface that matches json.Decoder's Decode method.
type Decoder interface {
	Decode(v interface{}) error
}

// RawMessage is a raw encoded JSON value.
type RawMessage []byte

// MarshalJSON returns m as the JSON encoding of m.
func (m RawMessage) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

// UnmarshalJSON sets *m to a copy of data.
func (m *RawMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return nil
	}
	*m = append((*m)[0:0], data...)
	return nil
}
