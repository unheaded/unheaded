// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ipmi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RedfishClient provides Redfish API access for out-of-band management.
// Redfish is the modern replacement for IPMI, providing a RESTful interface.
type RedfishClient struct {
	baseURL     string
	credentials *Credentials
	httpClient  *http.Client
	sessionID   string
	authToken   string
}

// NewRedfishClient creates a new Redfish client.
func NewRedfishClient(address string, credentials *Credentials) (*RedfishClient, error) {
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}

	if credentials == nil {
		return nil, fmt.Errorf("credentials are required")
	}

	// Ensure HTTPS scheme
	if !strings.HasPrefix(address, "https://") && !strings.HasPrefix(address, "http://") {
		address = "https://" + address
	}

	client := &RedfishClient{
		baseURL:     strings.TrimSuffix(address, "/"),
		credentials: credentials,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Create session
	if err := client.createSession(); err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	return client, nil
}

// createSession creates a Redfish session.
func (c *RedfishClient) createSession() error {
	// In production, POST to /redfish/v1/SessionService/Sessions
	// with username/password to get session token
	c.sessionID = "session-1"
	c.authToken = "auth-token"
	return nil
}

// Close closes the Redfish session.
func (c *RedfishClient) Close() error {
	if c.sessionID != "" {
		// In production, DELETE the session
		c.sessionID = ""
		c.authToken = ""
	}
	return nil
}

// doRequest performs an authenticated Redfish request.
func (c *RedfishClient) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("X-Auth-Token", c.authToken)
	}

	return c.httpClient.Do(req)
}

// GetServiceRoot returns the Redfish service root.
func (c *RedfishClient) GetServiceRoot(ctx context.Context) (*ServiceRoot, error) {
	resp, err := c.doRequest(ctx, "GET", "/redfish/v1", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var root ServiceRoot
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		return nil, err
	}

	return &root, nil
}

// ServiceRoot represents the Redfish service root.
type ServiceRoot struct {
	ID             string `json:"Id"`
	Name           string `json:"Name"`
	RedfishVersion string `json:"RedfishVersion"`
	UUID           string `json:"UUID"`
	Systems        Link   `json:"Systems"`
	Chassis        Link   `json:"Chassis"`
	Managers       Link   `json:"Managers"`
}

// Link represents a Redfish link.
type Link struct {
	OdataID string `json:"@odata.id"`
}

// GetSystems returns the computer systems collection.
func (c *RedfishClient) GetSystems(ctx context.Context) ([]ComputerSystem, error) {
	// In production, GET /redfish/v1/Systems
	return []ComputerSystem{
		{
			ID:           "1",
			Name:         "System",
			SerialNumber: "ABC123",
			PowerState:   "On",
		},
	}, nil
}

// ComputerSystem represents a Redfish computer system.
type ComputerSystem struct {
	ID               string           `json:"Id"`
	Name             string           `json:"Name"`
	Manufacturer     string           `json:"Manufacturer"`
	Model            string           `json:"Model"`
	SerialNumber     string           `json:"SerialNumber"`
	SKU              string           `json:"SKU"`
	PartNumber       string           `json:"PartNumber"`
	UUID             string           `json:"UUID"`
	HostName         string           `json:"HostName"`
	PowerState       string           `json:"PowerState"`
	Status           Status           `json:"Status"`
	IndicatorLED     string           `json:"IndicatorLED"`
	Boot             Boot             `json:"Boot"`
	ProcessorSummary ProcessorSummary `json:"ProcessorSummary"`
	MemorySummary    MemorySummary    `json:"MemorySummary"`
}

// Status represents component status.
type Status struct {
	State  string `json:"State"`
	Health string `json:"Health"`
}

// Boot represents boot configuration.
type Boot struct {
	BootSourceOverrideEnabled string   `json:"BootSourceOverrideEnabled"`
	BootSourceOverrideTarget  string   `json:"BootSourceOverrideTarget"`
	BootSourceOverrideMode    string   `json:"BootSourceOverrideMode"`
	AllowableValues           []string `json:"BootSourceOverrideTarget@Redfish.AllowableValues"`
}

// ProcessorSummary summarizes processor information.
type ProcessorSummary struct {
	Count  int    `json:"Count"`
	Model  string `json:"Model"`
	Status Status `json:"Status"`
}

// MemorySummary summarizes memory information.
type MemorySummary struct {
	TotalSystemMemoryGiB float64 `json:"TotalSystemMemoryGiB"`
	Status               Status  `json:"Status"`
}

// PowerOn powers on the system.
func (c *RedfishClient) PowerOn(ctx context.Context, systemID string) error {
	return c.resetSystem(ctx, systemID, "On")
}

// PowerOff powers off the system (force).
func (c *RedfishClient) PowerOff(ctx context.Context, systemID string) error {
	return c.resetSystem(ctx, systemID, "ForceOff")
}

// GracefulShutdown performs a graceful shutdown.
func (c *RedfishClient) GracefulShutdown(ctx context.Context, systemID string) error {
	return c.resetSystem(ctx, systemID, "GracefulShutdown")
}

// ForceRestart performs a force restart.
func (c *RedfishClient) ForceRestart(ctx context.Context, systemID string) error {
	return c.resetSystem(ctx, systemID, "ForceRestart")
}

// GracefulRestart performs a graceful restart.
func (c *RedfishClient) GracefulRestart(ctx context.Context, systemID string) error {
	return c.resetSystem(ctx, systemID, "GracefulRestart")
}

// PowerCycle performs a power cycle.
func (c *RedfishClient) PowerCycle(ctx context.Context, systemID string) error {
	return c.resetSystem(ctx, systemID, "PowerCycle")
}

// resetSystem sends a reset action to the system.
func (c *RedfishClient) resetSystem(ctx context.Context, systemID string, resetType string) error {
	path := fmt.Sprintf("/redfish/v1/Systems/%s/Actions/ComputerSystem.Reset", systemID)
	body := strings.NewReader(fmt.Sprintf(`{"ResetType": "%s"}`, resetType))

	resp, err := c.doRequest(ctx, "POST", path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("reset failed: %s", resp.Status)
	}

	return nil
}

// SetBootSource sets the boot source override.
func (c *RedfishClient) SetBootSource(ctx context.Context, systemID string, source string, enabled string) error {
	path := fmt.Sprintf("/redfish/v1/Systems/%s", systemID)
	body := strings.NewReader(fmt.Sprintf(`{
		"Boot": {
			"BootSourceOverrideTarget": "%s",
			"BootSourceOverrideEnabled": "%s"
		}
	}`, source, enabled))

	resp, err := c.doRequest(ctx, "PATCH", path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("set boot source failed: %s", resp.Status)
	}

	return nil
}

// SetBootPXE sets the system to boot from PXE once.
func (c *RedfishClient) SetBootPXE(ctx context.Context, systemID string) error {
	return c.SetBootSource(ctx, systemID, "Pxe", "Once")
}

// GetChassis returns chassis information.
func (c *RedfishClient) GetChassis(ctx context.Context) ([]Chassis, error) {
	// In production, GET /redfish/v1/Chassis
	return []Chassis{
		{
			ID:          "1",
			Name:        "System Chassis",
			ChassisType: "RackMount",
		},
	}, nil
}

// Chassis represents a Redfish chassis.
type Chassis struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	ChassisType  string `json:"ChassisType"`
	Manufacturer string `json:"Manufacturer"`
	Model        string `json:"Model"`
	SerialNumber string `json:"SerialNumber"`
	SKU          string `json:"SKU"`
	PartNumber   string `json:"PartNumber"`
	IndicatorLED string `json:"IndicatorLED"`
	Status       Status `json:"Status"`
	Power        Link   `json:"Power"`
	Thermal      Link   `json:"Thermal"`
}

// GetThermal returns thermal information for a chassis.
func (c *RedfishClient) GetThermal(ctx context.Context, chassisID string) (*Thermal, error) {
	// In production, GET /redfish/v1/Chassis/{chassisID}/Thermal
	return &Thermal{
		Temperatures: []Temperature{
			{Name: "CPU Temp", ReadingCelsius: 45.0, Status: Status{State: "Enabled", Health: "OK"}},
		},
		Fans: []Fan{
			{Name: "Fan1", Reading: 5400, ReadingUnits: "RPM", Status: Status{State: "Enabled", Health: "OK"}},
		},
	}, nil
}

// Thermal represents thermal information.
type Thermal struct {
	Temperatures []Temperature `json:"Temperatures"`
	Fans         []Fan         `json:"Fans"`
}

// Temperature represents a temperature sensor.
type Temperature struct {
	Name                      string  `json:"Name"`
	SensorNumber              int     `json:"SensorNumber"`
	ReadingCelsius            float64 `json:"ReadingCelsius"`
	UpperThresholdNonCritical float64 `json:"UpperThresholdNonCritical"`
	UpperThresholdCritical    float64 `json:"UpperThresholdCritical"`
	UpperThresholdFatal       float64 `json:"UpperThresholdFatal"`
	Status                    Status  `json:"Status"`
}

// Fan represents a fan sensor.
type Fan struct {
	Name         string  `json:"Name"`
	Reading      float64 `json:"Reading"`
	ReadingUnits string  `json:"ReadingUnits"`
	Status       Status  `json:"Status"`
}

// SetIndicatorLED sets the chassis indicator LED.
func (c *RedfishClient) SetIndicatorLED(ctx context.Context, chassisID string, state string) error {
	path := fmt.Sprintf("/redfish/v1/Chassis/%s", chassisID)
	body := strings.NewReader(fmt.Sprintf(`{"IndicatorLED": "%s"}`, state))

	resp, err := c.doRequest(ctx, "PATCH", path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("set indicator LED failed: %s", resp.Status)
	}

	return nil
}

// MountVirtualMedia mounts virtual media (ISO, etc).
func (c *RedfishClient) MountVirtualMedia(ctx context.Context, managerID, mediaType, imageURL string) error {
	path := fmt.Sprintf("/redfish/v1/Managers/%s/VirtualMedia/%s/Actions/VirtualMedia.InsertMedia", managerID, mediaType)
	body := strings.NewReader(fmt.Sprintf(`{"Image": "%s", "Inserted": true}`, imageURL))

	resp, err := c.doRequest(ctx, "POST", path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("mount virtual media failed: %s", resp.Status)
	}

	return nil
}

// EjectVirtualMedia ejects virtual media.
func (c *RedfishClient) EjectVirtualMedia(ctx context.Context, managerID, mediaType string) error {
	path := fmt.Sprintf("/redfish/v1/Managers/%s/VirtualMedia/%s/Actions/VirtualMedia.EjectMedia", managerID, mediaType)

	resp, err := c.doRequest(ctx, "POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("eject virtual media failed: %s", resp.Status)
	}

	return nil
}
