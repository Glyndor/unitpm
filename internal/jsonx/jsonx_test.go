package jsonx

import (
	"bytes"
	"strings"
	"testing"
)

type sample struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestMarshal_Unmarshal_Roundtrip(t *testing.T) {
	s := sample{Name: "api", Value: 42}
	b, err := Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var got sample
	if err := Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != s {
		t.Errorf("roundtrip mismatch: %+v != %+v", got, s)
	}
}

func TestUnmarshal_Invalid(t *testing.T) {
	var s sample
	if err := Unmarshal([]byte("not json"), &s); err == nil {
		t.Error("expected error on invalid JSON")
	}
}

func TestRawMessage(t *testing.T) {
	raw := RawMessage(`{"k":1}`)
	b, err := Marshal(struct {
		Inner RawMessage `json:"inner"`
	}{Inner: raw})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"inner":{"k":1}`) {
		t.Errorf("raw message did not pass through: %s", b)
	}
}

func TestEncoder(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(sample{Name: "x", Value: 1}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"name":"x"`) {
		t.Errorf("encoder output missing data: %q", out)
	}
}

func TestDecoder(t *testing.T) {
	buf := bytes.NewBufferString(`{"name":"y","value":7}`)
	dec := NewDecoder(buf)
	var s sample
	if err := dec.Decode(&s); err != nil {
		t.Fatal(err)
	}
	if s.Name != "y" || s.Value != 7 {
		t.Errorf("unexpected: %+v", s)
	}
}

func TestMarshalIndent(t *testing.T) {
	b, err := MarshalIndent(sample{Name: "z", Value: 3}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "\n") || !strings.Contains(s, `  "name"`) {
		t.Errorf("expected indented output, got %q", s)
	}
}

func TestRawMessage_MarshalJSON_Empty(t *testing.T) {
	var raw RawMessage
	b, err := raw.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "null" {
		t.Errorf("empty RawMessage should marshal to null, got %q", b)
	}
}

func TestRawMessage_UnmarshalJSON(t *testing.T) {
	var raw RawMessage
	if err := raw.UnmarshalJSON([]byte(`{"x":1}`)); err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"x":1}` {
		t.Errorf("unexpected raw: %s", raw)
	}

	// Nil receiver is a no-op (matches encoding/json convention).
	var nilRaw *RawMessage
	if err := nilRaw.UnmarshalJSON([]byte(`1`)); err != nil {
		t.Errorf("nil receiver should be a no-op, got %v", err)
	}
}
