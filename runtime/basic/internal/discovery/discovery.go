// Package discovery сканирует корень скриптов на script.manifest и отдаёт
// сводки для каталога моста. Симметрично registry.Discover для ядер.
package discovery

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"wire-auto/runtime/basic/internal/manifest"
)

// Script — сводка одного скрипта для каталога клиента.
type Script struct {
	Name         string
	Dir          string
	Language     string
	Version      string
	Capabilities []string
}

// Scan рекурсивно обходит scriptsDir и находит каждую директорию, содержащую
// script.manifest, на любой глубине — скрипты живут под категориями
// (scripts/examples/hello, scripts/community/…). Найдя валидный манифест,
// в этот каталог дальше не углубляемся (SkipDir): вложенных скриптов в нём нет.
// Папки без валидного манифеста тихо пропускаются (не ошибка); ошибкой является
// лишь нечитаемый корень. Результат отсортирован по Name.
func Scan(scriptsDir string) ([]Script, error) {
	// Нечитаемый корень — ошибка (контракт TestScanMissingDirIsError).
	// Ошибки обхода глубже корня терпим.
	if _, err := os.ReadDir(scriptsDir); err != nil {
		return nil, err
	}

	var out []Script
	_ = filepath.WalkDir(scriptsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Нечитаемая ветка глубже корня — пропускаем её, не рушим весь обход.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		mpath := filepath.Join(path, "script.manifest")
		if _, statErr := os.Stat(mpath); statErr != nil {
			return nil // не папка скрипта — идём глубже
		}
		s, loadErr := manifest.LoadScript(mpath)
		if loadErr != nil {
			// Невалидный манифест — пропускаем каталог, но не рушим обход.
			return filepath.SkipDir
		}
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			abs = path
		}
		caps := s.Capabilities
		if caps == nil {
			caps = []string{}
		}
		out = append(out, Script{
			Name:         s.Name,
			Dir:          abs,
			Language:     s.Language,
			Version:      s.Version,
			Capabilities: caps,
		})
		return filepath.SkipDir // каталог скрипта не содержит вложенных скриптов
	})

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
