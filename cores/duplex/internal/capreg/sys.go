// Под-реестр системных возможностей: инфо о хосте и список переменных окружения.
package capreg

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
)

var sysCaps = map[string]Handler{
	"sys.info":     sysInfo,
	"sys.env.list": sysEnvList,
}

func sysInfo(params json.RawMessage) (json.RawMessage, string, error) {
	host, _ := os.Hostname()
	return okResult(map[string]any{
		"os":        runtime.GOOS,
		"arch":      runtime.GOARCH,
		"hostname":  host,
		"numCPU":    runtime.NumCPU(),
		"goVersion": runtime.Version(),
	})
}

func sysEnvList(params json.RawMessage) (json.RawMessage, string, error) {
	var p struct {
		Prefix string `json:"prefix"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return badParams("sys.env.list: invalid params")
		}
	}
	names := []string{}
	for _, kv := range os.Environ() {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if p.Prefix == "" || strings.HasPrefix(name, p.Prefix) {
			names = append(names, name)
		}
	}
	return okResult(map[string]any{"names": names})
}
