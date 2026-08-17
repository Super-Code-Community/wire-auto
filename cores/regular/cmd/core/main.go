// Command core — стриминговый мост бандла regular: приложение шлёт команды в
// stdin, бандл стримит события в stdout (JSON Lines). Живёт до exit/EOF,
// обслуживая запуск за запуском. Рантайм встроен; отдельного admission нет —
// бандл знает своё ядро из собственного core.manifest.
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"time"

	"wire-auto/cores/regular/internal/bridge"
	"wire-auto/cores/regular/internal/discovery"
	"wire-auto/cores/regular/internal/exec"
	"wire-auto/cores/regular/internal/handshake"
	"wire-auto/cores/regular/internal/manifest"
)

// runStreaming выполняет один прогон: load core.manifest → route (только своё
// ядро) → handshake → spawn → pump, прокидывая ctx и onEvent в exec.Run.
func runStreaming(ctx context.Context, coreManifestPath, scriptDir string, onEvent func(exec.Event)) (exec.Result, error) {
	core, err := manifest.LoadCore(coreManifestPath)
	if err != nil {
		return exec.Result{}, err
	}
	scr, err := manifest.LoadScript(filepath.Join(scriptDir, "script.manifest"))
	if err != nil {
		return exec.Result{}, err
	}

	// regular ничего не provides; авторизованное множество пусто.
	reconciled, err := handshake.Reconcile(core, scr, nil)
	if err != nil {
		var he *handshake.Error
		if errors.As(err, &he) {
			return exec.Result{Status: "HANDSHAKE_FAILED", ErrorCode: he.Code, ErrorMessage: he.Message, Logs: []exec.LogLine{}}, nil
		}
		return exec.Result{}, err
	}

	absCoreDir, err := filepath.Abs(filepath.Dir(coreManifestPath))
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
func run(coreManifestPath, scriptDir string) (exec.Result, error) {
	return runStreaming(context.Background(), coreManifestPath, scriptDir, nil)
}

func main() {
	coreManifest := flag.String("core", "cores/regular/core.manifest", "path to this core's manifest")
	scriptsDir := flag.String("scripts", "scripts", "path to the scripts directory")
	flag.Parse()

	core, err := manifest.LoadCore(*coreManifest)
	if err != nil {
		os.Exit(1)
	}

	deps := bridge.Deps{
		List: func() ([]bridge.Script, error) {
			found, err := discovery.Scan(*scriptsDir)
			if err != nil {
				return nil, err
			}
			out := make([]bridge.Script, 0, len(found))
			for _, s := range found {
				if s.Core != core.Name {
					continue // чужое ядро — этот бинарь его не запустит
				}
				out = append(out, bridge.Script{
					Name:         s.Name,
					Dir:          s.Dir,
					Language:     s.Language,
					Version:      s.Version,
					Capabilities: s.Capabilities,
				})
			}
			return out, nil
		},
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) {
			return runStreaming(ctx, *coreManifest, dir, onEvent)
		},
	}

	if err := bridge.Serve(os.Stdin, os.Stdout, deps); err != nil {
		os.Exit(1)
	}
}
