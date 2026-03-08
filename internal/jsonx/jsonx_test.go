package jsonx

import (
	"bytes"
	"testing"
)

func TestMarshalUnmarshal(t *testing.T) {
	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	original := TestStruct{Name: "test", Value: 123}

	// Test Marshal
	data, err := Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Test Unmarshal
	var result TestStruct
	if err := Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result != original {
		t.Errorf("Expected %v, got %v", original, result)
	}
}

func TestMarshalIndent(t *testing.T) {
	data := map[string]string{"foo": "bar"}
	bytes, err := MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	expected := "{\n  \"foo\": \"bar\"\n}"
	if string(bytes) != expected {
		t.Errorf("Expected %q, got %q", expected, string(bytes))
	}
}

func TestEncoderDecoder(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	
	input := map[string]int{"a": 1}
	if err := enc.Encode(input); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	dec := NewDecoder(&buf)
	var output map[string]int
	if err := dec.Decode(&output); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if output["a"] != 1 {
		t.Errorf("Expected 1, got %d", output["a"])
	}
}

func TestRawMessage(t *testing.T) {
	raw := RawMessage(`{"foo":"bar"}`)
	data, err := raw.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if string(data) != `{"foo":"bar"}` {
		t.Errorf("Expected raw json, got %s", string(data))
	}

	var m RawMessage
	if err := m.UnmarshalJSON([]byte(`"test"`)); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if string(m) != `"test"` {
		t.Errorf("Expected \"test\", got %s", string(m))
	}
}
