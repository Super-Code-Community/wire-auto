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

	open := scanPorts("127.0.0.1", []int{openPort, closedPort}, 8, 500*time.Millisecond)

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
