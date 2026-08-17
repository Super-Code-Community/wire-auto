package ui

import (
	"strings"
	"testing"

	"wire-auto/apps/deview/internal/bridge"
)

func TestRenderMenuNumbersAndBadges(t *testing.T) {
	out := RenderMenu([]bridge.Script{
		{Name: "hello", Language: "python", Capabilities: []string{}},
		{Name: "flash", Language: "bash", Capabilities: []string{"serial"}},
	})
	if !strings.Contains(out, "1) hello") || !strings.Contains(out, "2) flash") {
		t.Fatalf("нет нумерации:\n%s", out)
	}
	if !strings.Contains(out, "python") || !strings.Contains(out, "serial") {
		t.Fatalf("нет бейджей языка/возможностей:\n%s", out)
	}
}

func TestRenderMenuEmpty(t *testing.T) {
	out := RenderMenu(nil)
	if !strings.Contains(strings.ToLower(out), "нет скриптов") {
		t.Fatalf("пустой каталог должен сообщать об отсутствии скриптов:\n%s", out)
	}
}

func TestRenderResultOKAndError(t *testing.T) {
	ok := RenderResult(bridge.Event{Type: "result", Status: "OK", ExitCode: 0})
	if !strings.Contains(ok, "OK") {
		t.Fatalf("нет OK: %s", ok)
	}
	bad := RenderResult(bridge.Event{Type: "result", Status: "SCRIPT_ERROR", ExitCode: 2})
	if !strings.Contains(bad, "SCRIPT_ERROR") || !strings.Contains(bad, "2") {
		t.Fatalf("нет статуса/кода ошибки: %s", bad)
	}
	er := RenderResult(bridge.Event{Type: "error", Message: "no such dir"})
	if !strings.Contains(er, "no such dir") {
		t.Fatalf("нет текста ошибки моста: %s", er)
	}
}

func TestRenderLogLevel(t *testing.T) {
	out := RenderLog(bridge.Event{Type: "log", Level: "warn", Message: "careful"})
	if !strings.Contains(out, "careful") {
		t.Fatalf("нет текста лога: %s", out)
	}
}
