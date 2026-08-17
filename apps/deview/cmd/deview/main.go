// Command deview — консольный браузер скриптов: поднимает мост wire, показывает
// нумерованное меню и рисует живой ход выполнения выбранного скрипта.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"wire-auto/apps/deview/internal/bridge"
	"wire-auto/apps/deview/internal/ui"
)

func main() {
	wireBin := flag.String("wire", "", "path to the wire bridge binary (default: go run ./cores/duplex/cmd/core)")
	flag.Parse()

	tr, err := startBridge(*wireBin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "не удалось запустить мост wire:", err)
		os.Exit(1)
	}
	client := bridge.NewClient(tr)
	defer client.Close()

	reader := bufio.NewScanner(os.Stdin)
	for {
		scripts, err := client.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка каталога:", err)
			return
		}
		fmt.Print(ui.RenderMenu(scripts))

		if !reader.Scan() {
			return
		}
		choice := strings.TrimSpace(reader.Text())
		if choice == "q" || choice == "quit" || choice == "exit" {
			return
		}
		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(scripts) {
			fmt.Println("Неверный выбор.")
			continue
		}
		runOne(client, reader, scripts[n-1])
	}
}

// runOne запускает выбранный скрипт и рисует его ход. Ctrl-C во время прогона
// шлёт cancel мосту. На событие prompt печатает запрос и читает строку из stdin.
func runOne(client *bridge.Client, reader *bufio.Scanner, s bridge.Script) {
	fmt.Printf("\n▶ %s\n", s.Name)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigCh)
		close(sigCh)
	}()
	go func() {
		for range sigCh {
			fmt.Println("\n… отмена")
			_ = client.Cancel()
		}
	}()

	term, err := client.Run(s.Dir, func(e bridge.Event) {
		switch e.Type {
		case "ready":
			fmt.Println("  ⏳ выполняется…")
		case "log":
			fmt.Println(ui.RenderLog(e))
		case "prompt":
			fmt.Printf("  ❔ %s ", e.Message)
			line := ""
			if reader.Scan() {
				line = strings.TrimSpace(reader.Text())
			}
			_ = client.SendInput(e.ID, line)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "обрыв связи с мостом:", err)
		return
	}
	fmt.Println(ui.RenderResult(term))
	fmt.Println()
}

// startBridge выбирает, как поднять мост: указанный бинарник или dev-режим
// через `go run`. Возвращает готовый транспорт.
func startBridge(wireBin string) (bridge.Transport, error) {
	if wireBin != "" {
		return bridge.NewProcessTransport(wireBin)
	}
	if env := os.Getenv("WIRE_BIN"); env != "" {
		return bridge.NewProcessTransport(env)
	}
	// dev-умолчание: запускать из корня репозитория.
	return bridge.NewProcessTransport("go", "run", "./cores/duplex/cmd/core")
}
