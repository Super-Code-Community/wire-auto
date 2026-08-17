package main

import (
	"net"
	"testing"
	"time"
)

func TestScanPorts(t *testing.T) {
	// открытый порт: слушаем и НЕ закрываем
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	openPort := ln.Addr().(*net.TCPAddr).Port

	// заведомо закрытый порт: слушаем, узнаём номер, закрываем
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := ln2.Addr().(*net.TCPAddr).Port
	ln2.Close()

	open := scanPorts("127.0.0.1", []int{openPort, closedPort}, 8, 500*time.Millisecond, nil)

	found := map[int]bool{}
	for _, p := range open {
		found[p] = true
	}
	if !found[openPort] {
		t.Errorf("открытый порт %d не найден в %v", openPort, open)
	}
	if found[closedPort] {
		t.Errorf("закрытый порт %d ошибочно найден в %v", closedPort, open)
	}
}

// scanPorts должен звать progress по мере прогона и завершить его done==total,
// чтобы main мог логировать ход скана (иначе долгий скан выглядит зависшим).
func TestScanPortsProgress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	ports := []int{closed, closed, closed, closed}
	lastDone, lastTotal := 0, 0
	calls := 0
	scanPorts("127.0.0.1", ports, 4, 300*time.Millisecond, func(done, total int) {
		calls++
		lastDone, lastTotal = done, total
	})

	if calls == 0 {
		t.Fatal("progress не вызывался ни разу")
	}
	if lastDone != len(ports) || lastTotal != len(ports) {
		t.Errorf("финальный progress = (%d/%d), want (%d/%d)",
			lastDone, lastTotal, len(ports), len(ports))
	}
}
