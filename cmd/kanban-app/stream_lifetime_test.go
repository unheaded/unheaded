// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/wotan-client/mock"
)

// main() initialises the TaskManager under a 10s timeout context and cancels
// it as soon as Initialize returns. The message streams must survive that —
// they used to die with it, and the board never received a Wotan message.
func TestStreams_OutliveInitializeContext_StopOnClose(t *testing.T) {
	mockClient := mock.NewMockClient(mock.WithAutoApprove())
	tm, err := NewTaskManager(mockClient, func(string, interface{}) {}, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	var refetches atomic.Int32
	tm.timelineManager.SetRefetch(func() error { refetches.Add(1); return nil })

	initCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := tm.Initialize(initCtx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	cancel() // exactly what main() does

	// A notification after the init ctx is gone must still be delivered.
	// The mock drops a message injected before the stream goroutine has
	// registered its channel (real Wotan retains), so inject until one lands.
	deadline := time.Now().Add(2 * time.Second)
	for refetches.Load() == 0 && time.Now().Before(deadline) {
		mockClient.InjectMessage(TopicTimelineUpdates, `{"event":"timeline_reloaded","source":"timeguru"}`)
		time.Sleep(20 * time.Millisecond)
	}
	if refetches.Load() == 0 {
		t.Fatal("stream died with the Initialize context: no notification was ever processed")
	}

	// Close ends the streams: a message after Close must not be processed.
	if err := tm.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-tm.streamCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel the stream context")
	}
}
