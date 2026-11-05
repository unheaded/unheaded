// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

// A listener that dies under Serve must end the run so the unit restarts the
// collector. Left alone, the process lives on with that address dead.
func TestServeOn_DeadListenerEndsTheRun(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveOn(ctx, &http.Server{ReadHeaderTimeout: time.Second},
		listeners{bound: []net.Listener{ln}, stop: func(err error) { stopped <- err }})

	time.Sleep(50 * time.Millisecond) // let Serve reach Accept
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-stopped:
		if err == nil {
			t.Fatal("the run ended with no reason")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a dead listener did not end the run")
	}
}
