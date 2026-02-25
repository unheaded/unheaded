// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"unheaded/pkg/transport"
)

const (
	// RegisterTopic is the Wotan topic for service registration.
	RegisterTopic = "system.discovery.register"

	// DeregisterTopic is the Wotan topic for service deregistration.
	DeregisterTopic = "system.discovery.deregister"
)

// RegistrationMessage is published when a service registers or deregisters.
type RegistrationMessage struct {
	Name      string `json:"name"`
	Port      int    `json:"port"`
	Proto     string `json:"proto"`
	Health    string `json:"health"`
	PID       int    `json:"pid"`
	StartedAt int64  `json:"started_at"`
}

// Registrar handles service registration and deregistration via the transport layer.
type Registrar struct {
	conn      transport.Connection
	service   ServiceEntry
	proto     string
	startedAt time.Time
}

// NewRegistrar creates a new registrar for the given service.
func NewRegistrar(conn transport.Connection, name string, port int, proto string) *Registrar {
	return &Registrar{
		conn: conn,
		service: ServiceEntry{
			Name:    name,
			Address: fmt.Sprintf("localhost:%d", port),
			Port:    port,
			Health:  "healthy",
		},
		proto:     proto,
		startedAt: time.Now(),
	}
}

// Register publishes a registration message to the discovery topic.
func (r *Registrar) Register(ctx context.Context) error {
	if r.conn == nil {
		return fmt.Errorf("registrar: transport connection is nil")
	}

	msg := RegistrationMessage{
		Name:      r.service.Name,
		Port:      r.service.Port,
		Proto:     r.proto,
		Health:    r.service.Health,
		PID:       os.Getpid(),
		StartedAt: r.startedAt.Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("registrar: marshal: %w", err)
	}

	if err := r.conn.Publish(ctx, RegisterTopic, data); err != nil {
		return fmt.Errorf("registrar: publish register: %w", err)
	}
	return nil
}

// Deregister publishes a deregistration message to the discovery topic.
func (r *Registrar) Deregister(ctx context.Context) error {
	if r.conn == nil {
		return fmt.Errorf("registrar: transport connection is nil")
	}

	msg := RegistrationMessage{
		Name:      r.service.Name,
		Port:      r.service.Port,
		Proto:     r.proto,
		Health:    "down",
		PID:       os.Getpid(),
		StartedAt: r.startedAt.Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("registrar: marshal: %w", err)
	}

	if err := r.conn.Publish(ctx, DeregisterTopic, data); err != nil {
		return fmt.Errorf("registrar: publish deregister: %w", err)
	}
	return nil
}

// ServiceName returns the name of the service this registrar manages.
func (r *Registrar) ServiceName() string {
	return r.service.Name
}
