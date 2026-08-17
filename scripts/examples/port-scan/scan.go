package main

import (
	"net"
	"sort"
	"strconv"
	"sync"
	"time"
)

// scanPorts конкурентно прозванивает порты host через worker-pool из workers
// горутин: каждая тянет порт из канала и делает net.DialTimeout. Успешный
// connect → порт открыт (conn закрываем). Ошибка (refuse/timeout) — просто
// «не открыт», без паники. Возвращает отсортированный список открытых портов.
func scanPorts(host string, ports []int, workers int, timeout time.Duration) []int {
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan int)
	results := make(chan int)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range jobs {
				addr := net.JoinHostPort(host, strconv.Itoa(port))
				conn, err := net.DialTimeout("tcp", addr, timeout)
				if err != nil {
					continue // отказ/таймаут — не открыт
				}
				conn.Close()
				results <- port
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
	for p := range results {
		open = append(open, p)
	}
	sort.Ints(open)
	return open
}
