// Package manifest читает и валидирует три вида паспортов платформы:
// runtime, core и script. Формат — TOML.
package manifest

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type Runtime struct {
	Name       string   `toml:"name"`
	Version    string   `toml:"version"`
	Protocols  []int    `toml:"protocols"`
	Transports []string `toml:"transports"`
	Cores      []string `toml:"cores"`
}

// Core describes a capability bundle. Protocol and API version numbering starts
// at 1, so a zero value means the field is absent (not set in the TOML file).
//
// Required by LoadCore: Name, CoreAPI, Protocol, Links.
// Optional: Version (not validated; informational only), Provides.
// Runtime.Cores is not validated non-empty: an empty cores list safely fails
// every handshake with UNKNOWN_CORE rather than silently accepting anything.
type Core struct {
	Name     string   `toml:"name"`
	Version  string   `toml:"version"`
	CoreAPI  int      `toml:"coreApi"`
	Protocol int      `toml:"protocol"`
	Provides []string `toml:"provides"`
	Links    []string `toml:"links"`
}

// Script describes a user script. Protocol and API version numbering starts
// at 1, so a zero value means the field is absent (not set in the TOML file).
//
// Required by LoadScript: Name, Core, CoreAPI, Link, Cmd.
// Optional: Version, Language (informational metadata for client badge display).
type Script struct {
	Name         string   `toml:"name"`
	Version      string   `toml:"version"`
	Core         string   `toml:"core"`
	CoreAPI      int      `toml:"coreApi"`
	Link         string   `toml:"link"`
	Cmd          []string `toml:"cmd"`
	Language     string   `toml:"language"`
	Capabilities []string `toml:"capabilities"`
}

func LoadRuntime(path string) (Runtime, error) {
	var r Runtime
	if _, err := toml.DecodeFile(path, &r); err != nil {
		return Runtime{}, fmt.Errorf("read runtime manifest %s: %w", path, err)
	}
	if r.Name == "" || len(r.Protocols) == 0 || len(r.Transports) == 0 {
		return Runtime{}, fmt.Errorf("runtime manifest %s: name, protocols, and transports are required", path)
	}
	return r, nil
}

func LoadCore(path string) (Core, error) {
	var c Core
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return Core{}, fmt.Errorf("read core manifest %s: %w", path, err)
	}
	if c.Name == "" || c.CoreAPI == 0 || c.Protocol == 0 || len(c.Links) == 0 {
		return Core{}, fmt.Errorf("core manifest %s: name, coreApi, protocol, links are required", path)
	}
	return c, nil
}

func LoadScript(path string) (Script, error) {
	var s Script
	if _, err := toml.DecodeFile(path, &s); err != nil {
		return Script{}, fmt.Errorf("read script manifest %s: %w", path, err)
	}
	if s.Name == "" || s.Core == "" || s.CoreAPI == 0 || s.Link == "" || len(s.Cmd) == 0 {
		return Script{}, fmt.Errorf("script manifest %s: name, core, coreApi, link, cmd are required", path)
	}
	return s, nil
}
