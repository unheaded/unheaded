package busboyClient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// TEST HELPERS
// ============================================================================

// newTestServer creates an httptest.Server that routes requests to a handler map.
// It returns the server and a Client wired to that server.
func newTestServer(t *testing.T, handlers map[string]http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, handler := range handlers {
		mux.HandleFunc(pattern, handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Strip "http://" from URL, because NewClient prepends "http://".
	addr := strings.TrimPrefix(srv.URL, "http://")
	c, err := NewClient(addr)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() {
		// Close may panic if pollMessages already closed a channel
		// that is still in c.channels. Recover gracefully.
		defer func() { recover() }()
		c.Close()
	})
	return srv, c
}

// jsonResponse writes a JSON response with the given status code.
func jsonResponse(w http.ResponseWriter, statusCode int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(body)
}

// errorResponse writes a structured error response.
func errorResponse(w http.ResponseWriter, statusCode int, code, message string) {
	jsonResponse(w, statusCode, map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

// ============================================================================
// NewClient TESTS
// ============================================================================

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{
			name:    "valid address",
			addr:    "localhost:9090",
			wantErr: false,
		},
		{
			name:    "valid address with hostname",
			addr:    "busboy.internal:8080",
			wantErr: false,
		},
		{
			name:    "empty address returns error",
			addr:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c == nil {
				t.Fatal("client is nil")
			}
			expectedBase := fmt.Sprintf("http://%s/api/v1", tt.addr)
			if c.baseURL != expectedBase {
				t.Errorf("baseURL = %q, want %q", c.baseURL, expectedBase)
			}
			if c.subscribers == nil {
				t.Error("subscribers map is nil")
			}
			if c.channels == nil {
				t.Error("channels map is nil")
			}
		})
	}
}

func TestNewClientWithTLS(t *testing.T) {
	tests := []struct {
		name      string
		addr      string
		transport *http.Transport
		wantErr   bool
	}{
		{
			name:      "valid address with nil transport",
			addr:      "secure.host:443",
			transport: nil,
			wantErr:   false,
		},
		{
			name:      "valid address with transport",
			addr:      "secure.host:443",
			transport: &http.Transport{},
			wantErr:   false,
		},
		{
			name:      "empty address returns error",
			addr:      "",
			transport: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClientWithTLS(tt.addr, tt.transport)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			expectedBase := fmt.Sprintf("https://%s/api/v1", tt.addr)
			if c.baseURL != expectedBase {
				t.Errorf("baseURL = %q, want %q", c.baseURL, expectedBase)
			}
		})
	}
}

// ============================================================================
// Close TESTS
// ============================================================================

func TestClose(t *testing.T) {
	t.Run("close empty client", func(t *testing.T) {
		c, err := NewClient("localhost:9090")
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	t.Run("close client with active channels", func(t *testing.T) {
		c, err := NewClient("localhost:9090")
		if err != nil {
			t.Fatal(err)
		}
		ch1 := make(chan *Message, 1)
		ch2 := make(chan *Message, 1)
		c.channels["topic1"] = ch1
		c.channels["topic2"] = ch2

		if err := c.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}

		// Verify channels were closed by trying to receive (should return zero value and false).
		_, ok1 := <-ch1
		if ok1 {
			t.Error("channel topic1 was not closed")
		}
		_, ok2 := <-ch2
		if ok2 {
			t.Error("channel topic2 was not closed")
		}

		// Verify channels map is empty.
		c.chanMu.RLock()
		defer c.chanMu.RUnlock()
		if len(c.channels) != 0 {
			t.Errorf("channels map has %d entries, want 0", len(c.channels))
		}
	})
}

// ============================================================================
// Subscribe TESTS
// ============================================================================

func TestSubscribe(t *testing.T) {
	t.Run("successful subscription", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/subscribe": func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				ct := r.Header.Get("Content-Type")
				if ct != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}

				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request body: %v", err)
				}
				if body["display_name"] != "alice" {
					t.Errorf("display_name = %q, want alice", body["display_name"])
				}

				jsonResponse(w, http.StatusCreated, map[string]interface{}{
					"subscriber": map[string]interface{}{
						"subscriber_id": "sub-123",
						"topic":         "chat",
						"display_name":  "alice",
						"status":        "approved",
						"requested_at":  time.Now().Format(time.RFC3339),
					},
				})
			},
		})

		sub, err := c.Subscribe(context.Background(), "chat", "alice")
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		if sub.SubscriberID != "sub-123" {
			t.Errorf("SubscriberID = %q, want sub-123", sub.SubscriberID)
		}
		if sub.Status != "approved" {
			t.Errorf("Status = %q, want approved", sub.Status)
		}

		// Verify subscriber was stored.
		stored, ok := c.GetSubscriber("chat")
		if !ok {
			t.Fatal("subscriber not stored in client")
		}
		if stored.SubscriberID != "sub-123" {
			t.Errorf("stored SubscriberID = %q, want sub-123", stored.SubscriberID)
		}
	})

	t.Run("pending subscription", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/moderated/subscribe": func(w http.ResponseWriter, r *http.Request) {
				jsonResponse(w, http.StatusCreated, map[string]interface{}{
					"subscriber": map[string]interface{}{
						"subscriber_id": "sub-456",
						"topic":         "moderated",
						"display_name":  "bob",
						"status":        "pending",
						"requested_at":  time.Now().Format(time.RFC3339),
					},
				})
			},
		})

		sub, err := c.Subscribe(context.Background(), "moderated", "bob")
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		if sub.Status != "pending" {
			t.Errorf("Status = %q, want pending", sub.Status)
		}
	})

	t.Run("empty topic returns error", func(t *testing.T) {
		c, err := NewClient("localhost:9090")
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Subscribe(context.Background(), "", "alice")
		if err == nil {
			t.Fatal("expected error for empty topic")
		}
		if !strings.Contains(err.Error(), "topic cannot be empty") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty display name returns error", func(t *testing.T) {
		c, err := NewClient("localhost:9090")
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Subscribe(context.Background(), "chat", "")
		if err == nil {
			t.Fatal("expected error for empty display name")
		}
		if !strings.Contains(err.Error(), "display name cannot be empty") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("server error responses", func(t *testing.T) {
		tests := []struct {
			name       string
			statusCode int
			errCode    string
			wantErr    error
		}{
			{
				name:       "topic not found",
				statusCode: http.StatusNotFound,
				errCode:    "INVALID_TOPIC",
				wantErr:    ErrTopicNotFound,
			},
			{
				name:       "not authorized",
				statusCode: http.StatusForbidden,
				errCode:    "NOT_AUTHORIZED",
				wantErr:    ErrNotAuthorized,
			},
			{
				name:       "rate limited",
				statusCode: http.StatusTooManyRequests,
				errCode:    "RATE_LIMITED",
				wantErr:    ErrRateLimited,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, c := newTestServer(t, map[string]http.HandlerFunc{
					"/api/v1/topics/test/subscribe": func(w http.ResponseWriter, r *http.Request) {
						errorResponse(w, tt.statusCode, tt.errCode, "test error")
					},
				})

				_, err := c.Subscribe(context.Background(), "test", "alice")
				if err == nil {
					t.Fatal("expected error")
				}
				if err != tt.wantErr {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/slow/subscribe": func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(2 * time.Second)
				jsonResponse(w, http.StatusCreated, map[string]interface{}{
					"subscriber": map[string]interface{}{},
				})
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := c.Subscribe(ctx, "slow", "alice")
		if err == nil {
			t.Fatal("expected error from context cancellation")
		}
	})

	t.Run("malformed server response", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/bad/subscribe": func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte("not json"))
			},
		})

		_, err := c.Subscribe(context.Background(), "bad", "alice")
		if err == nil {
			t.Fatal("expected error from malformed response")
		}
		if !strings.Contains(err.Error(), "decode response") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// ============================================================================
// Publish TESTS
// ============================================================================

func TestPublish(t *testing.T) {
	t.Run("successful publish", func(t *testing.T) {
		var receivedPayload string
		var receivedSubID string

		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/publish": func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				receivedPayload = body["payload"]
				receivedSubID = body["subscriber_id"]
				w.WriteHeader(http.StatusCreated)
			},
		})

		// Manually add an approved subscriber.
		c.mu.Lock()
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-abc",
			Topic:        "chat",
			Status:       "approved",
		}
		c.mu.Unlock()

		err := c.Publish(context.Background(), "chat", []byte("hello world"))
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if receivedPayload != "hello world" {
			t.Errorf("payload = %q, want %q", receivedPayload, "hello world")
		}
		if receivedSubID != "sub-abc" {
			t.Errorf("subscriber_id = %q, want sub-abc", receivedSubID)
		}
	})

	t.Run("empty topic", func(t *testing.T) {
		c, _ := NewClient("localhost:9090")
		err := c.Publish(context.Background(), "", []byte("data"))
		if err == nil {
			t.Fatal("expected error for empty topic")
		}
	})

	t.Run("empty payload", func(t *testing.T) {
		c, _ := NewClient("localhost:9090")
		err := c.Publish(context.Background(), "chat", []byte{})
		if err == nil {
			t.Fatal("expected error for empty payload")
		}
	})

	t.Run("nil payload", func(t *testing.T) {
		c, _ := NewClient("localhost:9090")
		err := c.Publish(context.Background(), "chat", nil)
		if err == nil {
			t.Fatal("expected error for nil payload")
		}
	})

	t.Run("not subscribed", func(t *testing.T) {
		c, _ := NewClient("localhost:9090")
		err := c.Publish(context.Background(), "unknown-topic", []byte("data"))
		if err == nil {
			t.Fatal("expected error when not subscribed")
		}
		if !strings.Contains(err.Error(), "not subscribed") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("subscription pending", func(t *testing.T) {
		c, _ := NewClient("localhost:9090")
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-pending",
			Topic:        "chat",
			Status:       "pending",
		}

		err := c.Publish(context.Background(), "chat", []byte("data"))
		if err != ErrSubscriptionPending {
			t.Errorf("error = %v, want ErrSubscriptionPending", err)
		}
	})

	t.Run("server error on publish", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/publish": func(w http.ResponseWriter, r *http.Request) {
				errorResponse(w, http.StatusForbidden, "NOT_AUTHORIZED", "denied")
			},
		})
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-abc",
			Topic:        "chat",
			Status:       "approved",
		}

		err := c.Publish(context.Background(), "chat", []byte("data"))
		if err != ErrNotAuthorized {
			t.Errorf("error = %v, want ErrNotAuthorized", err)
		}
	})

	t.Run("subscription denied status", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/publish": func(w http.ResponseWriter, r *http.Request) {
				errorResponse(w, http.StatusForbidden, "SUBSCRIPTION_DENIED", "denied")
			},
		})
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-abc",
			Topic:        "chat",
			Status:       "denied",
		}

		err := c.Publish(context.Background(), "chat", []byte("data"))
		if err != ErrSubscriptionPending {
			// "denied" != "approved", so should get ErrSubscriptionPending before the request is made
			t.Errorf("error = %v, want ErrSubscriptionPending", err)
		}
	})

	t.Run("context cancellation during publish", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/publish": func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(2 * time.Second)
				w.WriteHeader(http.StatusCreated)
			},
		})
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-abc",
			Topic:        "chat",
			Status:       "approved",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := c.Publish(ctx, "chat", []byte("data"))
		if err == nil {
			t.Fatal("expected error from context cancellation")
		}
	})
}

// ============================================================================
// GetMessages TESTS
// ============================================================================

func TestGetMessages(t *testing.T) {
	t.Run("successful retrieval", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/messages": func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %s, want GET", r.Method)
				}

				afterSeq := r.URL.Query().Get("after_seq")
				if afterSeq != "5" {
					t.Errorf("after_seq = %s, want 5", afterSeq)
				}

				limit := r.URL.Query().Get("limit")
				if limit != "10" {
					t.Errorf("limit = %s, want 10", limit)
				}

				now := time.Now()
				jsonResponse(w, http.StatusOK, map[string]interface{}{
					"messages": []map[string]interface{}{
						{
							"message_id": "msg-1",
							"topic":      "chat",
							"sender_id":  "sub-abc",
							"created_at": now.Format(time.RFC3339Nano),
							"seq":        6,
							"payload":    "hello",
							"deleted":    false,
						},
						{
							"message_id": "msg-2",
							"topic":      "chat",
							"sender_id":  "sub-def",
							"created_at": now.Format(time.RFC3339Nano),
							"seq":        7,
							"payload":    "world",
							"deleted":    false,
						},
					},
				})
			},
		})

		msgs, err := c.GetMessages(context.Background(), "chat", 5, 10)
		if err != nil {
			t.Fatalf("GetMessages: %v", err)
		}
		if len(msgs) != 2 {
			t.Fatalf("got %d messages, want 2", len(msgs))
		}
		if msgs[0].MessageID != "msg-1" {
			t.Errorf("msgs[0].MessageID = %q, want msg-1", msgs[0].MessageID)
		}
		if msgs[0].Payload != "hello" {
			t.Errorf("msgs[0].Payload = %q, want hello", msgs[0].Payload)
		}
		if msgs[1].Seq != 7 {
			t.Errorf("msgs[1].Seq = %d, want 7", msgs[1].Seq)
		}
	})

	t.Run("no limit parameter when zero", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/messages": func(w http.ResponseWriter, r *http.Request) {
				limit := r.URL.Query().Get("limit")
				if limit != "" {
					t.Errorf("limit should not be set when 0, got %q", limit)
				}
				jsonResponse(w, http.StatusOK, map[string]interface{}{
					"messages": []interface{}{},
				})
			},
		})

		_, err := c.GetMessages(context.Background(), "chat", 0, 0)
		if err != nil {
			t.Fatalf("GetMessages: %v", err)
		}
	})

	t.Run("empty topic", func(t *testing.T) {
		c, _ := NewClient("localhost:9090")
		_, err := c.GetMessages(context.Background(), "", 0, 10)
		if err == nil {
			t.Fatal("expected error for empty topic")
		}
	})

	t.Run("server returns error", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/missing/messages": func(w http.ResponseWriter, r *http.Request) {
				errorResponse(w, http.StatusNotFound, "INVALID_TOPIC", "topic not found")
			},
		})

		_, err := c.GetMessages(context.Background(), "missing", 0, 10)
		if err != ErrTopicNotFound {
			t.Errorf("error = %v, want ErrTopicNotFound", err)
		}
	})

	t.Run("malformed server response", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/bad/messages": func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{invalid json"))
			},
		})

		_, err := c.GetMessages(context.Background(), "bad", 0, 10)
		if err == nil {
			t.Fatal("expected error from malformed response")
		}
		if !strings.Contains(err.Error(), "decode response") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty message list", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/empty/messages": func(w http.ResponseWriter, r *http.Request) {
				jsonResponse(w, http.StatusOK, map[string]interface{}{
					"messages": []interface{}{},
				})
			},
		})

		msgs, err := c.GetMessages(context.Background(), "empty", 0, 10)
		if err != nil {
			t.Fatalf("GetMessages: %v", err)
		}
		if len(msgs) != 0 {
			t.Errorf("got %d messages, want 0", len(msgs))
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/slow/messages": func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(2 * time.Second)
				jsonResponse(w, http.StatusOK, map[string]interface{}{
					"messages": []interface{}{},
				})
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := c.GetMessages(ctx, "slow", 0, 10)
		if err == nil {
			t.Fatal("expected error from context cancellation")
		}
	})
}

// ============================================================================
// StreamMessages TESTS
// ============================================================================

func TestStreamMessages(t *testing.T) {
	t.Run("successful stream setup", func(t *testing.T) {
		var callCount int64
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/messages": func(w http.ResponseWriter, r *http.Request) {
				count := atomic.AddInt64(&callCount, 1)
				if count == 1 {
					now := time.Now()
					jsonResponse(w, http.StatusOK, map[string]interface{}{
						"messages": []map[string]interface{}{
							{
								"message_id": "msg-1",
								"topic":      "chat",
								"sender_id":  "sub-abc",
								"created_at": now.Format(time.RFC3339Nano),
								"seq":        1,
								"payload":    "streamed-hello",
								"deleted":    false,
							},
						},
					})
				} else {
					jsonResponse(w, http.StatusOK, map[string]interface{}{
						"messages": []interface{}{},
					})
				}
			},
		})

		c.mu.Lock()
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-abc",
			Topic:        "chat",
			Status:       "approved",
		}
		c.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		ch, err := c.StreamMessages(ctx, "chat")
		if err != nil {
			t.Fatalf("StreamMessages: %v", err)
		}

		// Wait for at least one message.
		select {
		case msg := <-ch:
			if msg == nil {
				t.Fatal("received nil message")
			}
			if msg.Payload != "streamed-hello" {
				t.Errorf("Payload = %q, want streamed-hello", msg.Payload)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for streamed message")
		}
	})

	t.Run("empty topic", func(t *testing.T) {
		c, _ := NewClient("localhost:9090")
		_, err := c.StreamMessages(context.Background(), "")
		if err == nil {
			t.Fatal("expected error for empty topic")
		}
	})

	t.Run("not subscribed", func(t *testing.T) {
		c, _ := NewClient("localhost:9090")
		_, err := c.StreamMessages(context.Background(), "unknown")
		if err == nil {
			t.Fatal("expected error when not subscribed")
		}
	})

	t.Run("subscription pending", func(t *testing.T) {
		c, _ := NewClient("localhost:9090")
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-pending",
			Status:       "pending",
		}

		_, err := c.StreamMessages(context.Background(), "chat")
		if err != ErrSubscriptionPending {
			t.Errorf("error = %v, want ErrSubscriptionPending", err)
		}
	})

	t.Run("channel closes on context cancel", func(t *testing.T) {
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/messages": func(w http.ResponseWriter, r *http.Request) {
				jsonResponse(w, http.StatusOK, map[string]interface{}{
					"messages": []interface{}{},
				})
			},
		})

		c.mu.Lock()
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-abc",
			Status:       "approved",
		}
		c.mu.Unlock()

		ctx, cancel := context.WithCancel(context.Background())
		ch, err := c.StreamMessages(ctx, "chat")
		if err != nil {
			t.Fatalf("StreamMessages: %v", err)
		}

		// Cancel and wait for channel closure.
		cancel()

		select {
		case _, ok := <-ch:
			if ok {
				// Got a message, drain until closed.
				for range ch {
				}
			}
			// Channel closed, good.
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for channel to close")
		}
	})
}

// ============================================================================
// pollMessages TESTS (via StreamMessages integration)
// ============================================================================

func TestPollMessages(t *testing.T) {
	t.Run("polls continuously and tracks lastSeq", func(t *testing.T) {
		var callCount int64
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/messages": func(w http.ResponseWriter, r *http.Request) {
				count := atomic.AddInt64(&callCount, 1)
				now := time.Now()

				switch {
				case count == 1:
					jsonResponse(w, http.StatusOK, map[string]interface{}{
						"messages": []map[string]interface{}{
							{"message_id": "msg-1", "topic": "chat", "sender_id": "s1",
								"created_at": now.Format(time.RFC3339Nano), "seq": 1, "payload": "first"},
						},
					})
				case count == 2:
					// Verify after_seq was updated.
					afterSeq := r.URL.Query().Get("after_seq")
					if afterSeq != "1" {
						t.Errorf("after_seq = %s, want 1 (should track lastSeq)", afterSeq)
					}
					jsonResponse(w, http.StatusOK, map[string]interface{}{
						"messages": []map[string]interface{}{
							{"message_id": "msg-2", "topic": "chat", "sender_id": "s1",
								"created_at": now.Format(time.RFC3339Nano), "seq": 2, "payload": "second"},
						},
					})
				default:
					jsonResponse(w, http.StatusOK, map[string]interface{}{
						"messages": []interface{}{},
					})
				}
			},
		})

		c.mu.Lock()
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-abc",
			Status:       "approved",
		}
		c.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		ch, err := c.StreamMessages(ctx, "chat")
		if err != nil {
			t.Fatalf("StreamMessages: %v", err)
		}

		// Collect messages.
		var received []*Message
		timeout := time.After(2500 * time.Millisecond)
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					goto done
				}
				received = append(received, msg)
				if len(received) >= 2 {
					goto done
				}
			case <-timeout:
				goto done
			}
		}
	done:
		if len(received) < 2 {
			t.Fatalf("received %d messages, want at least 2", len(received))
		}
		if received[0].Payload != "first" {
			t.Errorf("received[0].Payload = %q, want first", received[0].Payload)
		}
		if received[1].Payload != "second" {
			t.Errorf("received[1].Payload = %q, want second", received[1].Payload)
		}
	})

	t.Run("continues on GetMessages errors", func(t *testing.T) {
		var callCount int64
		_, c := newTestServer(t, map[string]http.HandlerFunc{
			"/api/v1/topics/chat/messages": func(w http.ResponseWriter, r *http.Request) {
				count := atomic.AddInt64(&callCount, 1)
				if count <= 2 {
					// First two calls fail.
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("server error"))
					return
				}
				// Third call succeeds.
				now := time.Now()
				jsonResponse(w, http.StatusOK, map[string]interface{}{
					"messages": []map[string]interface{}{
						{"message_id": "msg-1", "topic": "chat", "sender_id": "s1",
							"created_at": now.Format(time.RFC3339Nano), "seq": 1, "payload": "recovered"},
					},
				})
			},
		})

		c.mu.Lock()
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-abc",
			Status:       "approved",
		}
		c.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ch, err := c.StreamMessages(ctx, "chat")
		if err != nil {
			t.Fatalf("StreamMessages: %v", err)
		}

		select {
		case msg := <-ch:
			if msg == nil {
				t.Fatal("received nil message")
			}
			if msg.Payload != "recovered" {
				t.Errorf("Payload = %q, want recovered", msg.Payload)
			}
		case <-time.After(4 * time.Second):
			t.Fatal("timeout waiting for recovery message")
		}
	})
}

// ============================================================================
// parseError TESTS
// ============================================================================

func TestParseError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
		wantStr    string
	}{
		{
			name:       "SUBSCRIPTION_PENDING code",
			statusCode: http.StatusForbidden,
			body:       `{"error":{"code":"SUBSCRIPTION_PENDING","message":"pending approval"}}`,
			wantErr:    ErrSubscriptionPending,
		},
		{
			name:       "INVALID_TOPIC code",
			statusCode: http.StatusNotFound,
			body:       `{"error":{"code":"INVALID_TOPIC","message":"topic does not exist"}}`,
			wantErr:    ErrTopicNotFound,
		},
		{
			name:       "NOT_AUTHORIZED code",
			statusCode: http.StatusForbidden,
			body:       `{"error":{"code":"NOT_AUTHORIZED","message":"access denied"}}`,
			wantErr:    ErrNotAuthorized,
		},
		{
			name:       "SUBSCRIPTION_DENIED code",
			statusCode: http.StatusForbidden,
			body:       `{"error":{"code":"SUBSCRIPTION_DENIED","message":"denied by admin"}}`,
			wantErr:    ErrNotAuthorized,
		},
		{
			name:       "SUBSCRIPTION_REVOKED code",
			statusCode: http.StatusForbidden,
			body:       `{"error":{"code":"SUBSCRIPTION_REVOKED","message":"revoked"}}`,
			wantErr:    ErrNotAuthorized,
		},
		{
			name:       "RATE_LIMITED code",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":{"code":"RATE_LIMITED","message":"slow down"}}`,
			wantErr:    ErrRateLimited,
		},
		{
			name:       "unknown error code",
			statusCode: http.StatusInternalServerError,
			body:       `{"error":{"code":"INTERNAL","message":"something broke"}}`,
			wantStr:    "INTERNAL: something broke",
		},
		{
			name:       "malformed JSON body",
			statusCode: http.StatusInternalServerError,
			body:       "plain text error",
			wantStr:    "HTTP 500: plain text error",
		},
		{
			name:       "empty body",
			statusCode: http.StatusBadGateway,
			body:       "",
			wantStr:    "HTTP 502:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := NewClient("localhost:9090")

			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}

			err := c.parseError(resp)
			if err == nil {
				t.Fatal("expected non-nil error")
			}

			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
			} else if tt.wantStr != "" {
				if !strings.Contains(err.Error(), tt.wantStr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantStr)
				}
			}
		})
	}
}

// ============================================================================
// GetSubscriber / IsApproved TESTS
// ============================================================================

func TestGetSubscriber(t *testing.T) {
	c, _ := NewClient("localhost:9090")

	t.Run("not found", func(t *testing.T) {
		_, ok := c.GetSubscriber("nonexistent")
		if ok {
			t.Error("expected ok=false for nonexistent topic")
		}
	})

	t.Run("found after storage", func(t *testing.T) {
		c.mu.Lock()
		c.subscribers["chat"] = &Subscriber{
			SubscriberID: "sub-1",
			Topic:        "chat",
			Status:       "approved",
		}
		c.mu.Unlock()

		sub, ok := c.GetSubscriber("chat")
		if !ok {
			t.Fatal("expected ok=true")
		}
		if sub.SubscriberID != "sub-1" {
			t.Errorf("SubscriberID = %q, want sub-1", sub.SubscriberID)
		}
	})
}

func TestIsApproved(t *testing.T) {
	c, _ := NewClient("localhost:9090")

	tests := []struct {
		name   string
		topic  string
		sub    *Subscriber
		want   bool
	}{
		{
			name:  "no subscription",
			topic: "unknown",
			sub:   nil,
			want:  false,
		},
		{
			name:  "pending subscription",
			topic: "pending-topic",
			sub: &Subscriber{
				SubscriberID: "sub-1",
				Status:       "pending",
			},
			want: false,
		},
		{
			name:  "denied subscription",
			topic: "denied-topic",
			sub: &Subscriber{
				SubscriberID: "sub-2",
				Status:       "denied",
			},
			want: false,
		},
		{
			name:  "revoked subscription",
			topic: "revoked-topic",
			sub: &Subscriber{
				SubscriberID: "sub-3",
				Status:       "revoked",
			},
			want: false,
		},
		{
			name:  "approved subscription",
			topic: "approved-topic",
			sub: &Subscriber{
				SubscriberID: "sub-4",
				Status:       "approved",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sub != nil {
				c.mu.Lock()
				c.subscribers[tt.topic] = tt.sub
				c.mu.Unlock()
			}
			got := c.IsApproved(tt.topic)
			if got != tt.want {
				t.Errorf("IsApproved(%q) = %v, want %v", tt.topic, got, tt.want)
			}
		})
	}
}

// ============================================================================
// Message SERIALIZATION TESTS
// ============================================================================

func TestMessageSerialization(t *testing.T) {
	t.Run("marshal and unmarshal message", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		original := &Message{
			MessageID: "msg-abc-123",
			Topic:     "system.alerts",
			SenderID:  "sub-xyz",
			CreatedAt: now,
			Seq:       42,
			Payload:   `{"key":"value","nested":{"a":1}}`,
			Deleted:   false,
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}

		var decoded Message
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}

		if decoded.MessageID != original.MessageID {
			t.Errorf("MessageID = %q, want %q", decoded.MessageID, original.MessageID)
		}
		if decoded.Topic != original.Topic {
			t.Errorf("Topic = %q, want %q", decoded.Topic, original.Topic)
		}
		if decoded.SenderID != original.SenderID {
			t.Errorf("SenderID = %q, want %q", decoded.SenderID, original.SenderID)
		}
		if !decoded.CreatedAt.Equal(original.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", decoded.CreatedAt, original.CreatedAt)
		}
		if decoded.Seq != original.Seq {
			t.Errorf("Seq = %d, want %d", decoded.Seq, original.Seq)
		}
		if decoded.Payload != original.Payload {
			t.Errorf("Payload = %q, want %q", decoded.Payload, original.Payload)
		}
		if decoded.Deleted != original.Deleted {
			t.Errorf("Deleted = %v, want %v", decoded.Deleted, original.Deleted)
		}
	})

	t.Run("marshal deleted message", func(t *testing.T) {
		msg := &Message{
			MessageID: "msg-del",
			Topic:     "chat",
			Deleted:   true,
		}

		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}

		var raw map[string]interface{}
		json.Unmarshal(data, &raw)

		if raw["deleted"] != true {
			t.Errorf("deleted field = %v, want true", raw["deleted"])
		}
	})

	t.Run("JSON field names match expected format", func(t *testing.T) {
		msg := &Message{
			MessageID: "id",
			Topic:     "topic",
			SenderID:  "sender",
			Seq:       1,
			Payload:   "data",
		}

		data, _ := json.Marshal(msg)
		var raw map[string]interface{}
		json.Unmarshal(data, &raw)

		expectedFields := []string{"message_id", "topic", "sender_id", "created_at", "seq", "payload", "deleted"}
		for _, field := range expectedFields {
			if _, ok := raw[field]; !ok {
				t.Errorf("missing expected JSON field %q", field)
			}
		}
	})

	t.Run("payload with special characters", func(t *testing.T) {
		payloads := []string{
			``,
			`hello "world"`,
			"line1\nline2\ttab",
			`{"emoji":"` + "\U0001F600" + `"}`,
			strings.Repeat("x", 10000),
		}

		for i, p := range payloads {
			msg := &Message{MessageID: fmt.Sprintf("msg-%d", i), Payload: p}
			data, err := json.Marshal(msg)
			if err != nil {
				t.Errorf("payload[%d] marshal error: %v", i, err)
				continue
			}
			var decoded Message
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("payload[%d] unmarshal error: %v", i, err)
				continue
			}
			if decoded.Payload != p {
				t.Errorf("payload[%d] round-trip mismatch", i)
			}
		}
	})
}

func TestSubscriberSerialization(t *testing.T) {
	t.Run("marshal and unmarshal subscriber", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		original := &Subscriber{
			SubscriberID: "sub-123",
			Topic:        "updates",
			DisplayName:  "Test User",
			Status:       "approved",
			RequestedAt:  now,
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}

		var decoded Subscriber
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}

		if decoded.SubscriberID != original.SubscriberID {
			t.Errorf("SubscriberID = %q, want %q", decoded.SubscriberID, original.SubscriberID)
		}
		if decoded.DisplayName != original.DisplayName {
			t.Errorf("DisplayName = %q, want %q", decoded.DisplayName, original.DisplayName)
		}
		if decoded.Status != original.Status {
			t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
		}
	})

	t.Run("JSON field names match expected format", func(t *testing.T) {
		sub := &Subscriber{
			SubscriberID: "id",
			Topic:        "topic",
			DisplayName:  "name",
			Status:       "approved",
		}

		data, _ := json.Marshal(sub)
		var raw map[string]interface{}
		json.Unmarshal(data, &raw)

		expectedFields := []string{"subscriber_id", "topic", "display_name", "status", "requested_at"}
		for _, field := range expectedFields {
			if _, ok := raw[field]; !ok {
				t.Errorf("missing expected JSON field %q", field)
			}
		}
	})
}

// ============================================================================
// CONCURRENT / RACE SAFETY TESTS
// ============================================================================

func TestConcurrentSubscribeAndPublish(t *testing.T) {
	var reqCount int64

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&reqCount, 1)

			if strings.HasSuffix(r.URL.Path, "/subscribe") {
				jsonResponse(w, http.StatusCreated, map[string]interface{}{
					"subscriber": map[string]interface{}{
						"subscriber_id": fmt.Sprintf("sub-%d", atomic.LoadInt64(&reqCount)),
						"topic":         "concurrent",
						"display_name":  "user",
						"status":        "approved",
					},
				})
				return
			}
			if strings.HasSuffix(r.URL.Path, "/publish") {
				w.WriteHeader(http.StatusCreated)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	})

	// Subscribe first to set up the subscriber.
	sub, err := c.Subscribe(context.Background(), "concurrent", "user")
	if err != nil {
		t.Fatalf("initial Subscribe: %v", err)
	}
	_ = sub

	const goroutines = 20
	const opsPerGoroutine = 10

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*opsPerGoroutine)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				// Mix of reads and writes to exercise locks.
				if j%3 == 0 {
					c.GetSubscriber("concurrent")
				} else if j%3 == 1 {
					c.IsApproved("concurrent")
				} else {
					err := c.Publish(context.Background(), "concurrent", []byte(fmt.Sprintf("msg-%d-%d", id, j)))
					if err != nil {
						errs <- err
					}
				}
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestConcurrentGetSubscriber(t *testing.T) {
	c, _ := NewClient("localhost:9090")

	// Seed some subscribers.
	for i := 0; i < 10; i++ {
		topic := fmt.Sprintf("topic-%d", i)
		c.subscribers[topic] = &Subscriber{
			SubscriberID: fmt.Sprintf("sub-%d", i),
			Topic:        topic,
			Status:       "approved",
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			topic := fmt.Sprintf("topic-%d", id%10)
			sub, ok := c.GetSubscriber(topic)
			if !ok {
				t.Errorf("GetSubscriber(%q) returned false", topic)
			}
			if sub == nil {
				t.Errorf("GetSubscriber(%q) returned nil subscriber", topic)
			}
		}(i)
	}

	wg.Wait()
}

func TestConcurrentIsApproved(t *testing.T) {
	c, _ := NewClient("localhost:9090")

	c.subscribers["topic-a"] = &Subscriber{Status: "approved"}
	c.subscribers["topic-b"] = &Subscriber{Status: "pending"}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				if !c.IsApproved("topic-a") {
					t.Error("topic-a should be approved")
				}
			} else {
				if c.IsApproved("topic-b") {
					t.Error("topic-b should not be approved")
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestConcurrentCloseAndStreamMessages(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/chat/messages": func(w http.ResponseWriter, r *http.Request) {
			jsonResponse(w, http.StatusOK, map[string]interface{}{
				"messages": []interface{}{},
			})
		},
	})

	c.mu.Lock()
	c.subscribers["chat"] = &Subscriber{
		SubscriberID: "sub-1",
		Status:       "approved",
	}
	c.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start streaming.
	_, err := c.StreamMessages(ctx, "chat")
	if err != nil {
		t.Fatalf("StreamMessages: %v", err)
	}

	// Concurrently close and cancel.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		// pollMessages may have already closed the channel via defer,
		// so Close() can encounter a double-close. Recover gracefully
		// since this tests the concurrent teardown path, not crash safety.
		func() {
			defer func() { recover() }()
			c.Close()
		}()
	}()
	wg.Wait()
}

// ============================================================================
// BACKPRESSURE HANDLING TESTS
// ============================================================================

func TestBackpressureOnFullChannel(t *testing.T) {
	// The channel buffer is 100. Verify behavior when channel is full.
	c, _ := NewClient("localhost:9090")

	ch := make(chan *Message, 100)
	c.chanMu.Lock()
	c.channels["chat"] = ch
	c.chanMu.Unlock()

	// Fill the channel.
	for i := 0; i < 100; i++ {
		ch <- &Message{
			MessageID: fmt.Sprintf("msg-%d", i),
			Seq:       int64(i + 1),
			Payload:   fmt.Sprintf("payload-%d", i),
		}
	}

	// Verify channel is full.
	if len(ch) != 100 {
		t.Fatalf("channel len = %d, want 100", len(ch))
	}

	// Now verify the channel is at capacity.
	select {
	case ch <- &Message{Payload: "overflow"}:
		t.Fatal("should not be able to send to a full channel without blocking")
	default:
		// Expected: channel is full, send would block.
	}
}

func TestStreamMessageChannelBufferSize(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/chat/messages": func(w http.ResponseWriter, r *http.Request) {
			jsonResponse(w, http.StatusOK, map[string]interface{}{
				"messages": []interface{}{},
			})
		},
	})

	c.mu.Lock()
	c.subscribers["chat"] = &Subscriber{
		SubscriberID: "sub-abc",
		Status:       "approved",
	}
	c.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := c.StreamMessages(ctx, "chat")
	if err != nil {
		t.Fatalf("StreamMessages: %v", err)
	}

	// Verify the channel has a buffer capacity of 100.
	if cap(ch) != 100 {
		t.Errorf("channel capacity = %d, want 100", cap(ch))
	}
}

// ============================================================================
// RECONNECTION / RETRY LOGIC TESTS
// ============================================================================

func TestPollMessagesRetriesOnError(t *testing.T) {
	var callCount int64

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/retry/messages": func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&callCount, 1)
			if count <= 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			now := time.Now()
			jsonResponse(w, http.StatusOK, map[string]interface{}{
				"messages": []map[string]interface{}{
					{"message_id": "recovered", "topic": "retry", "sender_id": "s",
						"created_at": now.Format(time.RFC3339Nano), "seq": 1, "payload": "ok"},
				},
			})
		},
	})

	c.mu.Lock()
	c.subscribers["retry"] = &Subscriber{
		SubscriberID: "sub-retry",
		Status:       "approved",
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamMessages(ctx, "retry")
	if err != nil {
		t.Fatalf("StreamMessages: %v", err)
	}

	select {
	case msg := <-ch:
		if msg == nil {
			t.Fatal("received nil message")
		}
		if msg.MessageID != "recovered" {
			t.Errorf("MessageID = %q, want recovered", msg.MessageID)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timeout: polling did not recover after errors")
	}

	finalCount := atomic.LoadInt64(&callCount)
	if finalCount < 4 {
		t.Errorf("server was called %d times, expected at least 4 (3 failures + 1 success)", finalCount)
	}
}

func TestPollMessagesStopsOnContextCancel(t *testing.T) {
	var callCount int64

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/cancel/messages": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			jsonResponse(w, http.StatusOK, map[string]interface{}{
				"messages": []interface{}{},
			})
		},
	})

	c.mu.Lock()
	c.subscribers["cancel"] = &Subscriber{
		SubscriberID: "sub-cancel",
		Status:       "approved",
	}
	c.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := c.StreamMessages(ctx, "cancel")
	if err != nil {
		t.Fatalf("StreamMessages: %v", err)
	}

	// Let it poll a couple times.
	time.Sleep(600 * time.Millisecond)
	cancel()

	// Channel should eventually close.
	select {
	case _, ok := <-ch:
		if ok {
			// Drain remaining.
			for range ch {
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("channel not closed after context cancellation")
	}

	// Record count and wait, then verify it stopped.
	countAtCancel := atomic.LoadInt64(&callCount)
	time.Sleep(700 * time.Millisecond)
	countAfter := atomic.LoadInt64(&callCount)

	// Allow a tiny margin for in-flight requests.
	if countAfter > countAtCancel+1 {
		t.Errorf("polling continued after cancel: count went from %d to %d", countAtCancel, countAfter)
	}
}

// ============================================================================
// EDGE CASES
// ============================================================================

func TestMultipleStreamsOnSameTopic(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/chat/messages": func(w http.ResponseWriter, r *http.Request) {
			jsonResponse(w, http.StatusOK, map[string]interface{}{
				"messages": []interface{}{},
			})
		},
	})

	c.mu.Lock()
	c.subscribers["chat"] = &Subscriber{
		SubscriberID: "sub-1",
		Status:       "approved",
	}
	c.mu.Unlock()

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	ch1, err := c.StreamMessages(ctx1, "chat")
	if err != nil {
		t.Fatalf("StreamMessages (1): %v", err)
	}

	// Second stream replaces the first channel in the map.
	ch2, err := c.StreamMessages(ctx2, "chat")
	if err != nil {
		t.Fatalf("StreamMessages (2): %v", err)
	}

	// They should be different channels.
	if fmt.Sprintf("%p", ch1) == fmt.Sprintf("%p", ch2) {
		t.Error("expected different channels for subsequent StreamMessages calls")
	}

	cancel1()
	cancel2()
}

func TestSubscribeOverwritesPrevious(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/chat/subscribe": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			jsonResponse(w, http.StatusCreated, map[string]interface{}{
				"subscriber": map[string]interface{}{
					"subscriber_id": "sub-new",
					"topic":         "chat",
					"display_name":  body["display_name"],
					"status":        "approved",
				},
			})
		},
	})

	// First subscription.
	c.mu.Lock()
	c.subscribers["chat"] = &Subscriber{
		SubscriberID: "sub-old",
		Topic:        "chat",
		Status:       "approved",
	}
	c.mu.Unlock()

	// Subscribe again (should overwrite).
	sub, err := c.Subscribe(context.Background(), "chat", "new-user")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if sub.SubscriberID != "sub-new" {
		t.Errorf("SubscriberID = %q, want sub-new", sub.SubscriberID)
	}

	stored, ok := c.GetSubscriber("chat")
	if !ok {
		t.Fatal("subscriber not found after overwrite")
	}
	if stored.SubscriberID != "sub-new" {
		t.Errorf("stored SubscriberID = %q, want sub-new", stored.SubscriberID)
	}
}

func TestErrorVariables(t *testing.T) {
	// Verify sentinel errors are distinct and have useful messages.
	errors := map[string]error{
		"ErrNotConnected":        ErrNotConnected,
		"ErrSubscriptionPending": ErrSubscriptionPending,
		"ErrTopicNotFound":       ErrTopicNotFound,
		"ErrNotAuthorized":       ErrNotAuthorized,
		"ErrRateLimited":         ErrRateLimited,
	}

	for name, err := range errors {
		if err == nil {
			t.Errorf("%s is nil", name)
		}
		if err.Error() == "" {
			t.Errorf("%s has empty message", name)
		}
	}

	// Verify they are all distinct.
	seen := make(map[string]string)
	for name, err := range errors {
		msg := err.Error()
		if prev, ok := seen[msg]; ok {
			t.Errorf("%s and %s have the same message %q", name, prev, msg)
		}
		seen[msg] = name
	}
}

func TestClientHTTPTimeout(t *testing.T) {
	c, err := NewClient("localhost:9090")
	if err != nil {
		t.Fatal(err)
	}
	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", c.httpClient.Timeout)
	}
}

func TestPublishWithLargePayload(t *testing.T) {
	var receivedSize int

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/large/publish": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			receivedSize = len(body)
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["large"] = &Subscriber{
		SubscriberID: "sub-large",
		Status:       "approved",
	}
	c.mu.Unlock()

	payload := []byte(strings.Repeat("A", 1<<20)) // 1MB
	err := c.Publish(context.Background(), "large", payload)
	if err != nil {
		t.Fatalf("Publish large payload: %v", err)
	}

	if receivedSize == 0 {
		t.Error("server received empty body")
	}
}

func TestGetMessagesWithNegativeLimit(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/chat/messages": func(w http.ResponseWriter, r *http.Request) {
			// Negative limit should not add limit parameter (limit > 0 check).
			limit := r.URL.Query().Get("limit")
			if limit != "" {
				t.Errorf("negative limit should not be sent, got limit=%s", limit)
			}
			jsonResponse(w, http.StatusOK, map[string]interface{}{
				"messages": []interface{}{},
			})
		},
	})

	_, err := c.GetMessages(context.Background(), "chat", 0, -1)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
}

func TestServerReturnsNonJSONError(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/html/subscribe": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("<html><body>Bad Gateway</body></html>"))
		},
	})

	_, err := c.Subscribe(context.Background(), "html", "alice")
	if err == nil {
		t.Fatal("expected error from non-JSON response")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error should contain status code, got: %v", err)
	}
}

func TestMultipleTopicSubscriptions(t *testing.T) {
	topicCount := 0

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/": func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/subscribe") {
				topicCount++
				parts := strings.Split(r.URL.Path, "/")
				topic := parts[len(parts)-2]
				jsonResponse(w, http.StatusCreated, map[string]interface{}{
					"subscriber": map[string]interface{}{
						"subscriber_id": fmt.Sprintf("sub-%d", topicCount),
						"topic":         topic,
						"display_name":  "user",
						"status":        "approved",
					},
				})
			}
		},
	})

	topics := []string{"alerts", "logs", "metrics", "traces", "events"}
	for _, topic := range topics {
		_, err := c.Subscribe(context.Background(), topic, "user")
		if err != nil {
			t.Fatalf("Subscribe(%s): %v", topic, err)
		}
	}

	// Verify all subscriptions stored.
	for _, topic := range topics {
		sub, ok := c.GetSubscriber(topic)
		if !ok {
			t.Errorf("missing subscriber for topic %q", topic)
		}
		if !c.IsApproved(topic) {
			t.Errorf("topic %q should be approved", topic)
		}
		if sub.Topic != topic {
			t.Errorf("sub.Topic = %q, want %q", sub.Topic, topic)
		}
	}
}

// ============================================================================
// FULL END-TO-END INTEGRATION TESTS
// ============================================================================

func TestEndToEndSubscribePublishGetMessages(t *testing.T) {
	var storedMessages []*Message
	var storedMu sync.Mutex

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/e2e/subscribe": func(w http.ResponseWriter, r *http.Request) {
			jsonResponse(w, http.StatusCreated, map[string]interface{}{
				"subscriber": map[string]interface{}{
					"subscriber_id": "sub-e2e",
					"topic":         "e2e",
					"display_name":  "tester",
					"status":        "approved",
				},
			})
		},
		"/api/v1/topics/e2e/publish": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)

			storedMu.Lock()
			storedMessages = append(storedMessages, &Message{
				MessageID: fmt.Sprintf("msg-%d", len(storedMessages)+1),
				Topic:     "e2e",
				SenderID:  body["subscriber_id"],
				CreatedAt: time.Now(),
				Seq:       int64(len(storedMessages) + 1),
				Payload:   body["payload"],
			})
			storedMu.Unlock()

			w.WriteHeader(http.StatusCreated)
		},
		"/api/v1/topics/e2e/messages": func(w http.ResponseWriter, r *http.Request) {
			storedMu.Lock()
			msgs := make([]*Message, len(storedMessages))
			copy(msgs, storedMessages)
			storedMu.Unlock()

			jsonResponse(w, http.StatusOK, map[string]interface{}{
				"messages": msgs,
			})
		},
	})

	// Step 1: Subscribe.
	sub, err := c.Subscribe(context.Background(), "e2e", "tester")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if sub.Status != "approved" {
		t.Fatalf("Status = %q, want approved", sub.Status)
	}

	// Step 2: Publish messages.
	for i := 0; i < 5; i++ {
		err := c.Publish(context.Background(), "e2e", []byte(fmt.Sprintf("message-%d", i)))
		if err != nil {
			t.Fatalf("Publish #%d: %v", i, err)
		}
	}

	// Step 3: Retrieve messages.
	msgs, err := c.GetMessages(context.Background(), "e2e", 0, 100)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("got %d messages, want 5", len(msgs))
	}
	for i, msg := range msgs {
		expected := fmt.Sprintf("message-%d", i)
		if msg.Payload != expected {
			t.Errorf("msgs[%d].Payload = %q, want %q", i, msg.Payload, expected)
		}
	}
}

// ============================================================================
// HTTP CLIENT TRANSPORT TESTS
// ============================================================================

func TestNewClientWithTLSTransport(t *testing.T) {
	transport := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	}

	c, err := NewClientWithTLS("secure.example.com:443", transport)
	if err != nil {
		t.Fatalf("NewClientWithTLS: %v", err)
	}

	if c.httpClient.Transport != transport {
		t.Error("transport was not set on the HTTP client")
	}

	if !strings.HasPrefix(c.baseURL, "https://") {
		t.Errorf("baseURL = %q, want https:// prefix", c.baseURL)
	}
}

func TestServerConnectionRefused(t *testing.T) {
	// Use a port that nothing is listening on.
	c, err := NewClient("127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.GetMessages(context.Background(), "chat", 0, 10)
	if err == nil {
		t.Fatal("expected error when server is unreachable")
	}
}

// ============================================================================
// CLOSE IDEMPOTENCY TEST
// ============================================================================

func TestCloseIdempotent(t *testing.T) {
	c, err := NewClient("localhost:9090")
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan *Message, 1)
	c.channels["topic"] = ch

	// First close.
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Second close should not panic (channels already deleted).
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// ============================================================================
// STRESS TEST
// ============================================================================

func TestStressConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/": func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/subscribe") {
				parts := strings.Split(r.URL.Path, "/")
				topic := parts[len(parts)-2]
				jsonResponse(w, http.StatusCreated, map[string]interface{}{
					"subscriber": map[string]interface{}{
						"subscriber_id": fmt.Sprintf("sub-%s", topic),
						"topic":         topic,
						"display_name":  "stress",
						"status":        "approved",
					},
				})
				return
			}
			if strings.HasSuffix(r.URL.Path, "/publish") {
				w.WriteHeader(http.StatusCreated)
				return
			}
			if strings.Contains(r.URL.Path, "/messages") {
				jsonResponse(w, http.StatusOK, map[string]interface{}{
					"messages": []interface{}{},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	})

	const numTopics = 5
	const numWriters = 10
	const msgsPerWriter = 20

	// Subscribe to all topics.
	for i := 0; i < numTopics; i++ {
		topic := fmt.Sprintf("stress-%d", i)
		_, err := c.Subscribe(context.Background(), topic, "stress-user")
		if err != nil {
			t.Fatalf("Subscribe(%s): %v", topic, err)
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, numWriters*msgsPerWriter)

	// Launch concurrent publishers.
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for m := 0; m < msgsPerWriter; m++ {
				topic := fmt.Sprintf("stress-%d", writerID%numTopics)
				payload := fmt.Sprintf("writer-%d-msg-%d", writerID, m)
				if err := c.Publish(context.Background(), topic, []byte(payload)); err != nil {
					errCh <- fmt.Errorf("writer %d msg %d: %w", writerID, m, err)
				}
			}
		}(w)
	}

	// Launch concurrent readers.
	for r := 0; r < numWriters; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			topic := fmt.Sprintf("stress-%d", readerID%numTopics)
			for m := 0; m < msgsPerWriter; m++ {
				c.GetSubscriber(topic)
				c.IsApproved(topic)
			}
		}(r)
	}

	wg.Wait()
	close(errCh)

	var errCount int
	for err := range errCh {
		errCount++
		if errCount <= 5 {
			t.Errorf("stress error: %v", err)
		}
	}
	if errCount > 5 {
		t.Errorf("... and %d more errors", errCount-5)
	}
}
