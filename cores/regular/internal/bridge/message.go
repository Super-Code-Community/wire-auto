// Package bridge реализует верхнюю границу app↔runtime: JSON Lines поверх stdio.
// Команды приложения вниз (list/run/cancel/exit), события рантайма вверх
// (catalog/ready/log/result/error).
package bridge

import (
	"bufio"
	"encoding/json"
	"io"
)

const maxLineBytes = 1 << 20 // 1 MiB, как в protocol

// Command — сообщение приложение→рантайм.
type Command struct {
	Type string `json:"type"`
	Dir  string `json:"dir,omitempty"`
}

// Script — элемент каталога.
type Script struct {
	Name         string   `json:"name"`
	Dir          string   `json:"dir"`
	Language     string   `json:"language,omitempty"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities"`
}

// Event — сообщение рантайм→приложение. Одна структура на все типы; лишние
// поля опускаются через omitempty.
type Event struct {
	Type         string          `json:"type"`
	Scripts      []Script        `json:"scripts,omitempty"`
	Level        string          `json:"level,omitempty"`
	Message      string          `json:"message,omitempty"`
	Status       string          `json:"status,omitempty"`
	ExitCode     int             `json:"exitCode,omitempty"`
	ErrorCode    string          `json:"errorCode,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
}

// EncodeEvent пишет событие одной JSON-строкой с '\n'.
func EncodeEvent(w io.Writer, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return writeLine(w, data)
}

func writeLine(w io.Writer, data []byte) error {
	// Allocate a fresh slice so we never mutate the caller's backing array
	// (append would reuse it if capacity allowed).
	line := make([]byte, len(data)+1)
	copy(line, data)
	line[len(data)] = '\n'
	_, err := w.Write(line)
	return err
}

// CommandDecoder читает команды из потока построчно.
type CommandDecoder struct{ sc *bufio.Scanner }

func NewCommandDecoder(r io.Reader) *CommandDecoder {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	return &CommandDecoder{sc: sc}
}

// Next читает следующую команду; io.EOF по концу потока.
func (d *CommandDecoder) Next() (Command, error) {
	for d.sc.Scan() {
		line := d.sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var c Command
		if err := json.Unmarshal(line, &c); err != nil {
			return Command{}, err
		}
		return c, nil
	}
	if err := d.sc.Err(); err != nil {
		return Command{}, err
	}
	return Command{}, io.EOF
}
