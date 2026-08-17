package capreg

import (
	"encoding/json"
	"errors"
	"net"
	"testing"
)

func TestNetResolveLocalhost(t *testing.T) {
	res, code, err := Default["net.resolve"](json.RawMessage(`{"host":"localhost"}`))
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		Addrs []string `json:"addrs"`
	}
	if e := json.Unmarshal(res, &out); e != nil {
		t.Fatal(e)
	}
	if len(out.Addrs) == 0 {
		t.Fatal("localhost resolved to no addresses")
	}
}

func TestNetResolveBadParams(t *testing.T) {
	_, code, _ := Default["net.resolve"](json.RawMessage(`{}`))
	if code != "BAD_PARAMS" {
		t.Fatalf("code=%q, want BAD_PARAMS", code)
	}
}

func TestNetTCPConnectOpen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	params := json.RawMessage(`{"host":"127.0.0.1","port":` + port + `}`)
	res, code, err := Default["net.tcp.connect"](params)
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		Status string `json:"status"`
	}
	if e := json.Unmarshal(res, &out); e != nil {
		t.Fatal(e)
	}
	if out.Status != "open" {
		t.Fatalf("status=%q, want open", out.Status)
	}
}

func TestNetTCPConnectClosed(t *testing.T) {
	// Найти заведомо закрытый порт: слушаем, узнаём порт, закрываем.
	// Редкая гонка переиспользования порта (стал open) — ретраим свежим портом.
	var out struct {
		Status string `json:"status"`
	}
	for attempt := 0; attempt < 5; attempt++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		_, port, _ := net.SplitHostPort(ln.Addr().String())
		ln.Close() // порт свободен → connection refused
		params := json.RawMessage(`{"host":"127.0.0.1","port":` + port + `,"timeout_ms":500}`)
		res, code, _ := Default["net.tcp.connect"](params)
		if code != "" {
			t.Fatalf("code=%q", code)
		}
		if e := json.Unmarshal(res, &out); e != nil {
			t.Fatal(e)
		}
		if out.Status == "closed" {
			return // успех
		}
	}
	t.Fatalf("status=%q after retries, want closed", out.Status)
}

// fakeNetErr — синтетическая net.Error для проверки классификации без сети.
type fakeNetErr struct{ timeout bool }

func (e fakeNetErr) Error() string   { return "fake net error" }
func (e fakeNetErr) Timeout() bool   { return e.timeout }
func (e fakeNetErr) Temporary() bool { return false }

func TestDialStatusTimeoutIsFiltered(t *testing.T) {
	if got := dialStatus(fakeNetErr{timeout: true}); got != "filtered" {
		t.Fatalf("dialStatus(timeout) = %q, want filtered", got)
	}
}

func TestDialStatusOtherIsClosed(t *testing.T) {
	if got := dialStatus(errors.New("connection refused")); got != "closed" {
		t.Fatalf("dialStatus(other) = %q, want closed", got)
	}
}

func TestNetTCPConnectBadParams(t *testing.T) {
	_, code, _ := Default["net.tcp.connect"](json.RawMessage(`{"host":"x","port":0}`))
	if code != "BAD_PARAMS" {
		t.Fatalf("code=%q, want BAD_PARAMS", code)
	}
}

func TestNetTCPBanner(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, e := ln.Accept()
		if e != nil {
			return
		}
		c.Write([]byte("SSH-2.0-test"))
		c.Close()
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	params := json.RawMessage(`{"host":"127.0.0.1","port":` + port + `}`)
	res, code, err := Default["net.tcp.banner"](params)
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		Banner string `json:"banner"`
		Bytes  int    `json:"bytes"`
	}
	if e := json.Unmarshal(res, &out); e != nil {
		t.Fatal(e)
	}
	if out.Banner != "SSH-2.0-test" || out.Bytes != len("SSH-2.0-test") {
		t.Fatalf("banner=%q bytes=%d", out.Banner, out.Bytes)
	}
}

func TestNetInterfaces(t *testing.T) {
	res, code, err := Default["net.interfaces"](json.RawMessage(`{}`))
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		Interfaces []struct {
			Name  string   `json:"name"`
			Addrs []string `json:"addrs"`
			Flags []string `json:"flags"`
		} `json:"interfaces"`
	}
	if e := json.Unmarshal(res, &out); e != nil {
		t.Fatal(e)
	}
	if len(out.Interfaces) == 0 {
		t.Fatal("no network interfaces reported")
	}
}
