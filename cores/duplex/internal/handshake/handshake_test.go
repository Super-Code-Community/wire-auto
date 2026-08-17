package handshake

import (
	"errors"
	"testing"

	"wire-auto/cores/duplex/internal/manifest"
)

func duplexCore() manifest.Core {
	return manifest.Core{Name: "duplex", CoreAPI: 1, Protocol: 2, Links: []string{"stdio"}}
}
func duplexScript() manifest.Script {
	return manifest.Script{Name: "s", Core: "duplex", CoreAPI: 1, Link: "stdio", Cmd: []string{"python", "main.py"}, Capabilities: []string{"env.get"}}
}

func TestReconcileCarriesProvides(t *testing.T) {
	r, err := Reconcile(duplexCore(), duplexScript(), []string{"env.get"})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if r.Protocol != 2 {
		t.Fatalf("protocol = %d, want 2", r.Protocol)
	}
	if len(r.Provides) != 1 || r.Provides[0] != "env.get" {
		t.Fatalf("provides = %v, want [env.get]", r.Provides)
	}
}

func TestReconcileCapabilityDenied(t *testing.T) {
	_, err := Reconcile(duplexCore(), duplexScript(), []string{})
	var he *Error
	if !errors.As(err, &he) || he.Code != "CAPABILITY_DENIED" {
		t.Fatalf("err = %v, want CAPABILITY_DENIED", err)
	}
}
