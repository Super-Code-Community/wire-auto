package handshake

import (
	"errors"
	"testing"

	"wire-auto/cores/regular/internal/manifest"
)

func base() (manifest.Core, manifest.Script) {
	core := manifest.Core{Name: "regular", CoreAPI: 1, Protocol: 1, Links: []string{"stdio"}}
	scr := manifest.Script{Name: "hello", Core: "regular", CoreAPI: 1, Link: "stdio", Cmd: []string{"python", "main.py"}, Language: "python", Capabilities: []string{}}
	return core, scr
}

func TestReconcileOK(t *testing.T) {
	core, scr := base()
	got, err := Reconcile(core, scr, nil)
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
	core, scr := base()
	scr.Core = "weird"
	_, err := Reconcile(core, scr, nil)
	if got := codeOf(t, err); got != "UNKNOWN_CORE" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileCoreAPIMismatch(t *testing.T) {
	core, scr := base()
	scr.CoreAPI = 2
	_, err := Reconcile(core, scr, nil)
	if got := codeOf(t, err); got != "CORE_API_MISMATCH" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileCapabilityDenied(t *testing.T) {
	core, scr := base()
	scr.Capabilities = []string{"serial"}
	_, err := Reconcile(core, scr, nil)
	if got := codeOf(t, err); got != "CAPABILITY_DENIED" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileLinkUnsupported(t *testing.T) {
	core, scr := base()
	scr.Link = "weird"
	_, err := Reconcile(core, scr, nil)
	if got := codeOf(t, err); got != "LINK_UNSUPPORTED" {
		t.Fatalf("got %s", got)
	}
}
