package protocol

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestRequestResponseRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	req := Message{Type: TypeRequest, ID: "1", Capability: "env.get", Params: json.RawMessage(`{"name":"USER"}`)}
	if err := Encode(&buf, req); err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec := NewDecoder(&buf)
	got, err := dec.Next()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Type != TypeRequest || got.ID != "1" || got.Capability != "env.get" {
		t.Fatalf("got %+v", got)
	}
	if string(got.Params) != `{"name":"USER"}` {
		t.Fatalf("params = %s", got.Params)
	}
}

func TestResponseErrorEncoding(t *testing.T) {
	var buf bytes.Buffer
	_ = Encode(&buf, Message{Type: TypeResponse, ID: "1", Code: "ENV_NOT_FOUND", Message: "env var not set: USER"})
	// v1 fields must stay omitempty: no "protocol":0, no "params" key here.
	line := buf.String()
	if want := `{"type":"response","message":"env var not set: USER","id":"1","code":"ENV_NOT_FOUND"}` + "\n"; line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}

func TestDecoderEOF(t *testing.T) {
	dec := NewDecoder(bytes.NewReader(nil))
	if _, err := dec.Next(); err != io.EOF {
		t.Fatalf("err = %v, want EOF", err)
	}
}
