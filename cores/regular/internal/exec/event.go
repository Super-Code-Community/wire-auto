package exec

// Event — единица живого потока прогона наружу (в мост/клиент).
// Kind = "ready" (скрипт поднялся) либо "log" (строка вывода).
type Event struct {
	Kind    string
	Level   string
	Message string
}
