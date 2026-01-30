package jsonx

import (
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
// We return a compatible interface or wrapper if needed, 
// but sonic.ConfigStd.NewEncoder returns a *sonic.Encoder which is compatible with *json.Encoder's Encode method.
func NewEncoder(w interface{}) Encoder {
	// sonic.ConfigStd.NewEncoder takes an io.Writer
	return sonic.ConfigStd.NewEncoder(w.(interface{ Write(p []byte) (n int, err error) }))
}

// Encoder is an interface that matches json.Encoder's Encode method
type Encoder interface {
	Encode(v interface{}) error
}

// NewDecoder returns a new decoder that reads from r.
func NewDecoder(r interface{}) Decoder {
	return sonic.ConfigStd.NewDecoder(r.(interface{ Read(p []byte) (n int, err error) }))
}

// Decoder is an interface that matches json.Decoder's Decode method
type Decoder interface {
	Decode(v interface{}) error
}

// RawMessage is a raw encoded JSON value.
// We alias it to []byte (standard) or use json.RawMessage if we want to avoid importing encoding/json in other files.
// However, sonic treats []byte effectively as RawMessage.
// But to keep compatibility with struct tags like `json:"params"`, we don't need to redefine RawMessage unless we use it as a type.
// In internal/ipc/transport/decode.go, json.RawMessage is used.
// We should import encoding/json there for the type, OR define it here.
// encoding/json.RawMessage is just type RawMessage []byte
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
		return nil // Should be an error? json.RawMessage returns errors.New("json.RawMessage: UnmarshalJSON on nil pointer")
	}
	*m = append((*m)[0:0], data...)
	return nil
}
