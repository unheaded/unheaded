// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()

	if c.Name != "default" {
		t.Errorf("expected name 'default', got %q", c.Name)
	}
	if c.ListenAddress != "0.0.0.0:8080" {
		t.Errorf("expected listen address '0.0.0.0:8080', got %q", c.ListenAddress)
	}
	if c.Protocol != ProtocolHTTP {
		t.Errorf("expected protocol http, got %s", c.Protocol)
	}
	if c.Algorithm != AlgorithmRoundRobin {
		t.Errorf("expected algorithm round-robin, got %s", c.Algorithm)
	}
	if c.HealthCheck.Interval != 10*time.Second {
		t.Errorf("expected health check interval 10s, got %v", c.HealthCheck.Interval)
	}
	if c.Timeouts.Connect != 5*time.Second {
		t.Errorf("expected connect timeout 5s, got %v", c.Timeouts.Connect)
	}
	if !c.MetricsEnabled {
		t.Error("expected metrics enabled by default")
	}
	if c.DrainTimeout != 30*time.Second {
		t.Errorf("expected drain timeout 30s, got %v", c.DrainTimeout)
	}
	if !c.ConnectionPool.Enabled {
		t.Error("expected connection pool enabled by default")
	}
	if c.SessionPersistence.Type != SessionPersistenceNone {
		t.Errorf("expected session persistence none, got %s", c.SessionPersistence.Type)
	}
}

func validConfig() *Config {
	c := DefaultConfig()
	c.Backends = []BackendConfig{
		{Name: "b1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
	}
	return c
}

func TestConfigValidate_Valid(t *testing.T) {
	c := validConfig()
	if err := c.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestConfigValidate_MissingName(t *testing.T) {
	c := validConfig()
	c.Name = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestConfigValidate_MissingListenAddress(t *testing.T) {
	c := validConfig()
	c.ListenAddress = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing listen address")
	}
}

func TestConfigValidate_InvalidListenAddress(t *testing.T) {
	c := validConfig()
	c.ListenAddress = "not-valid"
	if err := c.Validate(); err == nil {
		t.Error("expected error for invalid listen address")
	}
}

func TestConfigValidate_InvalidProtocol(t *testing.T) {
	c := validConfig()
	c.Protocol = "ftp"
	if err := c.Validate(); err == nil {
		t.Error("expected error for invalid protocol")
	}
}

func TestConfigValidate_ValidProtocols(t *testing.T) {
	for _, p := range []Protocol{ProtocolTCP, ProtocolUDP, ProtocolHTTP, ProtocolHTTPS} {
		c := validConfig()
		c.Protocol = p
		if p == ProtocolHTTPS {
			// HTTPS requires TLS config; skip deeper validation here
			continue
		}
		if err := c.Validate(); err != nil {
			t.Errorf("protocol %s should be valid, got: %v", p, err)
		}
	}
}

func TestConfigValidate_InvalidAlgorithm(t *testing.T) {
	c := validConfig()
	c.Algorithm = "magic"
	if err := c.Validate(); err == nil {
		t.Error("expected error for invalid algorithm")
	}
}

func TestConfigValidate_ValidAlgorithms(t *testing.T) {
	algos := []Algorithm{
		AlgorithmRoundRobin, AlgorithmLeastConnections, AlgorithmIPHash,
		AlgorithmWeighted, AlgorithmWeightedLeastConn, AlgorithmRandom,
	}
	for _, a := range algos {
		c := validConfig()
		c.Algorithm = a
		if err := c.Validate(); err != nil {
			t.Errorf("algorithm %s should be valid, got: %v", a, err)
		}
	}
}

func TestConfigValidate_NoBackends(t *testing.T) {
	c := validConfig()
	c.Backends = nil
	if err := c.Validate(); err == nil {
		t.Error("expected error for no backends")
	}
}

func TestConfigValidate_DuplicateBackendNames(t *testing.T) {
	c := validConfig()
	c.Backends = []BackendConfig{
		{Name: "dup", Address: "127.0.0.1:8081", Weight: 1},
		{Name: "dup", Address: "127.0.0.1:8082", Weight: 1},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for duplicate backend names")
	}
}

func TestConfigValidate_HTTPSRequiresTLS(t *testing.T) {
	c := validConfig()
	c.Protocol = ProtocolHTTPS
	c.TLS = nil
	if err := c.Validate(); err == nil {
		t.Error("expected error: HTTPS requires TLS config")
	}
}

func TestConfigValidate_InvalidBackend(t *testing.T) {
	c := validConfig()
	c.Backends = []BackendConfig{
		{Name: "", Address: "127.0.0.1:8081"},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for invalid backend")
	}
}

func TestBackendConfigValidate_Valid(t *testing.T) {
	b := BackendConfig{Name: "b1", Address: "127.0.0.1:8081", Weight: 50}
	if err := b.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestBackendConfigValidate_MissingName(t *testing.T) {
	b := BackendConfig{Address: "127.0.0.1:8081"}
	if err := b.Validate(); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestBackendConfigValidate_MissingAddress(t *testing.T) {
	b := BackendConfig{Name: "b1"}
	if err := b.Validate(); err == nil {
		t.Error("expected error for missing address")
	}
}

func TestBackendConfigValidate_InvalidWeight(t *testing.T) {
	tests := []struct {
		weight int
	}{
		{-1},
		{101},
	}
	for _, tt := range tests {
		b := BackendConfig{Name: "b1", Address: "127.0.0.1:8081", Weight: tt.weight}
		if err := b.Validate(); err == nil {
			t.Errorf("expected error for weight %d", tt.weight)
		}
	}
}

func TestBackendConfigValidate_NegativePriority(t *testing.T) {
	b := BackendConfig{Name: "b1", Address: "127.0.0.1:8081", Priority: -1}
	if err := b.Validate(); err == nil {
		t.Error("expected error for negative priority")
	}
}

func TestBackendConfigValidate_NegativeMaxConnections(t *testing.T) {
	b := BackendConfig{Name: "b1", Address: "127.0.0.1:8081", MaxConnections: -1}
	if err := b.Validate(); err == nil {
		t.Error("expected error for negative max_connections")
	}
}

func TestBackendConfigValidate_URLAddress(t *testing.T) {
	// Address that isn't host:port but is a valid URL
	b := BackendConfig{Name: "b1", Address: "http://example.com"}
	if err := b.Validate(); err != nil {
		t.Errorf("expected URL address to be accepted, got: %v", err)
	}
}

func TestTLSConfigValidate_MissingCert(t *testing.T) {
	tc := &TLSConfig{KeyFile: "/tmp/key.pem"}
	if err := tc.Validate(); err == nil {
		t.Error("expected error for missing cert_file")
	}
}

func TestTLSConfigValidate_MissingKey(t *testing.T) {
	tc := &TLSConfig{CertFile: "/tmp/cert.pem"}
	if err := tc.Validate(); err == nil {
		t.Error("expected error for missing key_file")
	}
}

func TestTLSConfigValidate_NonexistentFiles(t *testing.T) {
	tc := &TLSConfig{CertFile: "/nonexistent/cert.pem", KeyFile: "/nonexistent/key.pem"}
	if err := tc.Validate(); err == nil {
		t.Error("expected error for nonexistent cert file")
	}
}

func TestTLSConfigValidate_InvalidMinVersion(t *testing.T) {
	// Create temp files for cert/key
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, []byte("cert"), 0644)
	os.WriteFile(keyFile, []byte("key"), 0644)

	tc := &TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.0"}
	if err := tc.Validate(); err == nil {
		t.Error("expected error for invalid min_version")
	}
}

func TestTLSConfigValidate_ValidMinVersions(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, []byte("cert"), 0644)
	os.WriteFile(keyFile, []byte("key"), 0644)

	for _, v := range []string{"", "1.2", "1.3"} {
		tc := &TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: v}
		if err := tc.Validate(); err != nil {
			t.Errorf("min_version %q should be valid, got: %v", v, err)
		}
	}
}

func TestHealthCheckConfigValidate_Valid(t *testing.T) {
	hc := HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckTCP,
		Interval:         10 * time.Second,
		Timeout:          5 * time.Second,
		Retries:          3,
		SuccessThreshold: 2,
	}
	if err := hc.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestHealthCheckConfigValidate_ZeroInterval(t *testing.T) {
	hc := HealthCheckConfig{Type: HealthCheckTCP, Timeout: 5 * time.Second, Retries: 1, SuccessThreshold: 1}
	if err := hc.Validate(); err == nil {
		t.Error("expected error for zero interval")
	}
}

func TestHealthCheckConfigValidate_ZeroTimeout(t *testing.T) {
	hc := HealthCheckConfig{Type: HealthCheckTCP, Interval: 10 * time.Second, Retries: 1, SuccessThreshold: 1}
	if err := hc.Validate(); err == nil {
		t.Error("expected error for zero timeout")
	}
}

func TestHealthCheckConfigValidate_TimeoutGteInterval(t *testing.T) {
	hc := HealthCheckConfig{
		Type: HealthCheckTCP, Interval: 5 * time.Second, Timeout: 5 * time.Second,
		Retries: 1, SuccessThreshold: 1,
	}
	if err := hc.Validate(); err == nil {
		t.Error("expected error for timeout >= interval")
	}
}

func TestHealthCheckConfigValidate_ZeroRetries(t *testing.T) {
	hc := HealthCheckConfig{
		Type: HealthCheckTCP, Interval: 10 * time.Second, Timeout: 5 * time.Second,
		Retries: 0, SuccessThreshold: 1,
	}
	if err := hc.Validate(); err == nil {
		t.Error("expected error for zero retries")
	}
}

func TestHealthCheckConfigValidate_ZeroSuccessThreshold(t *testing.T) {
	hc := HealthCheckConfig{
		Type: HealthCheckTCP, Interval: 10 * time.Second, Timeout: 5 * time.Second,
		Retries: 1, SuccessThreshold: 0,
	}
	if err := hc.Validate(); err == nil {
		t.Error("expected error for zero success_threshold")
	}
}

func TestHealthCheckConfigValidate_HTTPRequiresPath(t *testing.T) {
	for _, hcType := range []HealthCheckType{HealthCheckHTTP, HealthCheckHTTPS} {
		hc := HealthCheckConfig{
			Type: hcType, Interval: 10 * time.Second, Timeout: 5 * time.Second,
			Retries: 1, SuccessThreshold: 1,
		}
		if err := hc.Validate(); err == nil {
			t.Errorf("expected error for %s without http_path", hcType)
		}
	}
}

func TestHealthCheckConfigValidate_ExecRequiresCommand(t *testing.T) {
	hc := HealthCheckConfig{
		Type: HealthCheckExec, Interval: 10 * time.Second, Timeout: 5 * time.Second,
		Retries: 1, SuccessThreshold: 1,
	}
	if err := hc.Validate(); err == nil {
		t.Error("expected error for exec without command")
	}
}

func TestHealthCheckConfigValidate_InvalidType(t *testing.T) {
	hc := HealthCheckConfig{
		Type: "grpc", Interval: 10 * time.Second, Timeout: 5 * time.Second,
		Retries: 1, SuccessThreshold: 1,
	}
	if err := hc.Validate(); err == nil {
		t.Error("expected error for invalid health check type")
	}
}

func TestSessionPersistenceConfigValidate_None(t *testing.T) {
	sp := SessionPersistenceConfig{Type: SessionPersistenceNone}
	if err := sp.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestSessionPersistenceConfigValidate_CookieRequiresName(t *testing.T) {
	sp := SessionPersistenceConfig{Type: SessionPersistenceCookie}
	if err := sp.Validate(); err == nil {
		t.Error("expected error for cookie persistence without name")
	}
}

func TestSessionPersistenceConfigValidate_CookieValid(t *testing.T) {
	sp := SessionPersistenceConfig{Type: SessionPersistenceCookie, CookieName: "SRV"}
	if err := sp.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestSessionPersistenceConfigValidate_SourceIP(t *testing.T) {
	sp := SessionPersistenceConfig{Type: SessionPersistenceSourceIP}
	if err := sp.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestSessionPersistenceConfigValidate_HeaderRequiresName(t *testing.T) {
	sp := SessionPersistenceConfig{Type: SessionPersistenceHeader}
	if err := sp.Validate(); err == nil {
		t.Error("expected error for header persistence without name")
	}
}

func TestSessionPersistenceConfigValidate_HeaderValid(t *testing.T) {
	sp := SessionPersistenceConfig{Type: SessionPersistenceHeader, HeaderName: "X-Session"}
	if err := sp.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestSessionPersistenceConfigValidate_InvalidType(t *testing.T) {
	sp := SessionPersistenceConfig{Type: "magic"}
	if err := sp.Validate(); err == nil {
		t.Error("expected error for invalid persistence type")
	}
}

func TestSessionPersistenceConfigValidate_NegativeTTL(t *testing.T) {
	sp := SessionPersistenceConfig{Type: SessionPersistenceNone, TTL: -1}
	if err := sp.Validate(); err == nil {
		t.Error("expected error for negative TTL")
	}
}

func TestSessionPersistenceConfigValidate_NegativeMaxEntries(t *testing.T) {
	sp := SessionPersistenceConfig{Type: SessionPersistenceNone, MaxEntries: -1}
	if err := sp.Validate(); err == nil {
		t.Error("expected error for negative max_entries")
	}
}

func TestTimeoutConfigValidate_Valid(t *testing.T) {
	tc := TimeoutConfig{
		Connect: 5 * time.Second,
		Read:    30 * time.Second,
		Write:   30 * time.Second,
		Idle:    60 * time.Second,
		Request: 120 * time.Second,
	}
	if err := tc.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestTimeoutConfigValidate_NegativeValues(t *testing.T) {
	tests := []struct {
		name string
		tc   TimeoutConfig
	}{
		{"connect", TimeoutConfig{Connect: -1}},
		{"read", TimeoutConfig{Read: -1}},
		{"write", TimeoutConfig{Write: -1}},
		{"idle", TimeoutConfig{Idle: -1}},
		{"request", TimeoutConfig{Request: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.tc.Validate(); err == nil {
				t.Errorf("expected error for negative %s timeout", tt.name)
			}
		})
	}
}

func TestLoadConfig_Valid(t *testing.T) {
	c := validConfig()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("expected valid load, got: %v", err)
	}
	if loaded.Name != c.Name {
		t.Errorf("expected name %q, got %q", c.Name, loaded.Name)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte("{invalid"), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadConfig_InvalidConfig(t *testing.T) {
	// Valid JSON but invalid config (no backends)
	data := `{"name":"test","listen_address":"0.0.0.0:80","protocol":"http","algorithm":"round-robin"}`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(data), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestSaveConfig(t *testing.T) {
	c := validConfig()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := SaveConfig(c, path); err != nil {
		t.Fatalf("expected save to succeed, got: %v", err)
	}

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("saved config is not valid JSON: %v", err)
	}
	if loaded.Name != c.Name {
		t.Errorf("expected name %q, got %q", c.Name, loaded.Name)
	}
}

func TestSaveConfig_InvalidPath(t *testing.T) {
	c := validConfig()
	err := SaveConfig(c, "/nonexistent/dir/config.json")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestConfigValidate_HealthCheckDisabled(t *testing.T) {
	c := validConfig()
	c.HealthCheck.Enabled = false
	c.HealthCheck.Interval = 0 // Would fail if validated
	if err := c.Validate(); err != nil {
		t.Errorf("expected valid when health check disabled, got: %v", err)
	}
}
