// Command core — одноразовый прогонщик бандла duplex: встроенный рантайм
// запускает один скрипт по пути (load core.manifest→handshake→spawn→pump) и
// печатает исход. Двусторонний канал request/response обслуживает capreg-реестр;
// множество provides выводится из его ключей.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"wire-auto/cores/duplex/internal/capreg"
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

func run(coreManifestPath, scriptDir string) (exec.Result, error) {
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
		Env:            []string{"WIRE_SDK_DIR=" + sdkDir},
		Protocol:       reconciled.Protocol,
		CoreAPI:        reconciled.CoreAPI,
		ScriptArgs:     []string{},
		StartupTimeout: 10 * time.Second,
		RunTimeout:     60 * time.Second,
		CancelGrace:    2 * time.Second,
		Provides:       reconciled.Provides,
		Registry:       capreg.Default,
	}
	return exec.Run(context.Background(), spec), nil
}

func main() {
	coreManifest := flag.String("core", "cores/duplex/core.manifest", "path to this core's manifest")
	scriptDir := flag.String("script", "", "path to the script directory to run")
	flag.Parse()

	if *scriptDir == "" {
		fmt.Fprintln(os.Stderr, "usage: core -script <dir> [-core manifest]")
		os.Exit(2)
	}

	res, err := run(*coreManifest, *scriptDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	fmt.Printf("Status: %s\n", res.Status)
	if res.ErrorCode != "" {
		fmt.Printf("Error:  %s %s\n", res.ErrorCode, res.ErrorMessage)
	}
	for _, l := range res.Logs {
		fmt.Printf("[%s] %s\n", l.Level, l.Message)
	}
	if len(res.Result) > 0 {
		fmt.Printf("Result: %s\n", res.Result)
	}
	if res.Status != exec.StatusOK {
		os.Exit(1)
	}
}
