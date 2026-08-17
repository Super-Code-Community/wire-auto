package main

import (
	"net"
	"sort"
	"strconv"
	"sync"
	"time"
)

// progressStep — как часто (по числу прозвоненных портов) звать progress.
const progressStep = 4096

type scanResult struct {
	port int
	open bool
}

// scanPorts конкурентно прозванивает порты host через worker-pool из workers
// горутин: каждая тянет порт из канала и делает net.DialTimeout. Успешный
// connect → порт открыт. Ошибка (refuse/timeout) — просто «не открыт», без
// паники. Возвращает отсортированный список открытых портов.
//
// Безопасность для локальной сети: workers держат конкурентность в узде (много
// одновременных полуоткрытых исходящих соединений переполняют NAT роутера), а
// SetLinger(0) заставляет Close слать RST вместо FIN — эфемерный порт
// освобождается сразу, без TIME_WAIT, иначе полный свип 65535 портов выжигает
// весь диапазон динамических портов и кладёт сеть всей машины.
//
// progress (если не nil) вызывается по мере прогона — каждые progressStep
// прозвоненных портов и один раз в конце — с (сколько_прозвонено, всего). Зовётся
// из единственного потребителя, так что без гонок и без mutex у вызывающего.
func scanPorts(host string, ports []int, workers int, timeout time.Duration, progress func(done, total int)) []int {
	if workers < 1 {
		workers = 1
	}
	total := len(ports)
	jobs := make(chan int)
	results := make(chan scanResult, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range jobs {
				addr := net.JoinHostPort(host, strconv.Itoa(port))
				conn, err := net.DialTimeout("tcp", addr, timeout)
				if err != nil {
					results <- scanResult{port, false} // отказ/таймаут — не открыт
					continue
				}
				if tc, ok := conn.(*net.TCPConn); ok {
					tc.SetLinger(0) // Close → RST, освобождаем порт сразу (без TIME_WAIT)
				}
				conn.Close()
				results <- scanResult{port, true}
			}
		}()
	}

	go func() {
		for _, p := range ports {
			jobs <- p
		}
		close(jobs)
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	open := []int{}
	done := 0
	for r := range results {
		done++
		if r.open {
			open = append(open, r.port)
		}
		if progress != nil && done%progressStep == 0 {
			progress(done, total)
		}
	}
	if progress != nil {
		progress(done, total) // финальный тик — гарантируем done==total
	}
	sort.Ints(open)
	return open
}
