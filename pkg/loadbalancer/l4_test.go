// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"net"
	"testing"
	"time"
)

func makeL4PoolConfig() *Config {
	c := DefaultConfig()
	c.Algorithm = AlgorithmRoundRobin
	c.Protocol = ProtocolTCP
	c.ListenAddress = "127.0.0.1:0"
	c.Backends = []BackendConfig{
		{Name: "b1", Address: "127.0.0.1:9001", Weight: 1, Enabled: true},
	}
	return c
}

func TestNewL4Proxy(t *testing.T) {
	config := makeL4PoolConfig()
	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("NewBackendPool: %v", err)
	}
	ha := NewHealthAggregator(config.HealthCheck, pool)

	proxy, err := NewL4Proxy(config, pool, ha)
	if err != nil {
		t.Fatalf("NewL4Proxy: %v", err)
	}
	if proxy == nil {
		t.Fatal("proxy is nil")
	}
}

func TestL4ProxyStats(t *testing.T) {
	config := makeL4PoolConfig()
	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("NewBackendPool: %v", err)
	}
	ha := NewHealthAggregator(config.HealthCheck, pool)

	proxy, err := NewL4Proxy(config, pool, ha)
	if err != nil {
		t.Fatalf("NewL4Proxy: %v", err)
	}

	stats := proxy.Stats()
	if stats.ActiveConnections != 0 {
		t.Errorf("ActiveConnections = %d, want 0", stats.ActiveConnections)
	}
	if stats.TotalConnections != 0 {
		t.Errorf("TotalConnections = %d, want 0", stats.TotalConnections)
	}
}

func TestL4ProxyCallbacks(t *testing.T) {
	config := makeL4PoolConfig()
	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("NewBackendPool: %v", err)
	}
	ha := NewHealthAggregator(config.HealthCheck, pool)

	proxy, err := NewL4Proxy(config, pool, ha)
	if err != nil {
		t.Fatalf("NewL4Proxy: %v", err)
	}

	proxy.OnConnect(func(client, backend string) {})
	proxy.OnDisconnect(func(client, backend string, dur time.Duration, in, out int64) {})
	proxy.OnError(func(addr string, err error) {})

	if proxy.onConnect == nil || proxy.onDisconnect == nil || proxy.onError == nil {
		t.Error("callbacks should be set")
	}
}

func TestL4ProxyStartStop(t *testing.T) {
	config := makeL4PoolConfig()
	config.DrainTimeout = 100 * time.Millisecond
	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("NewBackendPool: %v", err)
	}
	ha := NewHealthAggregator(config.HealthCheck, pool)

	proxy, err := NewL4Proxy(config, pool, ha)
	if err != nil {
		t.Fatalf("NewL4Proxy: %v", err)
	}

	if err := proxy.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Double start should fail
	if err := proxy.Start(); err == nil {
		t.Error("expected error for double start")
	}

	if err := proxy.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Double stop should fail
	if err := proxy.Stop(); err == nil {
		t.Error("expected error for double stop")
	}
}

func TestL4ProxyUnsupportedProtocol(t *testing.T) {
	config := makeL4PoolConfig()
	config.Protocol = "sctp"
	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("NewBackendPool: %v", err)
	}
	ha := NewHealthAggregator(config.HealthCheck, pool)

	proxy, err := NewL4Proxy(config, pool, ha)
	if err != nil {
		t.Fatalf("NewL4Proxy: %v", err)
	}

	if err := proxy.Start(); err == nil {
		t.Error("expected error for unsupported protocol")
	}
}

func TestSetTCPKeepalive(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Enable keepalive
	err = SetTCPKeepalive(conn, TCPKeepaliveConfig{
		Enabled:  true,
		Idle:     30 * time.Second,
		Interval: 5 * time.Second,
		Count:    3,
	})
	if err != nil {
		t.Errorf("SetTCPKeepalive(enabled): %v", err)
	}

	// Disable keepalive
	err = SetTCPKeepalive(conn, TCPKeepaliveConfig{Enabled: false})
	if err != nil {
		t.Errorf("SetTCPKeepalive(disabled): %v", err)
	}
}

func TestSetTCPKeepalive_NonTCP(t *testing.T) {
	c1, _ := net.Pipe()
	defer c1.Close()

	err := SetTCPKeepalive(c1, TCPKeepaliveConfig{Enabled: true})
	if err != nil {
		t.Errorf("expected nil for non-TCP conn, got: %v", err)
	}
}

func TestHalfCloseConn(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	hc := &HalfCloseConn{Conn: c1}
	if err := hc.CloseWrite(); err != nil {
		t.Errorf("CloseWrite: %v", err)
	}
	if err := hc.CloseRead(); err != nil {
		t.Errorf("CloseRead: %v", err)
	}
}

func TestNewProxyProtocolListener(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer inner.Close()

	ppl := NewProxyProtocolListener(inner)
	if ppl == nil {
		t.Fatal("nil listener")
	}

	addr := ppl.Addr()
	if addr == nil {
		t.Error("Addr() returned nil")
	}

	ppl.Close()
}

func TestPortRangeListener_EmptyAddr(t *testing.T) {
	prl := &PortRangeListener{
		stopCh: make(chan struct{}),
	}
	if addr := prl.Addr(); addr != nil {
		t.Errorf("expected nil addr for empty listener, got %v", addr)
	}
}

func TestProxyProtocolConn_Read(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	ppc := &proxyProtocolConn{
		Conn:   c1,
		peeked: []byte("hello"),
	}

	buf := make([]byte, 10)
	n, err := ppc.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("got %q, want hello", string(buf[:n]))
	}
}

func TestProxyProtocolConn_RemoteAddr(t *testing.T) {
	c1, _ := net.Pipe()
	defer c1.Close()

	realAddr := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234}
	ppc := &proxyProtocolConn{
		Conn:     c1,
		realAddr: realAddr,
	}
	if got := ppc.RemoteAddr(); got != realAddr {
		t.Errorf("RemoteAddr = %v, want %v", got, realAddr)
	}

	// Without real addr — falls back to Conn
	ppc2 := &proxyProtocolConn{Conn: c1}
	if got := ppc2.RemoteAddr(); got == nil {
		t.Error("RemoteAddr should fall back to Conn.RemoteAddr()")
	}
}
