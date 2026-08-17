package exec

import (
	"context"
	"strings"
	"testing"
	"time"

	"wire-auto/cores/duplex/internal/capreg"
)

func v2Spec(dir string) Spec {
	return Spec{
		Dir:            dir,
		Command:        "python",
		Args:           []string{"main.py"},
		Protocol:       2,
		CoreAPI:        1,
		StartupTimeout: 5 * time.Second,
		RunTimeout:     5 * time.Second,
		Provides:       []string{"env.get"},
		Registry:       capreg.Default,
	}
}

func TestRequestResponseHappyPath(t *testing.T) {
	t.Setenv("WIRE_ECHO_VAR", "duplex-works")
	res := Run(context.Background(), v2Spec("testdata/env-echo"))
	if res.Status != StatusOK {
		t.Fatalf("status=%s err=%s", res.Status, res.ErrorMessage)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "duplex-works") {
		t.Fatalf("logs=%+v", res.Logs)
	}
}

func TestRequestCapabilityDenied(t *testing.T) {
	t.Setenv("WIRE_ECHO_VAR", "should-not-matter")
	spec := v2Spec("testdata/env-echo")
	spec.Provides = []string{} // env.get no longer authorized
	res := Run(context.Background(), spec)
	if res.Status != StatusOK {
		t.Fatalf("status=%s (script handles denial gracefully)", res.Status)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "<denied:CAPABILITY_DENIED>") {
		t.Fatalf("logs=%+v, want denial marker", res.Logs)
	}
}

func TestRequestOnProtocol1IsViolation(t *testing.T) {
	spec := v2Spec("testdata/env-echo")
	spec.Protocol = 1 // v1 script has no right to send request
	res := Run(context.Background(), spec)
	if res.Status != StatusProtocolViolation {
		t.Fatalf("status=%s, want PROTOCOL_VIOLATION", res.Status)
	}
}

func TestPlainV1FlowStillWorks(t *testing.T) {
	// A protocol-2 spec whose script never sends request behaves exactly like v1.
	res := Run(context.Background(), v2Spec("testdata/plain"))
	if res.Status != StatusOK {
		t.Fatalf("status=%s err=%s", res.Status, res.ErrorMessage)
	}
	if len(res.Logs) != 1 || res.Logs[0].Message != "no request here" {
		t.Fatalf("logs=%+v", res.Logs)
	}
}

func TestPromptRoundTrip(t *testing.T) {
	spec := v2Spec("testdata/ask")
	answers := make(chan PromptAnswer, 1)
	spec.Answers = answers
	var got Event
	spec.OnEvent = func(e Event) {
		if e.Kind == "prompt" {
			got = e
			answers <- PromptAnswer{ID: e.ID, Value: "Alice"}
		}
	}
	res := Run(context.Background(), spec)
	if res.Status != StatusOK {
		t.Fatalf("status=%s err=%s", res.Status, res.ErrorMessage)
	}
	if got.Kind != "prompt" || got.ID != "1" || got.Message != "name?" {
		t.Fatalf("prompt event=%+v", got)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "hello Alice") {
		t.Fatalf("logs=%+v", res.Logs)
	}
}

func TestPromptBeforeReadyIsViolation(t *testing.T) {
	res := Run(context.Background(), v2Spec("testdata/ask-early"))
	if res.Status != StatusProtocolViolation {
		t.Fatalf("status=%s, want PROTOCOL_VIOLATION", res.Status)
	}
}

func TestPromptIgnoresMismatchedAnswerID(t *testing.T) {
	spec := v2Spec("testdata/ask")
	answers := make(chan PromptAnswer, 2)
	spec.Answers = answers
	spec.OnEvent = func(e Event) {
		if e.Kind == "prompt" {
			answers <- PromptAnswer{ID: "99", Value: "wrong"} // не тот id — должен игнорироваться
			answers <- PromptAnswer{ID: e.ID, Value: "right"}  // верный id
		}
	}
	res := Run(context.Background(), spec)
	if res.Status != StatusOK {
		t.Fatalf("status=%s err=%s", res.Status, res.ErrorMessage)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "hello right") {
		t.Fatalf("logs=%+v, want 'hello right'", res.Logs)
	}
}
