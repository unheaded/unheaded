// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package anamnesis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSuricataReader_TailEVEJSON(t *testing.T) {
	tmpDir := t.TempDir()
	evePath := filepath.Join(tmpDir, "eve.json")

	// Create file with initial content
	if err := os.WriteFile(evePath, []byte(""), 0644); err != nil {
		t.Fatalf("create eve.json: %v", err)
	}

	pub := &mockPublisher{}
	reader := NewSuricataReader(evePath, pub)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- reader.Run(ctx)
	}()

	// Give it time to start tailing
	time.Sleep(100 * time.Millisecond)

	// Append a Monad alert line
	alertLine := `{"timestamp":"2026-01-01T00:00:00.000000+0000","event_type":"alert","src_ip":"10.10.10.1","dest_ip":"10.10.10.2","src_port":12345,"dest_port":80,"alert":{"signature_id":2900001,"signature":"MONAD Metric","severity":3,"category":"Monad Protocol"}}` + "\n"

	f, err := os.OpenFile(evePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	if _, err := f.WriteString(alertLine); err != nil {
		f.Close()
		t.Fatalf("write alert: %v", err)
	}
	f.Close()

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Cancel to stop
	cancel()

	err = <-errCh
	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		t.Errorf("Run error: %v", err)
	}
}

func TestSuricataReader_RunFileNotFound(t *testing.T) {
	pub := &mockPublisher{}
	reader := NewSuricataReader("/nonexistent/path/eve.json", pub)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := reader.Run(ctx)
	if err == nil {
		t.Error("expected error or context cancellation")
	}
}

func TestSuricataReader_DefaultPath(t *testing.T) {
	pub := &mockPublisher{}
	reader := NewSuricataReader("", pub)
	if reader.eveJSONPath != "/var/log/suricata/eve.json" {
		t.Errorf("expected default path, got %s", reader.eveJSONPath)
	}
}

func TestSuricataReader_TailNonAlertLine(t *testing.T) {
	tmpDir := t.TempDir()
	evePath := filepath.Join(tmpDir, "eve.json")

	// Create file
	if err := os.WriteFile(evePath, []byte(""), 0644); err != nil {
		t.Fatalf("create eve.json: %v", err)
	}

	pub := &mockPublisher{}
	reader := NewSuricataReader(evePath, pub)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go func() {
		reader.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Write a non-alert event
	nonAlertLine := `{"timestamp":"2026-01-01T00:00:00.000000+0000","event_type":"dns","src_ip":"10.0.0.1","dest_ip":"10.0.0.2"}` + "\n"
	f, _ := os.OpenFile(evePath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(nonAlertLine)
	f.Close()

	time.Sleep(500 * time.Millisecond)
	cancel()
}

// mockPublisher for tests — uses the same interface as the existing test mock.
type mockPublisher struct {
	published int
}

func (m *mockPublisher) Publish(_ context.Context, topic string, data []byte) error {
	m.published++
	return nil
}
