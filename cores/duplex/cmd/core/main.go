// Command core — стриминговый мост бандла duplex: приложение шлёт команды в
// stdin, бандл стримит события в stdout (JSON Lines). Живёт до exit/EOF,
// обслуживая запуск за запуском. Рантайм встроен; отдельного admission нет —
// бандл знает своё ядро из собственного core.manifest. Двусторонний канал
// request/response обслуживает capreg-реестр; множество provides выводится из
// его ключей.
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"time"

	"wire-auto/cores/duplex/internal/bridge"
	"wire-auto/cores/duplex/internal/capreg"
	"wire-auto/cores/duplex/internal/discovery"
	"wire-auto/cores/duplex/internal/exec"
	"wire-auto/cores/duplex/internal/handshake"
	"wire-auto/cores/duplex/internal/manifest"
)

// providesFromRegistry — авторизованное множество capability бандла: ключи
// реестра хендлеров (код — источник правды, не манифест). Сортировка ради
// детерминизма.
func providesFromRegistry(reg map[string]capreg.Handler) []string {
	out := make([]string, 0, len(reg))
	for k := range reg {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// runStreaming выполняет один прогон: load core.manifest → route (только своё
// ядро) → handshake → spawn → pump, прокидывая ctx и onEvent в exec.Run.
func runStreaming(ctx context.Context, coreManifestPath, scriptDir string, onEvent func(exec.Event), answers <-chan exec.PromptAnswer) (exec.Result, error) {
	core, err := manifest.LoadCore(coreManifestPath)
	if err != nil {
		return exec.Result{}, err
	}
	scr, err := manifest.LoadScript(filepath.Join(scriptDir, "script.manifest"))
	if err != nil {
		return exec.Result{}, err
	}

	provides := providesFromRegistry(capreg.Default)
	reconciled, err := handshake.Reconcile(core, scr, provides)
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
		Env:            []string{"WIRE_SDK_DIR=" + sdkDir, "GOWORK=off"},
		Protocol:       reconciled.Protocol,
		CoreAPI:        reconciled.CoreAPI,
		ScriptArgs:     []string{},
		StartupTimeout: 10 * time.Second,
		RunTimeout:     60 * time.Second,
		CancelGrace:    2 * time.Second,
		Provides:       reconciled.Provides,
		Registry:       capreg.Default,
		OnEvent:        onEvent,
		Answers:        answers,
	}
	return exec.Run(ctx, spec), nil
}

// run — тонкая обёртка без стриминга (используется e2e-тестами).
func run(coreManifestPath, scriptDir string) (exec.Result, error) {
	return runStreaming(context.Background(), coreManifestPath, scriptDir, nil, nil)
}

func main() {
	coreManifest := flag.String("core", "cores/duplex/core.manifest", "path to this core's manifest")
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
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event), answers <-chan exec.PromptAnswer) (exec.Result, error) {
			return runStreaming(ctx, *coreManifest, dir, onEvent, answers)
		},
	}

	if err := bridge.Serve(os.Stdin, os.Stdout, deps); err != nil {
		os.Exit(1)
	}
}
