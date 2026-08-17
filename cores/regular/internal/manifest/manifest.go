// Package manifest читает и валидирует два паспорта платформы: core и script.
// Формат — TOML. runtime.manifest упразднён: рантайм — встроенная библиотека
// бандла, а не отдельный паспорт.
package manifest

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// Core — контракт самого бандла. Обязательны: Name, CoreAPI, Protocol, Links.
// Optional: Version (информационно).
type Core struct {
	Name     string   `toml:"name"`
	Version  string   `toml:"version"`
	CoreAPI  int      `toml:"coreApi"`
	Protocol int      `toml:"protocol"`
	Links    []string `toml:"links"`
}

// Script — пользовательский скрипт. Обязательны: Name, Core, CoreAPI, Link, Cmd.
// Optional: Version, Language (метаданные для бейджа клиента).
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
