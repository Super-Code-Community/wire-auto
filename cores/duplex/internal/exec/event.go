package exec

// Event — единица живого потока прогона наружу (в мост/клиент).
// Kind = "ready" (скрипт поднялся), "log" (строка вывода) либо
// "prompt" (скрипт просит у человека строку ввода; ID коррелирует ответ).
type Event struct {
	Kind    string
	ID      string
	Level   string
	Message string
}
