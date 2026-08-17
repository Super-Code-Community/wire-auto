// Package registry discovers cores and admits those whose contract is
// compatible with the runtime — polymorphism by contract, not by name.
package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"wire-auto/runtime/basic/internal/manifest"
)

type Admitted struct {
	Manifest manifest.Core
	Dir      string
}

type Rejection struct {
	Name   string
	Dir    string
	Reason string
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func overlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// Compatible reports whether the runtime can drive the core, and if not, why.
func Compatible(rt manifest.Runtime, c manifest.Core) (bool, string) {
	if !containsInt(rt.Protocols, c.Protocol) {
		return false, fmt.Sprintf("core protocol %d not spoken by runtime %v", c.Protocol, rt.Protocols)
	}
	if !overlap(rt.Transports, c.Links) {
		return false, fmt.Sprintf("core links %v share no transport with runtime %v", c.Links, rt.Transports)
	}
	return true, ""
}

// Discover scans coresDir for <name>/core.manifest and partitions the cores
// into admitted (compatible, keyed by core name) and rejected (with reason).
func Discover(coresDir string, rt manifest.Runtime) (map[string]Admitted, []Rejection, error) {
	entries, err := os.ReadDir(coresDir)
	if err != nil {
		return nil, nil, err
	}
	admitted := map[string]Admitted{}
	var rejected []Rejection
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(coresDir, e.Name())
		mpath := filepath.Join(dir, "core.manifest")
		if _, err := os.Stat(mpath); err != nil {
			continue // not a core directory
		}
		c, err := manifest.LoadCore(mpath)
		if err != nil {
			rejected = append(rejected, Rejection{Name: e.Name(), Dir: dir, Reason: "invalid manifest: " + err.Error()})
			continue
		}
		if ok, why := Compatible(rt, c); !ok {
			rejected = append(rejected, Rejection{Name: c.Name, Dir: dir, Reason: why})
			continue
		}
		admitted[c.Name] = Admitted{Manifest: c, Dir: dir}
	}
	return admitted, rejected, nil
}
