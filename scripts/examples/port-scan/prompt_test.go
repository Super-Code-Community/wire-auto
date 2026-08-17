package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestAskPrompt(t *testing.T) {
	in := bufio.NewReader(strings.NewReader(
		`{"type":"response","id":"7","result":{"value":"example.com"}}` + "\n"))
	var out bytes.Buffer
	enc := json.NewEncoder(&out)

	got := askPrompt(enc, in, "7", "Введите адрес")

	if got != "example.com" {
		t.Errorf("value = %q, want example.com", got)
	}

	var sent struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(out.Bytes(), &sent); err != nil {
		t.Fatalf("отправленный prompt не парсится: %v", err)
	}
	if sent.Type != "prompt" || sent.ID != "7" || sent.Message != "Введите адрес" {
		t.Errorf("отправлен prompt = %+v", sent)
	}
}

func TestAskPromptEmpty(t *testing.T) {
	in := bufio.NewReader(strings.NewReader(
		`{"type":"response","id":"1","result":{"value":""}}` + "\n"))
	enc := json.NewEncoder(&bytes.Buffer{})

	if got := askPrompt(enc, in, "1", "x"); got != "" {
		t.Errorf("value = %q, want empty", got)
	}
}
