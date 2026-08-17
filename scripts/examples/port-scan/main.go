// Command port-scan — «толстый» Go-скрипт: сам конкурентно сканит все порты
// 1..65535 на localhost через scanPorts (net.DialTimeout в горутинах), не
// используя ядерные net.* capability. С ядром общается по сырому JSON-Lines-
// протоколу: hello → ready → log → done.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultHost = "127.0.0.1"
	// workers держим умеренным: тысячи одновременных полуоткрытых исходящих
	// соединений (фильтрованные порты удалённого хоста висят до таймаута)
	// переполняют NAT роутера и эфемерные порты — кладут сеть всей машины.
	workers = 100
	timeout = 500 * time.Millisecond
)

// askPrompt шлёт ядру prompt и синхронно ждёт response по тому же id, возвращая
// result.value. Симметрично вызову capability (request→response), но prompt — не
// capability: ввод идёт от человека, объявлять в манифесте не нужно.
func askPrompt(enc *json.Encoder, in *bufio.Reader, id, message string) string {
	enc.Encode(map[string]any{"type": "prompt", "id": id, "message": message})
	line, _ := in.ReadString('\n')
	var resp struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	json.Unmarshal([]byte(line), &resp)
	return resp.Result.Value
}

func main() {
	r := bufio.NewReader(os.Stdin)
	enc := json.NewEncoder(os.Stdout) // Encode пишет компактный JSON + '\n'

	r.ReadString('\n') // hello — говорим на протоколе напрямую, без SDK
	enc.Encode(map[string]any{"type": "ready"})

	// Спрашиваем адрес для прозвона; пустой ответ → localhost.
	host := strings.TrimSpace(askPrompt(enc, r, "1",
		"Введите адрес для прозвона (Enter — 127.0.0.1)"))
	if host == "" {
		host = defaultHost
	}

	ports := make([]int, 0, 65535)
	for p := 1; p <= 65535; p++ {
		ports = append(ports, p)
	}

	logf := func(format string, a ...any) {
		enc.Encode(map[string]any{
			"type": "log", "level": "info",
			"message": fmt.Sprintf(format, a...),
		})
	}

	logf("scanning %s ports 1..%d (%d workers, %s timeout)",
		host, len(ports), workers, timeout)

	progress := func(done, total int) {
		logf("scanned %d/%d ports", done, total)
	}
	open := scanPorts(host, ports, workers, timeout, progress)

	for _, p := range open {
		logf("port %d OPEN", p)
	}
	logf("done: %d open of %d scanned on %s", len(open), len(ports), host)
	enc.Encode(map[string]any{
		"type":     "done",
		"exitCode": 0,
		"result":   map[string]any{"host": host, "open": open, "scanned": len(ports)},
	})
}
