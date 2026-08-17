// Package capreg — реестр обработчиков capability для рантайма duplex.
// Обработчик исполняет уже авторизованный запрос (гейт provides — в exec).
package capreg

import (
	"encoding/json"
	"fmt"
	"os"
)

// Handler исполняет capability. Возвращает либо result (code==""), либо
// машиночитаемый code ошибки capability (+пояснение в err). Ошибка capability
// НЕ роняет прогон — она уезжает в response.code.
type Handler func(params json.RawMessage) (result json.RawMessage, code string, err error)

// Default — реестр v2. Ключи должны совпадать с core.provides.
var Default = map[string]Handler{
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
	out, err := json.Marshal(map[string]string{"value": v})
	if err != nil {
		return nil, "BAD_PARAMS", err
	}
	return out, "", nil
}
