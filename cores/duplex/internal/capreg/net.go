// Под-реестр сетевых возможностей: резолв, TCP-проба, граб баннера, интерфейсы.
package capreg

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"time"
)

var netCaps = map[string]Handler{
	"net.resolve":      netResolve,
	"net.tcp.connect":  netTCPConnect,
	"net.tcp.banner":   netTCPBanner,
	"net.interfaces":   netInterfaces,
}

func netResolve(params json.RawMessage) (json.RawMessage, string, error) {
	var p struct {
		Host      string `json:"host"`
		TimeoutMS int    `json:"timeout_ms"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Host == "" {
		return badParams("net.resolve requires a non-empty string field \"host\"")
	}
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(clampTimeout(p.TimeoutMS))*time.Millisecond)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, p.Host)
	if err != nil {
		return nil, "RESOLVE_FAILED", err
	}
	return okResult(map[string]any{"addrs": addrs})
}

// dialStatus классифицирует ошибку dial: таймаут → filtered, прочее → closed.
func dialStatus(err error) string {
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return "filtered"
	}
	return "closed"
}

func netTCPBanner(params json.RawMessage) (json.RawMessage, string, error) {
	var p struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		TimeoutMS int    `json:"timeout_ms"`
		ReadBytes int    `json:"read_bytes"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Host == "" || p.Port <= 0 || p.Port > 65535 {
		return badParams("net.tcp.banner requires \"host\" and \"port\" in 1..65535")
	}
	timeout := time.Duration(clampTimeout(p.TimeoutMS)) * time.Millisecond
	addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, "CONNECT_FAILED", err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, clampReadBytes(p.ReadBytes))
	n, _ := conn.Read(buf) // таймаут/EOF/частичное чтение — не ошибка capability
	return okResult(map[string]any{"banner": string(buf[:n]), "bytes": n})
}

func netTCPConnect(params json.RawMessage) (json.RawMessage, string, error) {
	var p struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		TimeoutMS int    `json:"timeout_ms"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Host == "" || p.Port <= 0 || p.Port > 65535 {
		return badParams("net.tcp.connect requires \"host\" and \"port\" in 1..65535")
	}
	timeout := time.Duration(clampTimeout(p.TimeoutMS)) * time.Millisecond
	addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return okResult(map[string]any{"status": dialStatus(err), "latency_ms": latency})
	}
	_ = conn.Close()
	return okResult(map[string]any{"status": "open", "latency_ms": latency})
}

func netInterfaces(params json.RawMessage) (json.RawMessage, string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, "INTERFACES_FAILED", err
	}
	type ifaceOut struct {
		Name  string   `json:"name"`
		MAC   string   `json:"mac"`
		Addrs []string `json:"addrs"`
		Flags []string `json:"flags"`
	}
	flagNames := []struct {
		flag net.Flags
		name string
	}{
		{net.FlagUp, "up"},
		{net.FlagLoopback, "loopback"},
		{net.FlagBroadcast, "broadcast"},
		{net.FlagPointToPoint, "pointtopoint"},
		{net.FlagMulticast, "multicast"},
	}
	out := make([]ifaceOut, 0, len(ifaces))
	for _, i := range ifaces {
		addrs := []string{}
		if aa, e := i.Addrs(); e == nil {
			for _, a := range aa {
				addrs = append(addrs, a.String())
			}
		}
		flags := []string{}
		for _, f := range flagNames {
			if i.Flags&f.flag != 0 {
				flags = append(flags, f.name)
			}
		}
		out = append(out, ifaceOut{Name: i.Name, MAC: i.HardwareAddr.String(), Addrs: addrs, Flags: flags})
	}
	return okResult(map[string]any{"interfaces": out})
}
