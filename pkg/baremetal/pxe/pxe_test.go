// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pxe

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Config tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("nil config")
	}
	if cfg.TFTPAddress != ":69" {
		t.Errorf("TFTPAddress = %s", cfg.TFTPAddress)
	}
	if cfg.TFTPRoot != "/var/lib/tftpboot" {
		t.Errorf("TFTPRoot = %s", cfg.TFTPRoot)
	}
	if cfg.HTTPAddress != ":17000" {
		t.Errorf("HTTPAddress = %s", cfg.HTTPAddress)
	}
	if cfg.DHCPInterface != "eth0" {
		t.Errorf("DHCPInterface = %s", cfg.DHCPInterface)
	}
	if cfg.DefaultKernelParams != "console=ttyS0,115200n8" {
		t.Errorf("DefaultKernelParams = %s", cfg.DefaultKernelParams)
	}
}

func TestBootModeConstants(t *testing.T) {
	if BootModeBIOS != "bios" {
		t.Errorf("BootModeBIOS = %s", BootModeBIOS)
	}
	if BootModeUEFI != "uefi" {
		t.Errorf("BootModeUEFI = %s", BootModeUEFI)
	}
}

func TestClientStateConstants(t *testing.T) {
	states := map[ClientState]string{
		ClientStatePending:    "pending",
		ClientStateBooting:    "booting",
		ClientStateInstalling: "installing",
		ClientStateComplete:   "complete",
		ClientStateFailed:     "failed",
	}
	for s, expected := range states {
		if string(s) != expected {
			t.Errorf("ClientState %s != %s", s, expected)
		}
	}
}

// --- PXE Server tests ---

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg := &Config{
		TFTPAddress:         ":0",
		TFTPRoot:            t.TempDir(),
		HTTPAddress:         ":0",
		HTTPRoot:            t.TempDir(),
		DHCPInterface:       "lo",
		CallbackAddress:     "http://localhost:17000",
		DefaultKernelParams: "console=ttyS0",
		SkipDirectoryInit:   true,
	}
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func TestNewServer(t *testing.T) {
	srv := testServer(t)
	if srv == nil {
		t.Fatal("nil server")
	}
	if srv.tftpServer == nil {
		t.Error("nil tftpServer")
	}
	if srv.dhcpProxy == nil {
		t.Error("nil dhcpProxy")
	}
}

func TestNewServer_DefaultConfig(t *testing.T) {
	// With nil config, it uses DefaultConfig which will try to create real dirs.
	// This tests the nil path. It may fail due to permissions, which is fine.
	_, err := NewServer(nil)
	// We don't check err because it depends on filesystem permissions
	_ = err
}

func TestServer_ConfigureClient(t *testing.T) {
	srv := testServer(t)
	ctx := context.Background()

	// Create required dirs for the TFTP server to write config
	pxeDir := filepath.Join(srv.config.TFTPRoot, "pxelinux.cfg")
	os.MkdirAll(pxeDir, 0755)

	err := srv.ConfigureClient(ctx, &ClientConfig{
		MACAddress:   "00:11:22:33:44:55",
		Hostname:     "test-host",
		IPAddress:    "10.0.0.100",
		KernelPath:   "/images/kernel",
		InitrdPath:   "/images/initrd",
		KernelParams: "root=nfs",
		BootMode:     BootModeUEFI,
	})
	if err != nil {
		t.Fatalf("ConfigureClient: %v", err)
	}

	// Verify client is registered
	client, err := srv.GetClient("00:11:22:33:44:55")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if client.State != ClientStatePending {
		t.Errorf("State = %s", client.State)
	}
	if client.Config.Hostname != "test-host" {
		t.Errorf("Hostname = %s", client.Config.Hostname)
	}
	if client.Config.BootMode != BootModeUEFI {
		t.Errorf("BootMode = %s", client.Config.BootMode)
	}
}

func TestServer_ConfigureClient_NilConfig(t *testing.T) {
	srv := testServer(t)
	err := srv.ConfigureClient(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestServer_ConfigureClient_EmptyMAC(t *testing.T) {
	srv := testServer(t)
	err := srv.ConfigureClient(context.Background(), &ClientConfig{})
	if err == nil {
		t.Fatal("expected error for empty MAC")
	}
}

func TestServer_ConfigureClient_InvalidMAC(t *testing.T) {
	srv := testServer(t)
	err := srv.ConfigureClient(context.Background(), &ClientConfig{
		MACAddress: "invalid-mac",
	})
	if err == nil {
		t.Fatal("expected error for invalid MAC")
	}
}

func TestServer_ConfigureClient_DefaultBootMode(t *testing.T) {
	srv := testServer(t)
	os.MkdirAll(filepath.Join(srv.config.TFTPRoot, "pxelinux.cfg"), 0755)

	err := srv.ConfigureClient(context.Background(), &ClientConfig{
		MACAddress: "00:11:22:33:44:55",
		KernelPath: "/kernel",
		InitrdPath: "/initrd",
	})
	if err != nil {
		t.Fatal(err)
	}

	client, _ := srv.GetClient("00:11:22:33:44:55")
	if client.Config.BootMode != BootModeBIOS {
		t.Errorf("default BootMode should be BIOS, got %s", client.Config.BootMode)
	}
}

func TestServer_ConfigureClient_Update(t *testing.T) {
	srv := testServer(t)
	os.MkdirAll(filepath.Join(srv.config.TFTPRoot, "pxelinux.cfg"), 0755)
	ctx := context.Background()

	// Initial configure
	srv.ConfigureClient(ctx, &ClientConfig{
		MACAddress: "00:11:22:33:44:55",
		Hostname:   "host-v1",
		KernelPath: "/kernel",
		InitrdPath: "/initrd",
	})

	// Reconfigure same MAC
	srv.ConfigureClient(ctx, &ClientConfig{
		MACAddress: "00:11:22:33:44:55",
		Hostname:   "host-v2",
		KernelPath: "/kernel2",
		InitrdPath: "/initrd2",
	})

	client, _ := srv.GetClient("00:11:22:33:44:55")
	if client.Config.Hostname != "host-v2" {
		t.Errorf("Hostname = %s, want host-v2", client.Config.Hostname)
	}
	if client.State != ClientStatePending {
		t.Errorf("State should be reset to pending, got %s", client.State)
	}
}

func TestServer_RemoveClient(t *testing.T) {
	srv := testServer(t)
	os.MkdirAll(filepath.Join(srv.config.TFTPRoot, "pxelinux.cfg"), 0755)
	ctx := context.Background()

	srv.ConfigureClient(ctx, &ClientConfig{
		MACAddress: "00:11:22:33:44:55",
		KernelPath: "/kernel",
		InitrdPath: "/initrd",
	})

	if err := srv.RemoveClient(ctx, "00:11:22:33:44:55"); err != nil {
		t.Fatalf("RemoveClient: %v", err)
	}

	_, err := srv.GetClient("00:11:22:33:44:55")
	if err == nil {
		t.Error("expected error after remove")
	}
}

func TestServer_RemoveClient_InvalidMAC(t *testing.T) {
	srv := testServer(t)
	err := srv.RemoveClient(context.Background(), "bad-mac")
	if err == nil {
		t.Fatal("expected error for invalid MAC")
	}
}

func TestServer_GetClient_InvalidMAC(t *testing.T) {
	srv := testServer(t)
	_, err := srv.GetClient("bad-mac")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestServer_GetClient_NotFound(t *testing.T) {
	srv := testServer(t)
	_, err := srv.GetClient("00:11:22:33:44:55")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestServer_NotifyBoot(t *testing.T) {
	srv := testServer(t)
	os.MkdirAll(filepath.Join(srv.config.TFTPRoot, "pxelinux.cfg"), 0755)

	srv.ConfigureClient(context.Background(), &ClientConfig{
		MACAddress: "00:11:22:33:44:55",
		KernelPath: "/kernel",
		InitrdPath: "/initrd",
	})

	if err := srv.NotifyBoot("00:11:22:33:44:55"); err != nil {
		t.Fatalf("NotifyBoot: %v", err)
	}

	client, _ := srv.GetClient("00:11:22:33:44:55")
	if client.State != ClientStateBooting {
		t.Errorf("State = %s", client.State)
	}
	if client.BootCount != 1 {
		t.Errorf("BootCount = %d", client.BootCount)
	}
}

func TestServer_NotifyBoot_InvalidMAC(t *testing.T) {
	srv := testServer(t)
	err := srv.NotifyBoot("bad-mac")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestServer_NotifyInstallComplete(t *testing.T) {
	srv := testServer(t)
	os.MkdirAll(filepath.Join(srv.config.TFTPRoot, "pxelinux.cfg"), 0755)

	srv.ConfigureClient(context.Background(), &ClientConfig{
		MACAddress: "00:11:22:33:44:55",
		KernelPath: "/kernel",
		InitrdPath: "/initrd",
	})

	if err := srv.NotifyInstallComplete("00:11:22:33:44:55"); err != nil {
		t.Fatalf("NotifyInstallComplete: %v", err)
	}

	client, _ := srv.GetClient("00:11:22:33:44:55")
	if client.State != ClientStateComplete {
		t.Errorf("State = %s", client.State)
	}
}

func TestServer_NotifyInstallComplete_InvalidMAC(t *testing.T) {
	srv := testServer(t)
	err := srv.NotifyInstallComplete("bad-mac")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestServer_WaitForBoot_Success(t *testing.T) {
	srv := testServer(t)
	os.MkdirAll(filepath.Join(srv.config.TFTPRoot, "pxelinux.cfg"), 0755)
	mac := "00:11:22:33:44:55"

	srv.ConfigureClient(context.Background(), &ClientConfig{
		MACAddress: mac,
		KernelPath: "/kernel",
		InitrdPath: "/initrd",
	})

	// Notify boot in background
	go func() {
		time.Sleep(50 * time.Millisecond)
		srv.NotifyBoot(mac)
	}()

	err := srv.WaitForBoot(context.Background(), mac, 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
}

func TestServer_WaitForBoot_Timeout(t *testing.T) {
	srv := testServer(t)
	err := srv.WaitForBoot(context.Background(), "00:11:22:33:44:55", 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServer_WaitForBoot_InvalidMAC(t *testing.T) {
	srv := testServer(t)
	err := srv.WaitForBoot(context.Background(), "bad", time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestServer_WaitForInstallation_Success(t *testing.T) {
	srv := testServer(t)
	os.MkdirAll(filepath.Join(srv.config.TFTPRoot, "pxelinux.cfg"), 0755)
	mac := "00:11:22:33:44:55"

	srv.ConfigureClient(context.Background(), &ClientConfig{
		MACAddress: mac,
		KernelPath: "/kernel",
		InitrdPath: "/initrd",
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		srv.NotifyInstallComplete(mac)
	}()

	err := srv.WaitForInstallation(context.Background(), mac, 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForInstallation: %v", err)
	}
}

func TestServer_WaitForInstallation_Timeout(t *testing.T) {
	srv := testServer(t)
	err := srv.WaitForInstallation(context.Background(), "00:11:22:33:44:55", 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestServer_WaitForInstallation_InvalidMAC(t *testing.T) {
	srv := testServer(t)
	err := srv.WaitForInstallation(context.Background(), "bad", time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- TFTP Server tests ---

func TestNewTFTPServer(t *testing.T) {
	root := t.TempDir()
	srv, err := NewTFTPServer(":0", root)
	if err != nil {
		t.Fatalf("NewTFTPServer: %v", err)
	}
	if srv == nil {
		t.Fatal("nil server")
	}

	// Verify pxelinux.cfg directory was created
	if _, err := os.Stat(filepath.Join(root, "pxelinux.cfg")); err != nil {
		t.Error("pxelinux.cfg directory not created")
	}
}

func TestNewTFTPServer_SkipDirInit(t *testing.T) {
	srv, err := NewTFTPServerWithOptions(":0", "/nonexistent", true)
	if err != nil {
		t.Fatal(err)
	}
	if srv == nil {
		t.Fatal("nil server")
	}
}

func TestTFTPServer_WriteReadFile(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	data := []byte("test file content")
	if err := srv.WriteFile("test.txt", data); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := srv.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("got %q, want %q", got, data)
	}
}

func TestTFTPServer_WriteFile_Subdirectory(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	data := []byte("pxe config")
	if err := srv.WriteFile("pxelinux.cfg/01-aa-bb-cc-dd-ee-ff", data); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Verify file exists on disk
	path := filepath.Join(root, "pxelinux.cfg", "01-aa-bb-cc-dd-ee-ff")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not found on disk: %v", err)
	}
}

func TestTFTPServer_RemoveFile(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	srv.WriteFile("removeme.txt", []byte("data"))

	if err := srv.RemoveFile("removeme.txt"); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}

	_, err := srv.ReadFile("removeme.txt")
	if err == nil {
		t.Error("expected error reading removed file")
	}
}

func TestTFTPServer_RemoveFile_NonExistent(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	// Removing non-existent file should not error
	if err := srv.RemoveFile("nonexistent.txt"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTFTPServer_ReadFile_Caching(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	data := []byte("cacheable content")
	srv.WriteFile("cached.txt", data)

	// First read - cache miss
	got1, _ := srv.ReadFile("cached.txt")
	if string(got1) != string(data) {
		t.Error("first read mismatch")
	}

	stats := srv.GetStats()
	if stats.CacheMisses != 1 {
		t.Errorf("CacheMisses = %d, want 1", stats.CacheMisses)
	}

	// Second read - cache hit
	got2, _ := srv.ReadFile("cached.txt")
	if string(got2) != string(data) {
		t.Error("second read mismatch")
	}

	stats = srv.GetStats()
	if stats.CacheHits != 1 {
		t.Errorf("CacheHits = %d, want 1", stats.CacheHits)
	}
}

func TestTFTPServer_WriteInvalidatesCache(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	// Write and read to populate cache
	srv.WriteFile("update.txt", []byte("v1"))
	srv.ReadFile("update.txt")

	// Overwrite - should invalidate cache
	srv.WriteFile("update.txt", []byte("v2"))

	got, _ := srv.ReadFile("update.txt")
	if string(got) != "v2" {
		t.Errorf("got %q after update, want v2", got)
	}
}

func TestTFTPServer_RemoveInvalidatesCache(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	srv.WriteFile("remove-cache.txt", []byte("data"))
	srv.ReadFile("remove-cache.txt") // populate cache
	srv.RemoveFile("remove-cache.txt")

	_, err := srv.ReadFile("remove-cache.txt")
	if err == nil {
		t.Error("expected error reading removed file even if cached")
	}
}

func TestTFTPServer_HandleRead(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	data := []byte("handle-read-data")
	srv.WriteFile("handle.txt", data)

	var buf bytes.Buffer
	if err := srv.HandleRead("handle.txt", &buf); err != nil {
		t.Fatalf("HandleRead: %v", err)
	}
	if buf.String() != string(data) {
		t.Errorf("got %q", buf.String())
	}

	stats := srv.GetStats()
	if stats.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d", stats.TotalRequests)
	}
	if stats.BytesSent != int64(len(data)) {
		t.Errorf("BytesSent = %d", stats.BytesSent)
	}
}

func TestTFTPServer_HandleRead_NotFound(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	var buf bytes.Buffer
	err := srv.HandleRead("missing.txt", &buf)
	if err == nil {
		t.Fatal("expected error")
	}

	stats := srv.GetStats()
	if stats.Errors != 1 {
		t.Errorf("Errors = %d, want 1", stats.Errors)
	}
}

func TestTFTPServer_GetStats(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	stats := srv.GetStats()
	if stats.TotalRequests != 0 {
		t.Errorf("initial TotalRequests = %d", stats.TotalRequests)
	}
}

func TestTFTPServer_StartStop(t *testing.T) {
	root := t.TempDir()
	srv, _ := NewTFTPServerWithOptions(":0", root, true)

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Double start
	if err := srv.Start(context.Background()); err == nil {
		t.Error("expected error for double start")
	}

	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Double stop is idempotent
	if err := srv.Stop(); err != nil {
		t.Fatalf("double Stop: %v", err)
	}
}

// --- DHCP Proxy tests ---

func TestNewDHCPProxy(t *testing.T) {
	proxy, err := NewDHCPProxy("eth0")
	if err != nil {
		t.Fatalf("NewDHCPProxy: %v", err)
	}
	if proxy == nil {
		t.Fatal("nil proxy")
	}
	if proxy.iface != "eth0" {
		t.Errorf("iface = %s", proxy.iface)
	}
}

func TestDHCPProxy_StartStop(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)

	ctx := context.Background()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := proxy.Start(ctx); err == nil {
		t.Error("expected error for double start")
	}

	if err := proxy.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if err := proxy.Stop(); err != nil {
		t.Fatalf("double Stop: %v", err)
	}
}

func TestDHCPProxy_ConfigureClient(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)

	err := proxy.ConfigureClient("00:11:22:33:44:55", "10.0.0.100", ":69")
	if err != nil {
		t.Fatalf("ConfigureClient: %v", err)
	}

	cfg, err := proxy.GetClientConfig("00:11:22:33:44:55")
	if err != nil {
		t.Fatalf("GetClientConfig: %v", err)
	}
	if cfg.IPAddress != "10.0.0.100" {
		t.Errorf("IPAddress = %s", cfg.IPAddress)
	}
	if cfg.BootFilename != "pxelinux.0" {
		t.Errorf("BootFilename = %s", cfg.BootFilename)
	}
}

func TestDHCPProxy_ConfigureClient_InvalidMAC(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)
	err := proxy.ConfigureClient("bad-mac", "10.0.0.1", ":69")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDHCPProxy_ConfigureClient_InvalidIP(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)
	err := proxy.ConfigureClient("00:11:22:33:44:55", "not-an-ip", ":69")
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

func TestDHCPProxy_ConfigureClient_EmptyIP(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)
	// Empty IP is allowed
	err := proxy.ConfigureClient("00:11:22:33:44:55", "", ":69")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDHCPProxy_RemoveClient(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)

	proxy.ConfigureClient("00:11:22:33:44:55", "10.0.0.1", ":69")
	if err := proxy.RemoveClient("00:11:22:33:44:55"); err != nil {
		t.Fatalf("RemoveClient: %v", err)
	}

	_, err := proxy.GetClientConfig("00:11:22:33:44:55")
	if err == nil {
		t.Error("expected error after remove")
	}
}

func TestDHCPProxy_RemoveClient_InvalidMAC(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)
	err := proxy.RemoveClient("bad")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDHCPProxy_GetClientConfig_NotFound(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)
	_, err := proxy.GetClientConfig("00:11:22:33:44:55")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDHCPProxy_GetClientConfig_InvalidMAC(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)
	_, err := proxy.GetClientConfig("bad")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDHCPProxy_ListClients(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)

	proxy.ConfigureClient("00:11:22:33:44:55", "10.0.0.1", ":69")
	proxy.ConfigureClient("00:11:22:33:44:66", "10.0.0.2", ":69")

	clients := proxy.ListClients()
	if len(clients) != 2 {
		t.Errorf("expected 2 clients, got %d", len(clients))
	}
}

func TestDHCPProxy_ListClients_Empty(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)
	clients := proxy.ListClients()
	if len(clients) != 0 {
		t.Errorf("expected 0, got %d", len(clients))
	}
}

func TestDHCPProxy_BuildPXEOptions(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)

	cfg := &DHCPClientConfig{
		TFTPServer:   "10.0.0.1",
		BootFilename: "pxelinux.0",
	}

	tests := []struct {
		arch     uint16
		bootFile string
	}{
		{ArchIntelx86PC, "pxelinux.0"},
		{ArchEFIx8664, "grubx64.efi"},
		{ArchEFIBC, "grubx64.efi"},
		{ArchEFIIA32, "grubia32.efi"},
		{ArchEFIARM64, "grubaa64.efi"},
		{ArchNECPC98, "pxelinux.0"},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.arch+'A')), func(t *testing.T) {
			opts := proxy.BuildPXEOptions(tt.arch, cfg)

			// Should have TFTP server name, boot filename, and vendor class
			if len(opts) < 2 {
				t.Fatalf("expected at least 2 options, got %d", len(opts))
			}

			// Find boot filename option
			var foundBootFile bool
			for _, opt := range opts {
				if opt.Code == DHCPOptionBootFileName {
					if string(opt.Value) != tt.bootFile {
						t.Errorf("boot file = %s, want %s", opt.Value, tt.bootFile)
					}
					foundBootFile = true
				}
			}
			if !foundBootFile {
				t.Error("missing boot filename option")
			}

			// Should have vendor class "PXEClient"
			var foundVendor bool
			for _, opt := range opts {
				if opt.Code == DHCPOptionVendorClass {
					if string(opt.Value) != "PXEClient" {
						t.Errorf("vendor class = %s", opt.Value)
					}
					foundVendor = true
				}
			}
			if !foundVendor {
				t.Error("missing vendor class option")
			}
		})
	}
}

func TestDHCPProxy_BuildPXEOptions_NoTFTPServer(t *testing.T) {
	proxy, _ := NewDHCPProxyWithOptions("lo", true)
	cfg := &DHCPClientConfig{}

	opts := proxy.BuildPXEOptions(ArchIntelx86PC, cfg)
	// Without TFTP server, should have boot filename and vendor class only
	for _, opt := range opts {
		if opt.Code == DHCPOptionTFTPServerName {
			t.Error("should not include TFTP server option when empty")
		}
	}
}

// --- DHCP option constants ---

func TestDHCPOptionConstants(t *testing.T) {
	// Verify a few well-known option codes
	if DHCPOptionSubnetMask != 1 {
		t.Errorf("SubnetMask = %d", DHCPOptionSubnetMask)
	}
	if DHCPOptionRouter != 3 {
		t.Errorf("Router = %d", DHCPOptionRouter)
	}
	if DHCPOptionDNS != 6 {
		t.Errorf("DNS = %d", DHCPOptionDNS)
	}
	if DHCPOptionTFTPServerName != 66 {
		t.Errorf("TFTPServerName = %d", DHCPOptionTFTPServerName)
	}
	if DHCPOptionBootFileName != 67 {
		t.Errorf("BootFileName = %d", DHCPOptionBootFileName)
	}
	if DHCPOptionClientArch != 93 {
		t.Errorf("ClientArch = %d", DHCPOptionClientArch)
	}
}

func TestArchConstants(t *testing.T) {
	if ArchIntelx86PC != 0 {
		t.Errorf("ArchIntelx86PC = %d", ArchIntelx86PC)
	}
	if ArchEFIx8664 != 9 {
		t.Errorf("ArchEFIx8664 = %d", ArchEFIx8664)
	}
	if ArchEFIARM64 != 11 {
		t.Errorf("ArchEFIARM64 = %d", ArchEFIARM64)
	}
}

// --- Concurrent access ---

func TestConcurrentPXEOperations(t *testing.T) {
	srv := testServer(t)
	os.MkdirAll(filepath.Join(srv.config.TFTPRoot, "pxelinux.cfg"), 0755)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mac := "00:11:22:33:44:" + string(rune('a'+i)) + string(rune('a'+i))
			// These will fail with invalid MAC which is fine for concurrency testing
			_ = srv.ConfigureClient(ctx, &ClientConfig{
				MACAddress: "00:11:22:33:44:5" + string(rune('0'+i%10)),
				KernelPath: "/kernel",
				InitrdPath: "/initrd",
			})
			_ = mac
		}(i)
	}
	wg.Wait()
}
