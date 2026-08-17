// Command gohello — минимальный Go-скрипт для e2e: читает hello, отвечает
// ready и done{ok:true}. Без сети — быстрый и детерминированный. Доказывает,
// что ядро проставляет GOWORK=off (иначе `go run .` упал бы на корневом go.work).
package main

import (
	"bufio"
	"encoding/json"
	"os"
)

func main() {
	r := bufio.NewReader(os.Stdin)
	r.ReadString('\n') // hello — говорим на протоколе напрямую, без SDK

	enc := json.NewEncoder(os.Stdout) // Encode пишет компактный JSON + '\n'
	enc.Encode(map[string]any{"type": "ready"})
	enc.Encode(map[string]any{
		"type":     "done",
		"exitCode": 0,
		"result":   map[string]any{"ok": true},
	})
}
