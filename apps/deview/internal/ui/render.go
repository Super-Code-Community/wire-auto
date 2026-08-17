package ui

import (
	"fmt"

	"wire-auto/apps/deview/internal/bridge"
)

// ANSI-цвета; при желании можно отключить, если stdout не терминал.
const (
	cReset  = "\033[0m"
	cGray   = "\033[90m"
	cYellow = "\033[33m"
	cRed    = "\033[31m"
	cGreen  = "\033[32m"
)

func levelColor(level string) string {
	switch level {
	case "warn":
		return cYellow
	case "error":
		return cRed
	default:
		return cGray
	}
}

// RenderLog форматирует одну строку лога с цветом по уровню.
func RenderLog(e bridge.Event) string {
	lvl := e.Level
	if lvl == "" {
		lvl = "info"
	}
	return fmt.Sprintf("  %s%-5s%s %s", levelColor(lvl), lvl, cReset, e.Message)
}

// RenderResult форматирует терминальное событие прогона (result или error).
func RenderResult(e bridge.Event) string {
	if e.Type == "error" {
		return fmt.Sprintf("%s✗ ошибка моста:%s %s", cRed, cReset, e.Message)
	}
	if e.Status == "OK" {
		line := fmt.Sprintf("%s✔ OK%s", cGreen, cReset)
		if len(e.Result) > 0 {
			line += fmt.Sprintf("  результат: %s", string(e.Result))
		}
		return line
	}
	line := fmt.Sprintf("%s✗ %s%s (код %d)", cRed, e.Status, cReset, e.ExitCode)
	if e.ErrorCode != "" {
		line += fmt.Sprintf(" [%s] %s", e.ErrorCode, e.ErrorMessage)
	}
	return line
}
