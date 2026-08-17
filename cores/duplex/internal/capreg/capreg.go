// Package capreg — реестр обработчиков capability для рантайма duplex.
// Обработчик исполняет уже авторизованный запрос (гейт provides — в exec).
// Default собирается слиянием под-реестров по категориям (env/net/sys).
package capreg

import "encoding/json"

// Handler исполняет capability. Возвращает либо result (code==""), либо
// машиночитаемый code ошибки capability (+пояснение в err). Ошибка capability
// НЕ роняет прогон — она уезжает в response.code.
type Handler func(params json.RawMessage) (result json.RawMessage, code string, err error)

// Default — реестр v2. Ключи должны совпадать с core.provides (выводятся из них).
var Default = merge(envCaps, netCaps, sysCaps)

// merge сливает под-реестры в один. Дубликат ключа между категориями — ошибка
// сборки (реестр строится на init), поэтому паникуем, а не тихо перетираем.
func merge(maps ...map[string]Handler) map[string]Handler {
	out := make(map[string]Handler)
	for _, m := range maps {
		for k, v := range m {
			if _, dup := out[k]; dup {
				panic("capreg: duplicate capability key: " + k)
			}
			out[k] = v
		}
	}
	return out
}
