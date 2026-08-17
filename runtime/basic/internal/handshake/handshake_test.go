package handshake

import (
	"errors"
	"testing"

	"wire-auto/runtime/basic/internal/manifest"
)

func base() (manifest.Runtime, manifest.Core, manifest.Script) {
	rt := manifest.Runtime{Name: "rt", Protocols: []int{1}, Cores: []string{"regular"}}
	core := manifest.Core{Name: "regular", CoreAPI: 1, Protocol: 1, Provides: []string{}, Links: []string{"stdio"}}
	scr := manifest.Script{Name: "hello", Core: "regular", CoreAPI: 1, Link: "stdio", Cmd: []string{"python", "main.py"}, Language: "python", Capabilities: []string{}}
	return rt, core, scr
}

func TestReconcileOK(t *testing.T) {
	rt, core, scr := base()
	got, err := Reconcile(rt, core, scr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Protocol != 1 || got.CoreAPI != 1 {
		t.Fatalf("bad reconciled: %+v", got)
	}
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *Error, got %v", err)
	}
	return e.Code
}

func TestReconcileUnknownCore(t *testing.T) {
	rt, core, scr := base()
	scr.Core = "weird"
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "UNKNOWN_CORE" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileUnknownCoreWrongObject(t *testing.T) {
	rt, core, scr := base()
	rt.Cores = append(rt.Cores, "other")
	scr.Core = "other"
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "UNKNOWN_CORE" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileCoreAPIMismatch(t *testing.T) {
	rt, core, scr := base()
	scr.CoreAPI = 2
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "CORE_API_MISMATCH" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileCapabilityDenied(t *testing.T) {
	rt, core, scr := base()
	scr.Capabilities = []string{"serial"}
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "CAPABILITY_DENIED" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileProtocolUnsupported(t *testing.T) {
	rt, core, scr := base()
	core.Protocol = 2
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "PROTOCOL_UNSUPPORTED" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileLinkUnsupported(t *testing.T) {
	rt, core, scr := base()
	scr.Link = "weird"
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "LINK_UNSUPPORTED" {
		t.Fatalf("got %s", got)
	}
}
