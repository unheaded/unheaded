// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package ipmi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- IPMI Client tests ---

func TestNewClient_Success(t *testing.T) {
	client, err := NewClient("192.168.1.100", &Credentials{
		Username: "admin",
		Password: "password",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client == nil {
		t.Fatal("nil client")
	}
	defer client.Close()

	if client.address != "192.168.1.100" {
		t.Errorf("address = %s", client.address)
	}
	if !client.connected {
		t.Error("should be connected after NewClient")
	}
}

func TestNewClient_EmptyAddress(t *testing.T) {
	_, err := NewClient("", &Credentials{Username: "admin", Password: "pass"})
	if err == nil {
		t.Fatal("expected error for empty address")
	}
	if !strings.Contains(err.Error(), "address is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewClient_NilCredentials(t *testing.T) {
	_, err := NewClient("192.168.1.100", nil)
	if err == nil {
		t.Fatal("expected error for nil credentials")
	}
	if !strings.Contains(err.Error(), "credentials are required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClient_Close(t *testing.T) {
	client, err := NewClient("192.168.1.100", &Credentials{Username: "admin", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if client.connected {
		t.Error("should not be connected after Close")
	}
	if client.sessionID != 0 {
		t.Error("sessionID should be 0 after Close")
	}
}

func testClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient("10.0.0.1", &Credentials{Username: "admin", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPowerOn(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	if err := c.PowerOn(context.Background()); err != nil {
		t.Fatalf("PowerOn: %v", err)
	}
}

func TestPowerOff(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	if err := c.PowerOff(context.Background()); err != nil {
		t.Fatalf("PowerOff: %v", err)
	}
}

func TestPowerCycle(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	if err := c.PowerCycle(context.Background()); err != nil {
		t.Fatalf("PowerCycle: %v", err)
	}
}

func TestReboot(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	if err := c.Reboot(context.Background()); err != nil {
		t.Fatalf("Reboot: %v", err)
	}
}

func TestSoftShutdown(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	if err := c.SoftShutdown(context.Background()); err != nil {
		t.Fatalf("SoftShutdown: %v", err)
	}
}

func TestGetPowerState(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	state, err := c.GetPowerState(context.Background())
	if err != nil {
		t.Fatalf("GetPowerState: %v", err)
	}
	if state != PowerStateOn {
		t.Errorf("state = %s, want %s", state, PowerStateOn)
	}
}

func TestGetChassisStatus(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	status, err := c.GetChassisStatus(context.Background())
	if err != nil {
		t.Fatalf("GetChassisStatus: %v", err)
	}
	if !status.PowerOn {
		t.Error("expected PowerOn = true")
	}
}

func TestSetBootDevice(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	devices := []BootDevice{
		BootDeviceNone, BootDevicePXE, BootDeviceDisk,
		BootDeviceCDROM, BootDeviceBIOS, BootDeviceUSB, BootDeviceRemote,
	}
	for _, d := range devices {
		if err := c.SetBootDevice(context.Background(), d); err != nil {
			t.Errorf("SetBootDevice(%s): %v", d, err)
		}
	}
}

func TestGetBootDevice(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	device, err := c.GetBootDevice(context.Background())
	if err != nil {
		t.Fatalf("GetBootDevice: %v", err)
	}
	if device != BootDeviceDisk {
		t.Errorf("device = %s", device)
	}
}

func TestGetSensors(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	sensors, err := c.GetSensors(context.Background())
	if err != nil {
		t.Fatalf("GetSensors: %v", err)
	}
	if len(sensors) != 3 {
		t.Errorf("expected 3 sensors, got %d", len(sensors))
	}

	// Verify first sensor
	if sensors[0].Name != "CPU Temp" {
		t.Errorf("first sensor name = %s", sensors[0].Name)
	}
	if sensors[0].Value != 45.0 {
		t.Errorf("CPU temp = %f", sensors[0].Value)
	}
}

func TestGetSEL(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	entries, err := c.GetSEL(context.Background())
	if err != nil {
		t.Fatalf("GetSEL: %v", err)
	}
	if entries == nil {
		t.Error("expected non-nil entries")
	}
}

func TestClearSEL(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	if err := c.ClearSEL(context.Background()); err != nil {
		t.Fatalf("ClearSEL: %v", err)
	}
}

func TestIdentifyOn(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	if err := c.IdentifyOn(context.Background(), 30*time.Second); err != nil {
		t.Fatalf("IdentifyOn: %v", err)
	}
}

func TestIdentifyOff(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	if err := c.IdentifyOff(context.Background()); err != nil {
		t.Fatalf("IdentifyOff: %v", err)
	}
}

func TestGetFRU(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	fru, err := c.GetFRU(context.Background())
	if err != nil {
		t.Fatalf("GetFRU: %v", err)
	}
	if fru.Manufacturer != "Unknown" {
		t.Errorf("Manufacturer = %s", fru.Manufacturer)
	}
	if fru.ProductName != "Unknown" {
		t.Errorf("ProductName = %s", fru.ProductName)
	}
}

func TestOpenConsole(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	cons, err := c.OpenConsole(context.Background())
	if err != nil {
		t.Fatalf("OpenConsole: %v", err)
	}
	if cons == nil {
		t.Fatal("nil console")
	}

	if err := cons.Close(); err != nil {
		t.Fatalf("Console.Close: %v", err)
	}
}

// Test all methods return error when disconnected
func TestDisconnectedErrors(t *testing.T) {
	c := testClient(t)
	c.Close()
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"PowerOn", func() error { return c.PowerOn(ctx) }},
		{"PowerOff", func() error { return c.PowerOff(ctx) }},
		{"PowerCycle", func() error { return c.PowerCycle(ctx) }},
		{"Reboot", func() error { return c.Reboot(ctx) }},
		{"SoftShutdown", func() error { return c.SoftShutdown(ctx) }},
		{"GetPowerState", func() error { _, e := c.GetPowerState(ctx); return e }},
		{"GetChassisStatus", func() error { _, e := c.GetChassisStatus(ctx); return e }},
		{"SetBootDevice", func() error { return c.SetBootDevice(ctx, BootDevicePXE) }},
		{"GetBootDevice", func() error { _, e := c.GetBootDevice(ctx); return e }},
		{"GetSensors", func() error { _, e := c.GetSensors(ctx); return e }},
		{"GetSEL", func() error { _, e := c.GetSEL(ctx); return e }},
		{"ClearSEL", func() error { return c.ClearSEL(ctx) }},
		{"IdentifyOn", func() error { return c.IdentifyOn(ctx, time.Second) }},
		{"IdentifyOff", func() error { return c.IdentifyOff(ctx) }},
		{"GetFRU", func() error { _, e := c.GetFRU(ctx); return e }},
		{"OpenConsole", func() error { _, e := c.OpenConsole(ctx); return e }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Error("expected error when disconnected")
			}
			if !strings.Contains(err.Error(), "not connected") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// --- Boot device and power state constants ---

func TestBootDeviceConstants(t *testing.T) {
	devices := map[BootDevice]string{
		BootDeviceNone:   "none",
		BootDevicePXE:    "pxe",
		BootDeviceDisk:   "disk",
		BootDeviceCDROM:  "cdrom",
		BootDeviceBIOS:   "bios",
		BootDeviceUSB:    "usb",
		BootDeviceRemote: "remote",
	}
	for d, expected := range devices {
		if string(d) != expected {
			t.Errorf("BootDevice %s != %s", d, expected)
		}
	}
}

func TestPowerStateConstants(t *testing.T) {
	if PowerStateOn != "on" {
		t.Errorf("PowerStateOn = %s", PowerStateOn)
	}
	if PowerStateOff != "off" {
		t.Errorf("PowerStateOff = %s", PowerStateOff)
	}
	if PowerStateUnknown != "unknown" {
		t.Errorf("PowerStateUnknown = %s", PowerStateUnknown)
	}
}

// --- Redfish Client tests ---

func TestNewRedfishClient_EmptyAddress(t *testing.T) {
	_, err := NewRedfishClient("", &Credentials{Username: "admin", Password: "pass"})
	if err == nil {
		t.Fatal("expected error for empty address")
	}
}

func TestNewRedfishClient_NilCredentials(t *testing.T) {
	_, err := NewRedfishClient("192.168.1.1", nil)
	if err == nil {
		t.Fatal("expected error for nil credentials")
	}
}

func TestNewRedfishClient_AddressNormalization(t *testing.T) {
	creds := &Credentials{Username: "admin", Password: "pass"}

	// Without scheme
	c, err := NewRedfishClient("192.168.1.1", creds)
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://192.168.1.1" {
		t.Errorf("baseURL = %s", c.baseURL)
	}
	c.Close()

	// With https scheme
	c, err = NewRedfishClient("https://192.168.1.1", creds)
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://192.168.1.1" {
		t.Errorf("baseURL = %s", c.baseURL)
	}
	c.Close()

	// With http scheme
	c, err = NewRedfishClient("http://192.168.1.1", creds)
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "http://192.168.1.1" {
		t.Errorf("baseURL = %s", c.baseURL)
	}
	c.Close()

	// Trailing slash removal
	c, err = NewRedfishClient("https://192.168.1.1/", creds)
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://192.168.1.1" {
		t.Errorf("baseURL = %s (trailing slash not removed)", c.baseURL)
	}
	c.Close()
}

func TestRedfishClient_Close(t *testing.T) {
	c, err := NewRedfishClient("192.168.1.1", &Credentials{Username: "admin", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.sessionID != "" {
		t.Error("sessionID should be cleared")
	}
	if c.authToken != "" {
		t.Error("authToken should be cleared")
	}

	// Double close should be fine
	if err := c.Close(); err != nil {
		t.Fatalf("double Close: %v", err)
	}
}

func TestRedfishClient_GetSystems(t *testing.T) {
	c, err := NewRedfishClient("192.168.1.1", &Credentials{Username: "admin", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	systems, err := c.GetSystems(context.Background())
	if err != nil {
		t.Fatalf("GetSystems: %v", err)
	}
	if len(systems) != 1 {
		t.Fatalf("expected 1 system, got %d", len(systems))
	}
	if systems[0].SerialNumber != "ABC123" {
		t.Errorf("SerialNumber = %s", systems[0].SerialNumber)
	}
}

func TestRedfishClient_GetChassis(t *testing.T) {
	c, err := NewRedfishClient("192.168.1.1", &Credentials{Username: "admin", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	chassis, err := c.GetChassis(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(chassis) != 1 {
		t.Fatalf("expected 1 chassis, got %d", len(chassis))
	}
	if chassis[0].ChassisType != "RackMount" {
		t.Errorf("ChassisType = %s", chassis[0].ChassisType)
	}
}

func TestRedfishClient_GetThermal(t *testing.T) {
	c, err := NewRedfishClient("192.168.1.1", &Credentials{Username: "admin", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	thermal, err := c.GetThermal(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(thermal.Temperatures) != 1 {
		t.Errorf("expected 1 temperature, got %d", len(thermal.Temperatures))
	}
	if len(thermal.Fans) != 1 {
		t.Errorf("expected 1 fan, got %d", len(thermal.Fans))
	}
}

// Test Redfish with a mock HTTP server for resetSystem, SetBootSource, etc.
func TestRedfishClient_ResetSystem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if r.Header.Get("X-Auth-Token") == "" {
			t.Error("missing X-Auth-Token header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:     server.URL,
		credentials: &Credentials{Username: "admin", Password: "pass"},
		httpClient:  server.Client(),
		sessionID:   "test",
		authToken:   "test-token",
	}

	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"PowerOn", func() error { return c.PowerOn(ctx, "1") }},
		{"PowerOff", func() error { return c.PowerOff(ctx, "1") }},
		{"GracefulShutdown", func() error { return c.GracefulShutdown(ctx, "1") }},
		{"ForceRestart", func() error { return c.ForceRestart(ctx, "1") }},
		{"GracefulRestart", func() error { return c.GracefulRestart(ctx, "1") }},
		{"PowerCycle", func() error { return c.PowerCycle(ctx, "1") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != nil {
				t.Errorf("%s: %v", tt.name, err)
			}
		})
	}
}

func TestRedfishClient_ResetSystem_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:     server.URL,
		credentials: &Credentials{Username: "admin", Password: "pass"},
		httpClient:  server.Client(),
		sessionID:   "test",
		authToken:   "test-token",
	}

	err := c.PowerOn(context.Background(), "1")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "reset failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRedfishClient_SetBootSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
		sessionID:  "test",
		authToken:  "test-token",
	}

	if err := c.SetBootSource(context.Background(), "1", "Pxe", "Once"); err != nil {
		t.Fatal(err)
	}
}

func TestRedfishClient_SetBootSource_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
		authToken:  "test-token",
	}

	err := c.SetBootSource(context.Background(), "1", "Pxe", "Once")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRedfishClient_SetBootPXE(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
		authToken:  "token",
	}

	if err := c.SetBootPXE(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/redfish/v1/Systems/1" {
		t.Errorf("path = %s", gotPath)
	}
}

func TestRedfishClient_SetIndicatorLED(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
		authToken:  "token",
	}

	if err := c.SetIndicatorLED(context.Background(), "1", "Blinking"); err != nil {
		t.Fatal(err)
	}
}

func TestRedfishClient_SetIndicatorLED_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
		authToken:  "token",
	}

	err := c.SetIndicatorLED(context.Background(), "1", "Lit")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRedfishClient_MountVirtualMedia(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		expected := fmt.Sprintf("/redfish/v1/Managers/%s/VirtualMedia/%s/Actions/VirtualMedia.InsertMedia", "1", "CD")
		if r.URL.Path != expected {
			t.Errorf("path = %s, want %s", r.URL.Path, expected)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
		authToken:  "token",
	}

	if err := c.MountVirtualMedia(context.Background(), "1", "CD", "http://example.com/image.iso"); err != nil {
		t.Fatal(err)
	}
}

func TestRedfishClient_MountVirtualMedia_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
		authToken:  "token",
	}

	err := c.MountVirtualMedia(context.Background(), "1", "CD", "http://example.com/image.iso")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRedfishClient_EjectVirtualMedia(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
		authToken:  "token",
	}

	if err := c.EjectVirtualMedia(context.Background(), "1", "CD"); err != nil {
		t.Fatal(err)
	}
}

func TestRedfishClient_EjectVirtualMedia_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
		authToken:  "token",
	}

	err := c.EjectVirtualMedia(context.Background(), "1", "CD")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRedfishClient_GetServiceRoot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/redfish/v1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		root := ServiceRoot{
			ID:             "RootService",
			Name:           "Root Service",
			RedfishVersion: "1.13.0",
			UUID:           "test-uuid",
		}
		json.NewEncoder(w).Encode(root)
	}))
	defer server.Close()

	c := &RedfishClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
		authToken:  "token",
	}

	root, err := c.GetServiceRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if root.RedfishVersion != "1.13.0" {
		t.Errorf("RedfishVersion = %s", root.RedfishVersion)
	}
	if root.UUID != "test-uuid" {
		t.Errorf("UUID = %s", root.UUID)
	}
}

// --- JSON serialization tests for types ---

func TestChassisStatus_JSON(t *testing.T) {
	status := ChassisStatus{
		PowerOn:       true,
		CoolingFault:  true,
		DriveFault:    false,
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}

	var decoded ChassisStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.PowerOn {
		t.Error("PowerOn should be true")
	}
	if !decoded.CoolingFault {
		t.Error("CoolingFault should be true")
	}
}

func TestSensorReading_JSON(t *testing.T) {
	sr := SensorReading{
		Name:   "CPU Temp",
		Value:  65.5,
		Unit:   "C",
		Status: "warning",
	}
	data, err := json.Marshal(sr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "CPU Temp") {
		t.Error("missing name in JSON")
	}
}

func TestFRUInfo_JSON(t *testing.T) {
	fru := FRUInfo{
		Manufacturer: "Dell",
		ProductName:  "PowerEdge R740",
		SerialNumber: "ABC123",
	}
	data, err := json.Marshal(fru)
	if err != nil {
		t.Fatal(err)
	}

	var decoded FRUInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Manufacturer != "Dell" {
		t.Errorf("Manufacturer = %s", decoded.Manufacturer)
	}
}
