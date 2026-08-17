package capreg

import (
	"encoding/json"
	"runtime"
	"testing"
)

func TestSysInfo(t *testing.T) {
	res, code, err := Default["sys.info"](json.RawMessage(`{}`))
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		OS     string `json:"os"`
		Arch   string `json:"arch"`
		NumCPU int    `json:"numCPU"`
	}
	json.Unmarshal(res, &out)
	if out.OS != runtime.GOOS || out.Arch != runtime.GOARCH || out.NumCPU < 1 {
		t.Fatalf("info = %+v", out)
	}
}

func TestSysEnvListPrefix(t *testing.T) {
	t.Setenv("WIRE_TEST_LISTVAR", "x")
	res, code, err := Default["sys.env.list"](json.RawMessage(`{"prefix":"WIRE_TEST_"}`))
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		Names []string `json:"names"`
	}
	json.Unmarshal(res, &out)
	found := false
	for _, n := range out.Names {
		if n == "WIRE_TEST_LISTVAR" {
			found = true
		}
	}
	if !found {
		t.Fatalf("WIRE_TEST_LISTVAR not found in %v", out.Names)
	}
}

func TestSysEnvListBadParams(t *testing.T) {
	_, code, _ := Default["sys.env.list"](json.RawMessage(`{bad`))
	if code != "BAD_PARAMS" {
		t.Fatalf("code=%q, want BAD_PARAMS", code)
	}
}
