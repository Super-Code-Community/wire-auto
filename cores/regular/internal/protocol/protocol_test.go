package protocol

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestEncodeCompactLine(t *testing.T) {
	var b bytes.Buffer
	if err := Encode(&b, Message{Type: TypeHello, Protocol: 1, CoreAPI: 1}); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if got != `{"type":"hello","protocol":1,"coreApi":1}`+"\n" {
		t.Fatalf("bad encoding: %q", got)
	}
}

func TestDecodeSequence(t *testing.T) {
	in := strings.NewReader(
		`{"type":"ready"}` + "\n" +
			`{"type":"log","level":"info","message":"hi"}` + "\n" +
			`{"type":"done","exitCode":0}` + "\n")
	d := NewDecoder(in)

	m1, err := d.Next()
	if err != nil || m1.Type != TypeReady {
		t.Fatalf("m1: %+v %v", m1, err)
	}
	m2, err := d.Next()
	if err != nil || m2.Type != TypeLog || m2.Message != "hi" {
		t.Fatalf("m2: %+v %v", m2, err)
	}
	m3, err := d.Next()
	if err != nil || m3.Type != TypeDone || m3.ExitCode == nil || *m3.ExitCode != 0 {
		t.Fatalf("m3: %+v %v", m3, err)
	}
	if _, err := d.Next(); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestDecodeBadJSON(t *testing.T) {
	d := NewDecoder(strings.NewReader("not json\n"))
	if _, err := d.Next(); err == nil {
		t.Fatal("expected error on bad json")
	}
}

func TestDecodeSkipsEmptyLines(t *testing.T) {
	d := NewDecoder(strings.NewReader("\n\n" + `{"type":"ready"}` + "\n\n"))

	m, err := d.Next()
	if err != nil || m.Type != TypeReady {
		t.Fatalf("first: %+v %v", m, err)
	}
	if _, err := d.Next(); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestDecodeRejectsOversizeLine(t *testing.T) {
	d := NewDecoder(strings.NewReader(strings.Repeat("a", MaxLineBytes+10) + "\n"))
	_, err := d.Next()
	if err == nil || err == io.EOF {
		t.Fatalf("expected non-EOF error on oversize line, got %v", err)
	}
}
