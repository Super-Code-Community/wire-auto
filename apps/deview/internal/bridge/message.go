// Package bridge — клиентская сторона протокола app↔runtime: кодирует команды,
// декодирует события (JSON Lines). Зеркало серверных типов в runtime/basic.
package bridge

import (
	"bufio"
	"encoding/json"
	"io"
)

const maxLineBytes = 1 << 20

type Command struct {
	Type  string `json:"type"`
	Dir   string `json:"dir,omitempty"`
	ID    string `json:"id,omitempty"`
	Value string `json:"value,omitempty"`
}

type Script struct {
	Name         string   `json:"name"`
	Dir          string   `json:"dir"`
	Language     string   `json:"language,omitempty"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities"`
}

type Event struct {
	Type         string          `json:"type"`
	ID           string          `json:"id,omitempty"`
	Scripts      []Script        `json:"scripts,omitempty"`
	Level        string          `json:"level,omitempty"`
	Message      string          `json:"message,omitempty"`
	Status       string          `json:"status,omitempty"`
	ExitCode     int             `json:"exitCode,omitempty"`
	ErrorCode    string          `json:"errorCode,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
}

func encodeCommand(w io.Writer, c Command) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

type eventDecoder struct{ sc *bufio.Scanner }

func newEventDecoder(r io.Reader) *eventDecoder {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	return &eventDecoder{sc: sc}
}

func (d *eventDecoder) next() (Event, error) {
	for d.sc.Scan() {
		line := d.sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return Event{}, err
		}
		return e, nil
	}
	if err := d.sc.Err(); err != nil {
		return Event{}, err
	}
	return Event{}, io.EOF
}
