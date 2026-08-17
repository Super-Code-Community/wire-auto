// Package protocol описывает сообщения моста и кодек JSON Lines:
// одно сообщение = одна строка компактного JSON + '\n'.
package protocol

import (
	"bufio"
	"encoding/json"
	"io"
)

const (
	TypeHello    = "hello"
	TypeReady    = "ready"
	TypeLog      = "log"
	TypeDone     = "done"
	TypeCancel   = "cancel"
	TypeRequest  = "request"
	TypeResponse = "response"
)

// MaxLineBytes bounds a single protocol line to guard against adversarial input.
const MaxLineBytes = 1 << 20 // 1 MiB

type Message struct {
	Type     string          `json:"type"`
	Protocol int             `json:"protocol,omitempty"`
	CoreAPI  int             `json:"coreApi,omitempty"`
	Args     []string        `json:"args,omitempty"`
	Level    string          `json:"level,omitempty"`
	Message  string          `json:"message,omitempty"`
	ExitCode *int            `json:"exitCode,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	// v2 additions
	ID         string          `json:"id,omitempty"`
	Capability string          `json:"capability,omitempty"`
	Params     json.RawMessage `json:"params,omitempty"`
	Code       string          `json:"code,omitempty"`
}

func Encode(w io.Writer, m Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

type Decoder struct {
	sc *bufio.Scanner
}

func NewDecoder(r io.Reader) *Decoder {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineBytes)
	return &Decoder{sc: sc}
}

// Next читает следующее сообщение. Возвращает io.EOF, когда поток кончился.
func (d *Decoder) Next() (Message, error) {
	for d.sc.Scan() {
		line := append([]byte(nil), d.sc.Bytes()...)
		if len(line) == 0 {
			continue
		}
		var m Message
		if err := json.Unmarshal(line, &m); err != nil {
			return Message{}, err
		}
		return m, nil
	}
	if err := d.sc.Err(); err != nil {
		return Message{}, err
	}
	return Message{}, io.EOF
}
