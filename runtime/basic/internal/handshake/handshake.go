// Package handshake сводит три паспорта (runtime, core, script) перед запуском.
// Первое несведение — стоп с машиночитаемым кодом.
package handshake

import "wire-auto/runtime/basic/internal/manifest"

type Reconciled struct {
	Protocol int
	CoreAPI  int
}

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func contains[T comparable](xs []T, v T) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// Reconcile проверяет совместимость сверху вниз согласно разделу 3 спеки.
func Reconcile(rt manifest.Runtime, core manifest.Core, scr manifest.Script) (Reconciled, error) {
	// The registry is the source of truth for core availability; only verify
	// that the script's target matches the core object passed in.
	if scr.Core != core.Name {
		return Reconciled{}, &Error{"UNKNOWN_CORE", "script targets core " + scr.Core + " which is not the reconciled core"}
	}
	if scr.CoreAPI != core.CoreAPI {
		return Reconciled{}, &Error{"CORE_API_MISMATCH", "script coreApi does not match core"}
	}
	for _, cap := range scr.Capabilities {
		if !contains(core.Provides, cap) {
			return Reconciled{}, &Error{"CAPABILITY_DENIED", "core does not provide capability " + cap}
		}
	}
	if !contains(rt.Protocols, core.Protocol) {
		return Reconciled{}, &Error{"PROTOCOL_UNSUPPORTED", "runtime does not speak core protocol"}
	}
	if !contains(core.Links, scr.Link) {
		return Reconciled{}, &Error{"LINK_UNSUPPORTED", "core does not support link " + scr.Link}
	}
	return Reconciled{Protocol: core.Protocol, CoreAPI: core.CoreAPI}, nil
}
