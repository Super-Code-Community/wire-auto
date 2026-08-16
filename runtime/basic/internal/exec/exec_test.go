package exec

import (
	"testing"
	"time"
)

// helloReadyLogDone: минимальный «скрипт» на python, эмулирующий шим.
const goodScript = `
import sys, json
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
send({"type":"log","level":"info","message":"hello from python"})
send({"type":"done","exitCode":0})
`

func baseSpec(code string) Spec {
	return Spec{
		Command:        "python",
		Args:           []string{"-c", code},
		Protocol:       1,
		CoreAPI:        1,
		StartupTimeout: 5 * time.Second,
		RunTimeout:     5 * time.Second,
	}
}

func TestRunOK(t *testing.T) {
	res := Run(baseSpec(goodScript))
	if res.Status != StatusOK {
		t.Fatalf("status=%s err=%s", res.Status, res.ErrorMessage)
	}
	if len(res.Logs) != 1 || res.Logs[0].Message != "hello from python" {
		t.Fatalf("logs=%+v", res.Logs)
	}
}

func TestRunScriptError(t *testing.T) {
	code := `
import sys, json
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
send({"type":"done","exitCode":3})
`
	res := Run(baseSpec(code))
	if res.Status != StatusScriptError || res.ExitCode != 3 {
		t.Fatalf("got status=%s exit=%d", res.Status, res.ExitCode)
	}
}

func TestRunProtocolViolationOnRequest(t *testing.T) {
	code := `
import sys, json
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
send({"type":"request","capability":"serial"})
`
	res := Run(baseSpec(code))
	if res.Status != StatusProtocolViolation {
		t.Fatalf("got status=%s", res.Status)
	}
}

func TestRunRunTimeout(t *testing.T) {
	code := `
import sys, json, time
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
time.sleep(30)
`
	spec := baseSpec(code)
	spec.RunTimeout = 500 * time.Millisecond
	spec.CancelGrace = 200 * time.Millisecond
	res := Run(spec)
	if res.Status != StatusRunTimeout {
		t.Fatalf("got status=%s", res.Status)
	}
}

func TestRunRunTimeoutSendsCancel(t *testing.T) {
	// Script sends ready, then blocks on stdin.readline() waiting for any line
	// (the cancel message). Upon receiving it, it sends done and exits cleanly.
	// This proves cancel is delivered: the process exits during the grace window,
	// not at hard-kill time.
	code := `
import sys, json
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
sys.stdin.readline()
send({"type":"done","exitCode":0})
`
	spec := baseSpec(code)
	spec.RunTimeout = 300 * time.Millisecond
	spec.CancelGrace = 2 * time.Second

	start := time.Now()
	res := Run(spec)
	elapsed := time.Since(start)

	if res.Status != StatusRunTimeout {
		t.Fatalf("got status=%s, want RUN_TIMEOUT", res.Status)
	}
	maxElapsed := spec.RunTimeout + spec.CancelGrace
	if elapsed >= maxElapsed {
		t.Fatalf("elapsed %v >= RunTimeout+CancelGrace %v; cancel not delivered in time", elapsed, maxElapsed)
	}
}

func TestRunStartupTimeout(t *testing.T) {
	code := `
import sys, time
sys.stdin.readline()
time.sleep(30)
`
	spec := baseSpec(code)
	spec.StartupTimeout = 500 * time.Millisecond
	res := Run(spec)
	if res.Status != StatusStartupTimeout {
		t.Fatalf("got status=%s", res.Status)
	}
}

func TestRunCrashNoDone(t *testing.T) {
	code := `
import sys, json
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
sys.exit(0)
`
	res := Run(baseSpec(code))
	if res.Status != StatusCrashed {
		t.Fatalf("got status=%s", res.Status)
	}
}
