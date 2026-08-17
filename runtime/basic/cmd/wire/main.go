// Command wire — двусторонний мост: приложение шлёт команды в stdin, рантайм
// стримит события в stdout (JSON Lines). Живёт до exit/EOF, обслуживая запуск
// за запуском.
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"time"

	"wire-auto/runtime/basic/internal/bridge"
	"wire-auto/runtime/basic/internal/discovery"
	"wire-auto/runtime/basic/internal/exec"
	"wire-auto/runtime/basic/internal/handshake"
	"wire-auto/runtime/basic/internal/manifest"
	"wire-auto/runtime/basic/internal/registry"
)

// runStreaming выполняет один прогон: discover→route→handshake→spawn→pump,
// прокидывая ctx (внешняя отмена) и onEvent (живой поток) в exec.Run.
func runStreaming(ctx context.Context, runtimePath, coresDir, scriptDir string, onEvent func(exec.Event)) (exec.Result, error) {
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
		OnEvent:        onEvent,
	}
	return exec.Run(ctx, spec), nil
}

// run — тонкая обёртка без стриминга (используется e2e-тестами).
func run(runtimePath, coresDir, scriptDir string) (exec.Result, error) {
	return runStreaming(context.Background(), runtimePath, coresDir, scriptDir, nil)
}

func main() {
	runtimePath := flag.String("runtime", "runtime/basic/runtime.manifest", "path to runtime manifest")
	coresDir := flag.String("cores", "cores", "path to the cores directory")
	scriptsDir := flag.String("scripts", "scripts", "path to the scripts directory")
	flag.Parse()

	deps := bridge.Deps{
		List: func() ([]bridge.Script, error) {
			found, err := discovery.Scan(*scriptsDir)
			if err != nil {
				return nil, err
			}
			out := make([]bridge.Script, len(found))
			for i, s := range found {
				out[i] = bridge.Script{
					Name:         s.Name,
					Dir:          s.Dir,
					Language:     s.Language,
					Version:      s.Version,
					Capabilities: s.Capabilities,
				}
			}
			return out, nil
		},
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) {
			return runStreaming(ctx, *runtimePath, *coresDir, dir, onEvent)
		},
	}

	if err := bridge.Serve(os.Stdin, os.Stdout, deps); err != nil {
		os.Exit(1)
	}
}
