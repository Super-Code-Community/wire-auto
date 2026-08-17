package capreg

import (
	"encoding/json"
	"fmt"
	"os"
)

// envCaps — под-реестр возможностей работы с переменными окружения.
var envCaps = map[string]Handler{
	"env.get": envGet,
}

func envGet(params json.RawMessage) (json.RawMessage, string, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, "BAD_PARAMS", fmt.Errorf("env.get requires a non-empty string field \"name\"")
	}
	v, ok := os.LookupEnv(p.Name)
	if !ok {
		return nil, "ENV_NOT_FOUND", fmt.Errorf("env var not set: %s", p.Name)
	}
	return okResult(map[string]string{"value": v})
}
