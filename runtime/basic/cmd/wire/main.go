// Command wire — минимальный рантайм: сводит манифесты и запускает скрипт нужным ядром.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"wire-auto/runtime/basic/internal/exec"
	"wire-auto/runtime/basic/internal/handshake"
	"wire-auto/runtime/basic/internal/manifest"
	"wire-auto/runtime/basic/internal/registry"
)

func run(runtimePath, coresDir, scriptDir string) (exec.Result, error) {
	rt, err := manifest.LoadRuntime(runtimePath)
	if err != nil {
		return exec.Result{}, err
	}

	admitted, rejected, err := registry.Discover(coresDir, rt)
	if err != nil {
		return exec.Result{}, err
	}

	scr, err := manifest.LoadScript(filepath.Join(scriptDir, "script.manifest"))
	if err != nil {
		return exec.Result{}, err
	}

	// Route by the core the script targets. UNKNOWN_CORE = no such admitted core;
	// CORE_INCOMPATIBLE = a core with that name exists but its contract was rejected.
	core, ok := admitted[scr.Core]
	if !ok {
		code, msg := "UNKNOWN_CORE", "no admitted core named "+scr.Core
		for _, r := range rejected {
			if r.Name == scr.Core {
				code, msg = "CORE_INCOMPATIBLE", r.Reason
				break
			}
		}
		return exec.Result{Status: "HANDSHAKE_FAILED", ErrorCode: code, ErrorMessage: msg, Logs: []exec.LogLine{}}, nil
	}

	reconciled, err := handshake.Reconcile(rt, core.Manifest, scr)
	if err != nil {
		var he *handshake.Error
		if errors.As(err, &he) {
			return exec.Result{Status: "HANDSHAKE_FAILED", ErrorCode: he.Code, ErrorMessage: he.Message, Logs: []exec.LogLine{}}, nil
		}
		return exec.Result{}, err
	}

	absCoreDir, err := filepath.Abs(core.Dir)
	if err != nil {
		return exec.Result{}, err
	}
	sdkDir := filepath.Join(absCoreDir, "sdk")

	spec := exec.Spec{
		Dir:            scriptDir,
		Command:        scr.Cmd[0],
		Args:           scr.Cmd[1:],
		Env:            []string{"WIRE_SDK_DIR=" + sdkDir},
		Protocol:       reconciled.Protocol,
		CoreAPI:        reconciled.CoreAPI,
		ScriptArgs:     []string{},
		StartupTimeout: 10 * time.Second,
		RunTimeout:     60 * time.Second,
		CancelGrace:    2 * time.Second,
	}
	return exec.Run(spec), nil
}

func main() {
	runtimePath := flag.String("runtime", "runtime/basic/runtime.manifest", "path to runtime manifest")
	coresDir := flag.String("cores", "cores", "path to the cores directory")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wire [--runtime path] [--cores path] <script-dir>")
		os.Exit(2)
	}
	res, err := run(*runtimePath, *coresDir, flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	if res.Status != exec.StatusOK {
		os.Exit(1)
	}
}
