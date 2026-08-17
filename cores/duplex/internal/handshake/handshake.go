// Package handshake сводит два паспорта (core и script) перед запуском.
// Первое несведение — стоп с машиночитаемым кодом. runtime.manifest упразднён.
package handshake

import "wire-auto/cores/duplex/internal/manifest"

type Reconciled struct {
	Protocol int
	CoreAPI  int
	Provides []string
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

// Reconcile сводит контракт бандла (core) со скриптом. provides подаёт
// вызывающий (ключи capreg-реестра); capability скрипта обязаны в нём лежать.
func Reconcile(core manifest.Core, scr manifest.Script, provides []string) (Reconciled, error) {
	if scr.Core != core.Name {
		return Reconciled{}, &Error{"UNKNOWN_CORE", "script targets core " + scr.Core + " which is not this core"}
	}
	if scr.CoreAPI != core.CoreAPI {
		return Reconciled{}, &Error{"CORE_API_MISMATCH", "script coreApi does not match core"}
	}
	for _, cap := range scr.Capabilities {
		if !contains(provides, cap) {
			return Reconciled{}, &Error{"CAPABILITY_DENIED", "core does not provide capability " + cap}
		}
	}
	if !contains(core.Links, scr.Link) {
		return Reconciled{}, &Error{"LINK_UNSUPPORTED", "core does not support link " + scr.Link}
	}
	return Reconciled{Protocol: core.Protocol, CoreAPI: core.CoreAPI, Provides: provides}, nil
}
