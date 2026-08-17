// Общие хелперы разбора params и политики таймаутов для хендлеров capreg.
package capreg

import (
	"encoding/json"
	"fmt"
)

const (
	defaultTimeoutMS = 1000
	maxTimeoutMS     = 10000
	defaultReadBytes = 256
	maxReadBytes     = 4096
)

// clampTimeout зажимает timeout_ms в (0, maxTimeoutMS]; 0/отрицательное → дефолт.
func clampTimeout(ms int) int {
	if ms <= 0 {
		return defaultTimeoutMS
	}
	if ms > maxTimeoutMS {
		return maxTimeoutMS
	}
	return ms
}

// clampReadBytes зажимает read_bytes в (0, maxReadBytes]; 0/отрицательное → дефолт.
func clampReadBytes(n int) int {
	if n <= 0 {
		return defaultReadBytes
	}
	if n > maxReadBytes {
		return maxReadBytes
	}
	return n
}

// badParams — единый способ вернуть код BAD_PARAMS с пояснением.
func badParams(format string, args ...any) (json.RawMessage, string, error) {
	return nil, "BAD_PARAMS", fmt.Errorf(format, args...)
}

// okResult маршалит успешный результат; при ошибке маршалинга → BAD_PARAMS.
func okResult(v any) (json.RawMessage, string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return nil, "BAD_PARAMS", err
	}
	return out, "", nil
}
