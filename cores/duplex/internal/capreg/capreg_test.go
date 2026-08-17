package capreg

import (
	"encoding/json"
	"testing"
)

func TestEnvGetFound(t *testing.T) {
	t.Setenv("WIRE_TEST_VAR", "hello")
	result, code, err := Default["env.get"](json.RawMessage(`{"name":"WIRE_TEST_VAR"}`))
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct{ Value string `json:"value"` }
	if e := json.Unmarshal(result, &out); e != nil {
		t.Fatal(e)
	}
	if out.Value != "hello" {
		t.Fatalf("value = %q, want hello", out.Value)
	}
}

func TestEnvGetNotFound(t *testing.T) {
	_, code, _ := Default["env.get"](json.RawMessage(`{"name":"WIRE_DEFINITELY_MISSING_XYZ"}`))
	if code != "ENV_NOT_FOUND" {
		t.Fatalf("code = %q, want ENV_NOT_FOUND", code)
	}
}

func TestEnvGetBadParams(t *testing.T) {
	_, code, _ := Default["env.get"](json.RawMessage(`{}`))
	if code != "BAD_PARAMS" {
		t.Fatalf("code = %q, want BAD_PARAMS", code)
	}
}
