package logagg

import (
	"context"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"unheaded/pkg/transport"
)

// mockConnection implements transport.Connection for testing.
type mockConnection struct {
	mu        sync.Mutex
	published []publishedMsg
	healthy   bool
}

type publishedMsg struct {
	topic string
	data  []byte
}

func (m *mockConnection) Type() transport.Type { return "mock" }

func (m *mockConnection) Publish(_ context.Context, topic string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, publishedMsg{topic: topic, data: data})
	return nil
}

func (m *mockConnection) Subscribe(_ context.Context, _ string) (<-chan []byte, error) {
	return nil, nil
}

func (m *mockConnection) Close() error { return nil }

func (m *mockConnection) Healthy() bool { return m.healthy }

func (m *mockConnection) getPublished() []publishedMsg {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]publishedMsg, len(m.published))
	copy(result, m.published)
	return result
}

func TestPublisher_NilConnection(t *testing.T) {
	p := NewPublisher("test", nil)
	if p.Enabled() {
		t.Error("publisher with nil conn should be disabled")
	}
	// Should not panic
	p.Run(nil, zerolog.InfoLevel, "hello")
}

func TestPublisher_Enabled(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := NewPublisher("test", conn)
	if !p.Enabled() {
		t.Error("publisher with conn should be enabled")
	}
}

func TestPublisher_SetEnabled(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := NewPublisher("test", conn)

	p.SetEnabled(false)
	if p.Enabled() {
		t.Error("SetEnabled(false) should disable")
	}

	p.SetEnabled(true)
	if !p.Enabled() {
		t.Error("SetEnabled(true) should enable")
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level zerolog.Level
		want  string
	}{
		{zerolog.DebugLevel, "debug"},
		{zerolog.InfoLevel, "info"},
		{zerolog.WarnLevel, "warn"},
		{zerolog.ErrorLevel, "error"},
		{zerolog.FatalLevel, "fatal"},
		{zerolog.PanicLevel, "panic"},
		{zerolog.TraceLevel, "trace"},
	}
	for _, tt := range tests {
		if got := levelString(tt.level); got != tt.want {
			t.Errorf("levelString(%v) = %q, want %q", tt.level, got, tt.want)
		}
	}
}
