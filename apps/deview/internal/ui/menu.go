// Package ui рендерит консольный вывод deview: меню и события прогона.
// Только форматирование строк — ввод/вывод живёт в cmd/deview.
package ui

import (
	"fmt"
	"strings"

	"wire-auto/apps/deview/internal/bridge"
)

// RenderMenu форматирует нумерованный список скриптов с бейджами.
func RenderMenu(scripts []bridge.Script) string {
	if len(scripts) == 0 {
		return "Нет скриптов в каталоге.\n"
	}
	var b strings.Builder
	b.WriteString("Доступные скрипты:\n")
	for i, s := range scripts {
		badges := s.Language
		if len(s.Capabilities) > 0 {
			badges += " · " + strings.Join(s.Capabilities, ",")
		}
		if badges != "" {
			badges = "  [" + badges + "]"
		}
		fmt.Fprintf(&b, "  %d) %s%s\n", i+1, s.Name, badges)
	}
	b.WriteString("\nНомер — запустить, q — выход: ")
	return b.String()
}
