package cloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ============================================================================
// TEST HELPERS
// ============================================================================

// newTestService creates a Service with a silent logger and default config
// for isolated, self-contained unit tests.
func newTestService() *Service {
	log := logger.New(io.Discard)
	return NewService(log, nil, nil)
}

// newTestServiceWithConfig creates a Service with a custom config.
func newTestServiceWithConfig(cfg *Config) *Service {
	log := logger.New(io.Discard)
	return NewService(log, nil, cfg)
}

// mustCreateDashboard is a test helper that creates a dashboard and fails the
// test immediately if creation returns an error.
func mustCreateDashboard(t *testing.T, svc *Service, d *Dashboard) {
	t.Helper()
	if err := svc.CreateDashboard(d); err != nil {
		t.Fatalf("mustCreateDashboard: %v", err)
	}
}

// mustCreateSession is a test helper that creates a session and fails the
// test immediately if creation returns an error.
func mustCreateSession(t *testing.T, svc *Service, userID string) *Session {
	t.Helper()
	sess, err := svc.CreateSession(userID)
	if err != nil {
		t.Fatalf("mustCreateSession: %v", err)
	}
	return sess
}

// drainBroadcasts reads all pending broadcasts from the channel without blocking.
func drainBroadcasts(svc *Service) []*Broadcast {
	var result []*Broadcast
	for {
		select {
		case b := <-svc.broadcasts:
			result = append(result, b)
		default:
			return result
		}
	}
}

// ============================================================================
// DEFAULT CONFIG TESTS
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"SessionTimeout", cfg.SessionTimeout, 24 * time.Hour},
		{"MaxNotifications", cfg.MaxNotifications, 100},
		{"DefaultTheme", cfg.DefaultTheme, "dark"},
		{"DefaultLayout", cfg.DefaultLayout, LayoutGrid},
		{"TemplateDir", cfg.TemplateDir, "./templates"},
		{"StaticDir", cfg.StaticDir, "./static"},
		{"BroadcastBuffer", cfg.BroadcastBuffer, 1000},
		{"WotanTopic", cfg.WotanTopic, "cloak.dashboard"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if fmt.Sprintf("%v", tt.got) != fmt.Sprintf("%v", tt.want) {
				t.Errorf("DefaultConfig().%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

// ============================================================================
// NEW SERVICE TESTS
// ============================================================================

func TestNewService(t *testing.T) {
	t.Run("creates service with default config when nil", func(t *testing.T) {
		svc := newTestService()
		if svc == nil {
			t.Fatal("expected non-nil service")
		}
		if svc.config == nil {
			t.Error("expected config to be set")
		}
		if svc.dashboards == nil {
			t.Error("expected dashboards map to be initialized")
		}
		if svc.sessions == nil {
			t.Error("expected sessions map to be initialized")
		}
		if svc.notifications == nil {
			t.Error("expected notifications map to be initialized")
		}
		if svc.broadcasts == nil {
			t.Error("expected broadcasts channel to be initialized")
		}
		if svc.config.DefaultTheme != "dark" {
			t.Errorf("expected default theme 'dark', got %q", svc.config.DefaultTheme)
		}
	})

	t.Run("creates service with custom config", func(t *testing.T) {
		cfg := &Config{
			SessionTimeout:   1 * time.Hour,
			MaxNotifications: 10,
			DefaultTheme:     "light",
			DefaultLayout:    LayoutFreeform,
			BroadcastBuffer:  50,
			WotanTopic:      "custom.topic",
		}
		svc := newTestServiceWithConfig(cfg)

		if svc.config.SessionTimeout != 1*time.Hour {
			t.Errorf("expected SessionTimeout 1h, got %v", svc.config.SessionTimeout)
		}
		if svc.config.MaxNotifications != 10 {
			t.Errorf("expected MaxNotifications 10, got %d", svc.config.MaxNotifications)
		}
		if svc.config.DefaultTheme != "light" {
			t.Errorf("expected DefaultTheme 'light', got %q", svc.config.DefaultTheme)
		}
		if svc.config.DefaultLayout != LayoutFreeform {
			t.Errorf("expected DefaultLayout 'freeform', got %q", svc.config.DefaultLayout)
		}
	})

	t.Run("creates service with nil wotan client", func(t *testing.T) {
		log := logger.New(io.Discard)
		svc := NewService(log, nil, nil)
		if svc.wotan != nil {
			t.Error("expected nil wotan client")
		}
	})
}

// ============================================================================
// START / STOP TESTS
// ============================================================================

func TestStartStop(t *testing.T) {
	t.Run("start and stop without error", func(t *testing.T) {
		svc := newTestService()
		ctx, cancel := context.WithCancel(context.Background())

		err := svc.Start(ctx)
		if err != nil {
			t.Fatalf("Start() returned error: %v", err)
		}

		cancel()

		err = svc.Stop()
		if err != nil {
			t.Fatalf("Stop() returned error: %v", err)
		}
	})
}

// ============================================================================
// DASHBOARD CRUD TESTS
// ============================================================================

func TestCreateDashboard(t *testing.T) {
	tests := []struct {
		name      string
		dashboard *Dashboard
		wantID    bool // if true, ID should be auto-generated
		wantTheme string
	}{
		{
			name: "auto-generates ID when empty",
			dashboard: &Dashboard{
				Name:    "Test Dashboard",
				OwnerID: "user-1",
			},
			wantID:    true,
			wantTheme: "dark",
		},
		{
			name: "preserves provided ID",
			dashboard: &Dashboard{
				ID:      "custom-id",
				Name:    "Custom Dashboard",
				OwnerID: "user-2",
			},
			wantID:    false,
			wantTheme: "dark",
		},
		{
			name: "applies default theme when empty",
			dashboard: &Dashboard{
				Name:    "Themed Dashboard",
				OwnerID: "user-3",
			},
			wantID:    true,
			wantTheme: "dark",
		},
		{
			name: "preserves custom theme",
			dashboard: &Dashboard{
				Name:    "Light Dashboard",
				OwnerID: "user-4",
				Theme:   "light",
			},
			wantID:    true,
			wantTheme: "light",
		},
		{
			name: "applies default layout when nil",
			dashboard: &Dashboard{
				Name:    "No Layout",
				OwnerID: "user-5",
			},
			wantID:    true,
			wantTheme: "dark",
		},
		{
			name: "preserves custom layout",
			dashboard: &Dashboard{
				Name:    "Custom Layout",
				OwnerID: "user-6",
				Layout: &Layout{
					Type:      LayoutFreeform,
					Columns:   6,
					RowHeight: 200,
					Margin:    20,
				},
			},
			wantID:    true,
			wantTheme: "dark",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			err := svc.CreateDashboard(tt.dashboard)
			if err != nil {
				t.Fatalf("CreateDashboard() error = %v", err)
			}

			if tt.wantID && tt.dashboard.ID == "" {
				t.Error("expected auto-generated ID, got empty")
			}
			if !tt.wantID && tt.dashboard.ID != "custom-id" {
				t.Errorf("expected ID 'custom-id', got %q", tt.dashboard.ID)
			}
			if tt.dashboard.Theme != tt.wantTheme {
				t.Errorf("expected theme %q, got %q", tt.wantTheme, tt.dashboard.Theme)
			}
			if tt.dashboard.CreatedAt.IsZero() {
				t.Error("expected CreatedAt to be set")
			}
			if tt.dashboard.UpdatedAt.IsZero() {
				t.Error("expected UpdatedAt to be set")
			}
			if tt.dashboard.Layout == nil {
				t.Error("expected Layout to be set")
			}

			// Verify it was stored
			got, ok := svc.GetDashboard(tt.dashboard.ID)
			if !ok {
				t.Fatal("expected dashboard to be retrievable")
			}
			if got.Name != tt.dashboard.Name {
				t.Errorf("expected Name %q, got %q", tt.dashboard.Name, got.Name)
			}
		})
	}
}

func TestCreateDashboard_DefaultLayout(t *testing.T) {
	svc := newTestService()
	d := &Dashboard{Name: "Test", OwnerID: "u1"}
	mustCreateDashboard(t, svc, d)

	if d.Layout.Type != LayoutGrid {
		t.Errorf("expected layout type %q, got %q", LayoutGrid, d.Layout.Type)
	}
	if d.Layout.Columns != 12 {
		t.Errorf("expected 12 columns, got %d", d.Layout.Columns)
	}
	if d.Layout.RowHeight != 100 {
		t.Errorf("expected row height 100, got %d", d.Layout.RowHeight)
	}
	if d.Layout.Margin != 10 {
		t.Errorf("expected margin 10, got %d", d.Layout.Margin)
	}
}

func TestGetDashboard(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*Service)
		id     string
		wantOK bool
	}{
		{
			name: "existing dashboard",
			setup: func(svc *Service) {
				mustCreateDashboard(t, svc, &Dashboard{ID: "dash-1", Name: "My Dash", OwnerID: "u1"})
			},
			id:     "dash-1",
			wantOK: true,
		},
		{
			name:   "non-existent dashboard",
			setup:  func(svc *Service) {},
			id:     "non-existent",
			wantOK: false,
		},
		{
			name:   "empty ID",
			setup:  func(svc *Service) {},
			id:     "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			tt.setup(svc)

			_, ok := svc.GetDashboard(tt.id)
			if ok != tt.wantOK {
				t.Errorf("GetDashboard(%q) ok = %v, want %v", tt.id, ok, tt.wantOK)
			}
		})
	}
}

func TestUpdateDashboard(t *testing.T) {
	t.Run("updates existing dashboard", func(t *testing.T) {
		svc := newTestService()
		d := &Dashboard{ID: "dash-1", Name: "Original", OwnerID: "u1"}
		mustCreateDashboard(t, svc, d)

		drainBroadcasts(svc) // clear any pending broadcasts

		updated := &Dashboard{
			ID:      "dash-1",
			Name:    "Updated",
			OwnerID: "u1",
		}
		err := svc.UpdateDashboard(updated)
		if err != nil {
			t.Fatalf("UpdateDashboard() error = %v", err)
		}

		got, ok := svc.GetDashboard("dash-1")
		if !ok {
			t.Fatal("expected dashboard to exist after update")
		}
		if got.Name != "Updated" {
			t.Errorf("expected name 'Updated', got %q", got.Name)
		}
		if got.UpdatedAt.IsZero() {
			t.Error("expected UpdatedAt to be set")
		}

		// Verify broadcast was sent
		broadcasts := drainBroadcasts(svc)
		if len(broadcasts) != 1 {
			t.Fatalf("expected 1 broadcast, got %d", len(broadcasts))
		}
		if broadcasts[0].Type != "dashboard.updated" {
			t.Errorf("expected broadcast type 'dashboard.updated', got %q", broadcasts[0].Type)
		}
		if broadcasts[0].Target != "dashboard:dash-1" {
			t.Errorf("expected broadcast target 'dashboard:dash-1', got %q", broadcasts[0].Target)
		}
	})

	t.Run("returns error for non-existent dashboard", func(t *testing.T) {
		svc := newTestService()
		err := svc.UpdateDashboard(&Dashboard{ID: "no-such-dash"})
		if err == nil {
			t.Error("expected error for non-existent dashboard")
		}
	})
}

func TestDeleteDashboard(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Service)
		id      string
		wantErr bool
	}{
		{
			name: "deletes existing dashboard",
			setup: func(svc *Service) {
				mustCreateDashboard(t, svc, &Dashboard{ID: "dash-del", Name: "To Delete", OwnerID: "u1"})
			},
			id:      "dash-del",
			wantErr: false,
		},
		{
			name:    "returns error for non-existent dashboard",
			setup:   func(svc *Service) {},
			id:      "non-existent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			tt.setup(svc)

			err := svc.DeleteDashboard(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteDashboard(%q) error = %v, wantErr = %v", tt.id, err, tt.wantErr)
			}

			if !tt.wantErr {
				_, ok := svc.GetDashboard(tt.id)
				if ok {
					t.Error("expected dashboard to be deleted")
				}
			}
		})
	}
}

func TestListDashboards(t *testing.T) {
	t.Run("returns all dashboards for empty ownerID", func(t *testing.T) {
		svc := newTestService()
		mustCreateDashboard(t, svc, &Dashboard{ID: "d1", Name: "One", OwnerID: "u1"})
		mustCreateDashboard(t, svc, &Dashboard{ID: "d2", Name: "Two", OwnerID: "u2"})
		mustCreateDashboard(t, svc, &Dashboard{ID: "d3", Name: "Three", OwnerID: "u1"})

		all := svc.ListDashboards("")
		if len(all) != 3 {
			t.Errorf("expected 3 dashboards, got %d", len(all))
		}
	})

	t.Run("filters dashboards by ownerID", func(t *testing.T) {
		svc := newTestService()
		mustCreateDashboard(t, svc, &Dashboard{ID: "d1", Name: "One", OwnerID: "u1"})
		mustCreateDashboard(t, svc, &Dashboard{ID: "d2", Name: "Two", OwnerID: "u2"})
		mustCreateDashboard(t, svc, &Dashboard{ID: "d3", Name: "Three", OwnerID: "u1"})

		u1Dashboards := svc.ListDashboards("u1")
		if len(u1Dashboards) != 2 {
			t.Errorf("expected 2 dashboards for u1, got %d", len(u1Dashboards))
		}
		for _, d := range u1Dashboards {
			if d.OwnerID != "u1" {
				t.Errorf("expected ownerID 'u1', got %q", d.OwnerID)
			}
		}
	})

	t.Run("returns empty slice for no dashboards", func(t *testing.T) {
		svc := newTestService()
		result := svc.ListDashboards("u1")
		if result != nil && len(result) != 0 {
			t.Errorf("expected empty result, got %d dashboards", len(result))
		}
	})

	t.Run("returns empty for non-matching ownerID", func(t *testing.T) {
		svc := newTestService()
		mustCreateDashboard(t, svc, &Dashboard{ID: "d1", Name: "One", OwnerID: "u1"})

		result := svc.ListDashboards("no-such-user")
		if result != nil && len(result) != 0 {
			t.Errorf("expected empty result, got %d dashboards", len(result))
		}
	})
}

// ============================================================================
// WIDGET TESTS
// ============================================================================

func TestAddWidget(t *testing.T) {
	tests := []struct {
		name        string
		dashboardID string
		widget      *Widget
		setupDash   bool
		wantErr     bool
	}{
		{
			name:        "adds widget to existing dashboard",
			dashboardID: "dash-w1",
			widget: &Widget{
				Type:   WidgetChart,
				Title:  "CPU Usage",
				Source: "metrics/cpu",
			},
			setupDash: true,
			wantErr:   false,
		},
		{
			name:        "adds widget with custom ID",
			dashboardID: "dash-w2",
			widget: &Widget{
				ID:     "my-widget",
				Type:   WidgetTable,
				Title:  "Requests",
				Source: "metrics/requests",
			},
			setupDash: true,
			wantErr:   false,
		},
		{
			name:        "returns error for non-existent dashboard",
			dashboardID: "no-such-dash",
			widget: &Widget{
				Type:  WidgetMetric,
				Title: "Latency",
			},
			setupDash: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			if tt.setupDash {
				mustCreateDashboard(t, svc, &Dashboard{ID: tt.dashboardID, Name: "Test", OwnerID: "u1"})
			}

			err := svc.AddWidget(tt.dashboardID, tt.widget)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddWidget() error = %v, wantErr = %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				d, ok := svc.GetDashboard(tt.dashboardID)
				if !ok {
					t.Fatal("expected dashboard to exist")
				}
				if len(d.Widgets) != 1 {
					t.Fatalf("expected 1 widget, got %d", len(d.Widgets))
				}
				if tt.widget.ID == "" {
					t.Error("expected widget ID to be auto-generated")
				}
				if d.Widgets[0].Title != tt.widget.Title {
					t.Errorf("expected widget title %q, got %q", tt.widget.Title, d.Widgets[0].Title)
				}
			}
		})
	}
}

func TestAddWidget_AutoGeneratesID(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-1", Name: "Test", OwnerID: "u1"})

	w := &Widget{Type: WidgetLog, Title: "Logs"}
	if err := svc.AddWidget("dash-1", w); err != nil {
		t.Fatalf("AddWidget() error = %v", err)
	}
	if w.ID == "" {
		t.Error("expected auto-generated widget ID")
	}
}

func TestAddWidget_MultipleWidgets(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-1", Name: "Test", OwnerID: "u1"})

	for i := 0; i < 5; i++ {
		w := &Widget{
			Type:  WidgetMetric,
			Title: fmt.Sprintf("Widget %d", i),
		}
		if err := svc.AddWidget("dash-1", w); err != nil {
			t.Fatalf("AddWidget(%d) error = %v", i, err)
		}
	}

	d, _ := svc.GetDashboard("dash-1")
	if len(d.Widgets) != 5 {
		t.Errorf("expected 5 widgets, got %d", len(d.Widgets))
	}
}

func TestRemoveWidget(t *testing.T) {
	tests := []struct {
		name        string
		dashboardID string
		widgetID    string
		setupDash   bool
		setupWidget bool
		wantErr     bool
	}{
		{
			name:        "removes existing widget",
			dashboardID: "dash-1",
			widgetID:    "widget-1",
			setupDash:   true,
			setupWidget: true,
			wantErr:     false,
		},
		{
			name:        "returns error for non-existent widget",
			dashboardID: "dash-1",
			widgetID:    "no-such-widget",
			setupDash:   true,
			setupWidget: false,
			wantErr:     true,
		},
		{
			name:        "returns error for non-existent dashboard",
			dashboardID: "no-such-dash",
			widgetID:    "widget-1",
			setupDash:   false,
			setupWidget: false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			if tt.setupDash {
				mustCreateDashboard(t, svc, &Dashboard{ID: tt.dashboardID, Name: "Test", OwnerID: "u1"})
			}
			if tt.setupWidget {
				w := &Widget{ID: tt.widgetID, Type: WidgetChart, Title: "Chart"}
				if err := svc.AddWidget(tt.dashboardID, w); err != nil {
					t.Fatalf("setup AddWidget error: %v", err)
				}
			}

			err := svc.RemoveWidget(tt.dashboardID, tt.widgetID)
			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveWidget() error = %v, wantErr = %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				d, _ := svc.GetDashboard(tt.dashboardID)
				if len(d.Widgets) != 0 {
					t.Errorf("expected 0 widgets after removal, got %d", len(d.Widgets))
				}
			}
		})
	}
}

func TestRemoveWidget_FromMiddle(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-1", Name: "Test", OwnerID: "u1"})

	widgets := []*Widget{
		{ID: "w1", Type: WidgetChart, Title: "First"},
		{ID: "w2", Type: WidgetTable, Title: "Second"},
		{ID: "w3", Type: WidgetMetric, Title: "Third"},
	}
	for _, w := range widgets {
		if err := svc.AddWidget("dash-1", w); err != nil {
			t.Fatalf("AddWidget error: %v", err)
		}
	}

	// Remove the middle widget
	if err := svc.RemoveWidget("dash-1", "w2"); err != nil {
		t.Fatalf("RemoveWidget error: %v", err)
	}

	d, _ := svc.GetDashboard("dash-1")
	if len(d.Widgets) != 2 {
		t.Fatalf("expected 2 widgets, got %d", len(d.Widgets))
	}
	if d.Widgets[0].ID != "w1" {
		t.Errorf("expected first widget ID 'w1', got %q", d.Widgets[0].ID)
	}
	if d.Widgets[1].ID != "w3" {
		t.Errorf("expected second widget ID 'w3', got %q", d.Widgets[1].ID)
	}
}

// ============================================================================
// WIDGET TYPE TESTS
// ============================================================================

func TestWidgetTypes(t *testing.T) {
	tests := []struct {
		name     string
		wtype    WidgetType
		expected string
	}{
		{"chart", WidgetChart, "chart"},
		{"table", WidgetTable, "table"},
		{"metric", WidgetMetric, "metric"},
		{"log", WidgetLog, "log"},
		{"alert", WidgetAlert, "alert"},
		{"topology", WidgetTopology, "topology"},
		{"text", WidgetText, "text"},
		{"iframe", WidgetIframe, "iframe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.wtype) != tt.expected {
				t.Errorf("WidgetType %q = %q, want %q", tt.name, tt.wtype, tt.expected)
			}
		})
	}
}

// ============================================================================
// LAYOUT TYPE TESTS
// ============================================================================

func TestLayoutTypes(t *testing.T) {
	tests := []struct {
		name     string
		ltype    LayoutType
		expected string
	}{
		{"grid", LayoutGrid, "grid"},
		{"freeform", LayoutFreeform, "freeform"},
		{"fixed", LayoutFixed, "fixed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.ltype) != tt.expected {
				t.Errorf("LayoutType %q = %q, want %q", tt.name, tt.ltype, tt.expected)
			}
		})
	}
}

// ============================================================================
// SESSION MANAGEMENT TESTS
// ============================================================================

func TestCreateSession(t *testing.T) {
	t.Run("creates session with defaults", func(t *testing.T) {
		svc := newTestService()
		sess := mustCreateSession(t, svc, "user-1")

		if sess.ID == "" {
			t.Error("expected session ID to be generated")
		}
		if sess.UserID != "user-1" {
			t.Errorf("expected UserID 'user-1', got %q", sess.UserID)
		}
		if sess.Theme != "dark" {
			t.Errorf("expected default theme 'dark', got %q", sess.Theme)
		}
		if sess.Preferences == nil {
			t.Error("expected Preferences to be initialized")
		}
		if sess.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
		if sess.LastActivity.IsZero() {
			t.Error("expected LastActivity to be set")
		}
		if sess.ExpiresAt.IsZero() {
			t.Error("expected ExpiresAt to be set")
		}
		if sess.ExpiresAt.Before(sess.CreatedAt) {
			t.Error("expected ExpiresAt to be after CreatedAt")
		}
	})

	t.Run("creates unique sessions for same user", func(t *testing.T) {
		svc := newTestService()
		s1 := mustCreateSession(t, svc, "user-1")
		// small sleep to ensure different nanosecond timestamps
		time.Sleep(time.Nanosecond)
		s2 := mustCreateSession(t, svc, "user-1")

		if s1.ID == s2.ID {
			t.Error("expected different session IDs for same user")
		}
	})

	t.Run("session expires according to config timeout", func(t *testing.T) {
		cfg := &Config{
			SessionTimeout:  2 * time.Hour,
			BroadcastBuffer: 100,
			DefaultTheme:    "dark",
			DefaultLayout:   LayoutGrid,
		}
		svc := newTestServiceWithConfig(cfg)
		sess := mustCreateSession(t, svc, "user-1")

		// ExpiresAt should be approximately 2 hours from now
		expectedExpiry := sess.CreatedAt.Add(2 * time.Hour)
		diff := sess.ExpiresAt.Sub(expectedExpiry)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("ExpiresAt differs from expected by %v", diff)
		}
	})
}

func TestGetSession(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*Service) string // returns session ID
		wantOK bool
	}{
		{
			name: "retrieves existing session",
			setup: func(svc *Service) string {
				sess := mustCreateSession(t, svc, "user-1")
				return sess.ID
			},
			wantOK: true,
		},
		{
			name: "returns false for non-existent session",
			setup: func(svc *Service) string {
				return "no-such-session"
			},
			wantOK: false,
		},
		{
			name: "returns false for expired session",
			setup: func(svc *Service) string {
				sess := mustCreateSession(t, svc, "user-1")
				// Manually expire the session
				svc.mu.Lock()
				sess.ExpiresAt = time.Now().Add(-1 * time.Hour)
				svc.mu.Unlock()
				return sess.ID
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			sessionID := tt.setup(svc)

			_, ok := svc.GetSession(sessionID)
			if ok != tt.wantOK {
				t.Errorf("GetSession() ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestTouchSession(t *testing.T) {
	t.Run("updates activity and extends expiry", func(t *testing.T) {
		svc := newTestService()
		sess := mustCreateSession(t, svc, "user-1")

		originalActivity := sess.LastActivity
		originalExpiry := sess.ExpiresAt

		time.Sleep(time.Millisecond) // ensure time progression

		err := svc.TouchSession(sess.ID)
		if err != nil {
			t.Fatalf("TouchSession() error = %v", err)
		}

		updated, ok := svc.GetSession(sess.ID)
		if !ok {
			t.Fatal("expected session to still exist")
		}
		if !updated.LastActivity.After(originalActivity) {
			t.Error("expected LastActivity to be updated")
		}
		if !updated.ExpiresAt.After(originalExpiry) || updated.ExpiresAt.Equal(originalExpiry) {
			t.Error("expected ExpiresAt to be extended")
		}
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		svc := newTestService()
		err := svc.TouchSession("no-such-session")
		if err == nil {
			t.Error("expected error for non-existent session")
		}
	})
}

func TestDestroySession(t *testing.T) {
	t.Run("destroys existing session", func(t *testing.T) {
		svc := newTestService()
		sess := mustCreateSession(t, svc, "user-1")

		err := svc.DestroySession(sess.ID)
		if err != nil {
			t.Fatalf("DestroySession() error = %v", err)
		}

		_, ok := svc.GetSession(sess.ID)
		if ok {
			t.Error("expected session to be destroyed")
		}
	})

	t.Run("no error when destroying non-existent session", func(t *testing.T) {
		svc := newTestService()
		err := svc.DestroySession("no-such-session")
		if err != nil {
			t.Errorf("DestroySession() should not error for non-existent session, got %v", err)
		}
	})
}

func TestCleanupExpiredSessions(t *testing.T) {
	svc := newTestService()

	// Create 3 sessions, expire 2
	s1 := mustCreateSession(t, svc, "user-1")
	s2 := mustCreateSession(t, svc, "user-2")
	s3 := mustCreateSession(t, svc, "user-3")

	svc.mu.Lock()
	s1.ExpiresAt = time.Now().Add(-1 * time.Hour) // expired
	s3.ExpiresAt = time.Now().Add(-1 * time.Hour) // expired
	svc.mu.Unlock()

	svc.cleanupExpiredSessions()

	_, ok1 := svc.GetSession(s1.ID)
	_, ok2 := svc.GetSession(s2.ID)
	_, ok3 := svc.GetSession(s3.ID)

	if ok1 {
		t.Error("expected s1 to be cleaned up")
	}
	if !ok2 {
		t.Error("expected s2 to still exist")
	}
	if ok3 {
		t.Error("expected s3 to be cleaned up")
	}
}

// ============================================================================
// NOTIFICATION TESTS
// ============================================================================

func TestAddNotification(t *testing.T) {
	t.Run("adds notification with auto-generated ID", func(t *testing.T) {
		svc := newTestService()

		notif := &Notification{
			UserID:  "user-1",
			Type:    NotifyInfo,
			Title:   "Test",
			Message: "Hello World",
		}
		err := svc.AddNotification(notif)
		if err != nil {
			t.Fatalf("AddNotification() error = %v", err)
		}
		if notif.ID == "" {
			t.Error("expected auto-generated ID")
		}
		if notif.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}

		drainBroadcasts(svc) // consume the broadcast
	})

	t.Run("preserves provided ID", func(t *testing.T) {
		svc := newTestService()
		notif := &Notification{
			ID:      "custom-notif",
			UserID:  "user-1",
			Type:    NotifySuccess,
			Title:   "Done",
			Message: "All good",
		}
		err := svc.AddNotification(notif)
		if err != nil {
			t.Fatalf("AddNotification() error = %v", err)
		}
		if notif.ID != "custom-notif" {
			t.Errorf("expected ID 'custom-notif', got %q", notif.ID)
		}
		drainBroadcasts(svc)
	})

	t.Run("prepends notification (newest first)", func(t *testing.T) {
		svc := newTestService()
		for i := 0; i < 3; i++ {
			time.Sleep(time.Nanosecond) // ensure ordering
			svc.AddNotification(&Notification{
				ID:      fmt.Sprintf("notif-%d", i),
				UserID:  "user-1",
				Type:    NotifyInfo,
				Title:   fmt.Sprintf("Notif %d", i),
				Message: "msg",
			})
		}
		drainBroadcasts(svc)

		notifs := svc.GetNotifications("user-1", false)
		if len(notifs) != 3 {
			t.Fatalf("expected 3 notifications, got %d", len(notifs))
		}
		// Newest should be first
		if notifs[0].ID != "notif-2" {
			t.Errorf("expected newest first, got %q", notifs[0].ID)
		}
		if notifs[2].ID != "notif-0" {
			t.Errorf("expected oldest last, got %q", notifs[2].ID)
		}
	})

	t.Run("limits notifications to MaxNotifications", func(t *testing.T) {
		cfg := &Config{
			MaxNotifications: 3,
			BroadcastBuffer:  100,
			DefaultTheme:     "dark",
			DefaultLayout:    LayoutGrid,
			SessionTimeout:   time.Hour,
		}
		svc := newTestServiceWithConfig(cfg)

		for i := 0; i < 5; i++ {
			svc.AddNotification(&Notification{
				ID:      fmt.Sprintf("notif-%d", i),
				UserID:  "user-1",
				Type:    NotifyInfo,
				Title:   fmt.Sprintf("Notif %d", i),
				Message: "msg",
			})
		}
		drainBroadcasts(svc)

		notifs := svc.GetNotifications("user-1", false)
		if len(notifs) != 3 {
			t.Errorf("expected 3 notifications (max), got %d", len(notifs))
		}
	})

	t.Run("sends broadcast on new notification", func(t *testing.T) {
		svc := newTestService()
		drainBroadcasts(svc)

		svc.AddNotification(&Notification{
			UserID:  "user-1",
			Type:    NotifyWarning,
			Title:   "Alert",
			Message: "Something happened",
		})

		broadcasts := drainBroadcasts(svc)
		if len(broadcasts) != 1 {
			t.Fatalf("expected 1 broadcast, got %d", len(broadcasts))
		}
		if broadcasts[0].Type != "notification.new" {
			t.Errorf("expected broadcast type 'notification.new', got %q", broadcasts[0].Type)
		}
		if broadcasts[0].Target != "user:user-1" {
			t.Errorf("expected broadcast target 'user:user-1', got %q", broadcasts[0].Target)
		}
	})
}

func TestGetNotifications(t *testing.T) {
	t.Run("returns all notifications for user", func(t *testing.T) {
		svc := newTestService()
		for i := 0; i < 3; i++ {
			svc.AddNotification(&Notification{
				ID:      fmt.Sprintf("notif-%d", i),
				UserID:  "user-1",
				Type:    NotifyInfo,
				Title:   "Test",
				Message: "msg",
			})
		}
		drainBroadcasts(svc)

		notifs := svc.GetNotifications("user-1", false)
		if len(notifs) != 3 {
			t.Errorf("expected 3 notifications, got %d", len(notifs))
		}
	})

	t.Run("returns only unread notifications", func(t *testing.T) {
		svc := newTestService()
		for i := 0; i < 3; i++ {
			svc.AddNotification(&Notification{
				ID:      fmt.Sprintf("notif-%d", i),
				UserID:  "user-1",
				Type:    NotifyInfo,
				Title:   "Test",
				Message: "msg",
			})
		}
		drainBroadcasts(svc)

		// Mark one as read
		svc.MarkNotificationRead("user-1", "notif-1")

		unread := svc.GetNotifications("user-1", true)
		if len(unread) != 2 {
			t.Errorf("expected 2 unread notifications, got %d", len(unread))
		}
	})

	t.Run("returns nil for user with no notifications", func(t *testing.T) {
		svc := newTestService()
		notifs := svc.GetNotifications("no-such-user", false)
		if notifs != nil {
			t.Errorf("expected nil notifications, got %v", notifs)
		}
	})

	t.Run("does not mix notifications between users", func(t *testing.T) {
		svc := newTestService()
		svc.AddNotification(&Notification{ID: "n1", UserID: "user-1", Type: NotifyInfo, Title: "U1", Message: "for u1"})
		svc.AddNotification(&Notification{ID: "n2", UserID: "user-2", Type: NotifyInfo, Title: "U2", Message: "for u2"})
		drainBroadcasts(svc)

		u1Notifs := svc.GetNotifications("user-1", false)
		u2Notifs := svc.GetNotifications("user-2", false)

		if len(u1Notifs) != 1 {
			t.Errorf("expected 1 notification for user-1, got %d", len(u1Notifs))
		}
		if len(u2Notifs) != 1 {
			t.Errorf("expected 1 notification for user-2, got %d", len(u2Notifs))
		}
		if u1Notifs[0].ID != "n1" {
			t.Errorf("expected user-1 notification ID 'n1', got %q", u1Notifs[0].ID)
		}
		if u2Notifs[0].ID != "n2" {
			t.Errorf("expected user-2 notification ID 'n2', got %q", u2Notifs[0].ID)
		}
	})
}

func TestMarkNotificationRead(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		notificationID string
		setupNotifs    bool
		wantErr        bool
	}{
		{
			name:           "marks existing notification as read",
			userID:         "user-1",
			notificationID: "notif-1",
			setupNotifs:    true,
			wantErr:        false,
		},
		{
			name:           "returns error for non-existent notification",
			userID:         "user-1",
			notificationID: "no-such-notif",
			setupNotifs:    true,
			wantErr:        true,
		},
		{
			name:           "returns error for user with no notifications",
			userID:         "no-such-user",
			notificationID: "notif-1",
			setupNotifs:    false,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			if tt.setupNotifs {
				svc.AddNotification(&Notification{
					ID:      "notif-1",
					UserID:  "user-1",
					Type:    NotifyInfo,
					Title:   "Test",
					Message: "msg",
				})
				drainBroadcasts(svc)
			}

			err := svc.MarkNotificationRead(tt.userID, tt.notificationID)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarkNotificationRead() error = %v, wantErr = %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Verify it is actually marked read
				notifs := svc.GetNotifications(tt.userID, true)
				for _, n := range notifs {
					if n.ID == tt.notificationID {
						t.Error("expected notification to not appear in unread list")
					}
				}
			}
		})
	}
}

func TestNotificationTypes(t *testing.T) {
	tests := []struct {
		name     string
		ntype    NotificationType
		expected string
	}{
		{"info", NotifyInfo, "info"},
		{"success", NotifySuccess, "success"},
		{"warning", NotifyWarning, "warning"},
		{"error", NotifyError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.ntype) != tt.expected {
				t.Errorf("NotificationType %q = %q, want %q", tt.name, tt.ntype, tt.expected)
			}
		})
	}
}

// ============================================================================
// BROADCAST TESTS
// ============================================================================

func TestBroadcast(t *testing.T) {
	t.Run("sends broadcast to channel", func(t *testing.T) {
		svc := newTestService()
		b := &Broadcast{
			Type:    "test.event",
			Target:  "all",
			Payload: "hello",
		}
		svc.Broadcast(b)

		select {
		case received := <-svc.broadcasts:
			if received.Type != "test.event" {
				t.Errorf("expected type 'test.event', got %q", received.Type)
			}
			if received.Target != "all" {
				t.Errorf("expected target 'all', got %q", received.Target)
			}
		case <-time.After(time.Second):
			t.Error("timeout waiting for broadcast")
		}
	})

	t.Run("drops message when buffer is full", func(t *testing.T) {
		cfg := &Config{
			BroadcastBuffer:  1,
			DefaultTheme:     "dark",
			DefaultLayout:    LayoutGrid,
			SessionTimeout:   time.Hour,
			MaxNotifications: 100,
		}
		svc := newTestServiceWithConfig(cfg)

		// Fill the buffer
		svc.Broadcast(&Broadcast{Type: "fill", Target: "all", Payload: "1"})

		// This should not block (drops the message)
		svc.Broadcast(&Broadcast{Type: "overflow", Target: "all", Payload: "2"})

		// We should only get the first message
		select {
		case b := <-svc.broadcasts:
			if b.Type != "fill" {
				t.Errorf("expected first broadcast 'fill', got %q", b.Type)
			}
		case <-time.After(time.Second):
			t.Error("timeout waiting for broadcast")
		}
	})
}

func TestBroadcastLoop(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())

	go svc.broadcastLoop(ctx)

	// Send a broadcast
	svc.broadcasts <- &Broadcast{
		Type:    "test.loop",
		Target:  "all",
		Payload: "loop test",
	}

	// Give it time to process
	time.Sleep(50 * time.Millisecond)

	// Cancel context to stop the loop
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// ============================================================================
// SESSION CLEANUP LOOP TEST
// ============================================================================

func TestSessionCleanupLoop(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go svc.sessionCleanupLoop(ctx)

	// Just verify it runs without error by canceling the context
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// ============================================================================
// STATS TESTS
// ============================================================================

func TestStats(t *testing.T) {
	t.Run("returns zero stats for empty service", func(t *testing.T) {
		svc := newTestService()
		stats := svc.Stats()

		if stats["total_dashboards"].(int) != 0 {
			t.Errorf("expected 0 dashboards, got %v", stats["total_dashboards"])
		}
		if stats["active_sessions"].(int) != 0 {
			t.Errorf("expected 0 sessions, got %v", stats["active_sessions"])
		}
		if stats["total_notifications"].(int) != 0 {
			t.Errorf("expected 0 notifications, got %v", stats["total_notifications"])
		}
		if stats["users_with_notifications"].(int) != 0 {
			t.Errorf("expected 0 users with notifications, got %v", stats["users_with_notifications"])
		}
	})

	t.Run("returns accurate counts", func(t *testing.T) {
		svc := newTestService()

		// Create dashboards
		mustCreateDashboard(t, svc, &Dashboard{ID: "d1", Name: "One", OwnerID: "u1"})
		mustCreateDashboard(t, svc, &Dashboard{ID: "d2", Name: "Two", OwnerID: "u2"})

		// Create sessions
		mustCreateSession(t, svc, "user-1")
		mustCreateSession(t, svc, "user-2")
		mustCreateSession(t, svc, "user-3")

		// Create notifications
		svc.AddNotification(&Notification{UserID: "user-1", Type: NotifyInfo, Title: "T1", Message: "m1"})
		svc.AddNotification(&Notification{UserID: "user-1", Type: NotifyInfo, Title: "T2", Message: "m2"})
		svc.AddNotification(&Notification{UserID: "user-2", Type: NotifyInfo, Title: "T3", Message: "m3"})
		drainBroadcasts(svc)

		stats := svc.Stats()

		if stats["total_dashboards"].(int) != 2 {
			t.Errorf("expected 2 dashboards, got %v", stats["total_dashboards"])
		}
		if stats["active_sessions"].(int) != 3 {
			t.Errorf("expected 3 sessions, got %v", stats["active_sessions"])
		}
		if stats["total_notifications"].(int) != 3 {
			t.Errorf("expected 3 notifications, got %v", stats["total_notifications"])
		}
		if stats["users_with_notifications"].(int) != 2 {
			t.Errorf("expected 2 users with notifications, got %v", stats["users_with_notifications"])
		}
	})
}

// ============================================================================
// HTTP HANDLER TESTS
// ============================================================================

func TestHTTPHandler_Index(t *testing.T) {
	svc := newTestService()
	handler := svc.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type text/html, got %q", ct)
	}
	body := rec.Body.String()
	if body == "" {
		t.Error("expected non-empty response body")
	}
}

func TestHTTPHandler_Dashboards(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "d1", Name: "API Dash", OwnerID: "u1"})

	handler := svc.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboards", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var dashboards []*Dashboard
	if err := json.NewDecoder(rec.Body).Decode(&dashboards); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(dashboards) != 1 {
		t.Errorf("expected 1 dashboard, got %d", len(dashboards))
	}
}

func TestHTTPHandler_Sessions(t *testing.T) {
	svc := newTestService()
	mustCreateSession(t, svc, "user-1")
	mustCreateSession(t, svc, "user-2")

	handler := svc.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["active_sessions"].(float64) != 2 {
		t.Errorf("expected 2 active sessions, got %v", result["active_sessions"])
	}
}

func TestHTTPHandler_Notifications(t *testing.T) {
	svc := newTestService()
	handler := svc.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var result []interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty notifications, got %d", len(result))
	}
}

// ============================================================================
// DATA AGGREGATION TESTS
// ============================================================================

func TestDashboardDataAggregation(t *testing.T) {
	t.Run("aggregates widgets with positions and sizes", func(t *testing.T) {
		svc := newTestService()
		mustCreateDashboard(t, svc, &Dashboard{
			ID:      "dash-agg",
			Name:    "Aggregation Test",
			OwnerID: "u1",
		})

		widgets := []struct {
			wtype    WidgetType
			title    string
			pos      Position
			size     Size
			source   string
			refresh  time.Duration
		}{
			{WidgetChart, "CPU", Position{0, 0}, Size{6, 4}, "metrics/cpu", 5 * time.Second},
			{WidgetTable, "Logs", Position{6, 0}, Size{6, 4}, "logs/recent", 10 * time.Second},
			{WidgetMetric, "RPS", Position{0, 4}, Size{3, 2}, "metrics/rps", 1 * time.Second},
			{WidgetAlert, "Alerts", Position{3, 4}, Size{3, 2}, "alerts/active", 30 * time.Second},
		}

		for _, w := range widgets {
			err := svc.AddWidget("dash-agg", &Widget{
				Type:        w.wtype,
				Title:       w.title,
				Source:      w.source,
				Position:    w.pos,
				Size:        w.size,
				RefreshRate: w.refresh,
			})
			if err != nil {
				t.Fatalf("AddWidget(%q) error = %v", w.title, err)
			}
		}

		d, ok := svc.GetDashboard("dash-agg")
		if !ok {
			t.Fatal("expected dashboard to exist")
		}
		if len(d.Widgets) != 4 {
			t.Fatalf("expected 4 widgets, got %d", len(d.Widgets))
		}

		// Verify positions are preserved
		if d.Widgets[0].Position.X != 0 || d.Widgets[0].Position.Y != 0 {
			t.Errorf("expected first widget at (0,0), got (%d,%d)", d.Widgets[0].Position.X, d.Widgets[0].Position.Y)
		}
		if d.Widgets[1].Position.X != 6 || d.Widgets[1].Position.Y != 0 {
			t.Errorf("expected second widget at (6,0), got (%d,%d)", d.Widgets[1].Position.X, d.Widgets[1].Position.Y)
		}

		// Verify sizes are preserved
		if d.Widgets[0].Size.Width != 6 || d.Widgets[0].Size.Height != 4 {
			t.Errorf("expected first widget size (6,4), got (%d,%d)", d.Widgets[0].Size.Width, d.Widgets[0].Size.Height)
		}

		// Verify refresh rates
		if d.Widgets[0].RefreshRate != 5*time.Second {
			t.Errorf("expected refresh rate 5s, got %v", d.Widgets[0].RefreshRate)
		}
	})

	t.Run("dashboard with config maps on widgets", func(t *testing.T) {
		svc := newTestService()
		mustCreateDashboard(t, svc, &Dashboard{ID: "dash-cfg", Name: "Config Test", OwnerID: "u1"})

		w := &Widget{
			Type:   WidgetChart,
			Title:  "Custom Chart",
			Source: "metrics/custom",
			Config: map[string]interface{}{
				"chartType":   "line",
				"showLegend":  true,
				"maxDataPoints": 100,
				"color":       "#ff0000",
			},
		}
		if err := svc.AddWidget("dash-cfg", w); err != nil {
			t.Fatalf("AddWidget() error = %v", err)
		}

		d, _ := svc.GetDashboard("dash-cfg")
		if d.Widgets[0].Config["chartType"] != "line" {
			t.Errorf("expected chartType 'line', got %v", d.Widgets[0].Config["chartType"])
		}
		if d.Widgets[0].Config["showLegend"] != true {
			t.Errorf("expected showLegend true, got %v", d.Widgets[0].Config["showLegend"])
		}
	})
}

// ============================================================================
// UI STATE TESTS
// ============================================================================

func TestUIState(t *testing.T) {
	t.Run("UIState struct fields serialize correctly", func(t *testing.T) {
		now := time.Now()
		state := &UIState{
			SessionID:     "sess-1",
			DashboardID:   "dash-1",
			ActiveWidgets: []string{"w1", "w2", "w3"},
			Filters: map[string]string{
				"service": "api",
				"level":   "error",
			},
			TimeRange: &TimeRange{
				Start:  now.Add(-1 * time.Hour),
				End:    now,
				Preset: "1h",
			},
			Zoom: 1.5,
		}

		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("failed to marshal UIState: %v", err)
		}

		var decoded UIState
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal UIState: %v", err)
		}

		if decoded.SessionID != "sess-1" {
			t.Errorf("expected SessionID 'sess-1', got %q", decoded.SessionID)
		}
		if decoded.DashboardID != "dash-1" {
			t.Errorf("expected DashboardID 'dash-1', got %q", decoded.DashboardID)
		}
		if len(decoded.ActiveWidgets) != 3 {
			t.Errorf("expected 3 active widgets, got %d", len(decoded.ActiveWidgets))
		}
		if decoded.Filters["service"] != "api" {
			t.Errorf("expected filter service=api, got %q", decoded.Filters["service"])
		}
		if decoded.TimeRange == nil {
			t.Fatal("expected TimeRange to be non-nil")
		}
		if decoded.TimeRange.Preset != "1h" {
			t.Errorf("expected preset '1h', got %q", decoded.TimeRange.Preset)
		}
		if decoded.Zoom != 1.5 {
			t.Errorf("expected zoom 1.5, got %f", decoded.Zoom)
		}
	})
}

func TestTimeRange(t *testing.T) {
	tests := []struct {
		name   string
		preset string
	}{
		{"1 hour", "1h"},
		{"24 hours", "24h"},
		{"7 days", "7d"},
		{"30 days", "30d"},
		{"custom", "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &TimeRange{
				Start:  time.Now().Add(-1 * time.Hour),
				End:    time.Now(),
				Preset: tt.preset,
			}

			data, err := json.Marshal(tr)
			if err != nil {
				t.Fatalf("failed to marshal TimeRange: %v", err)
			}

			var decoded TimeRange
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal TimeRange: %v", err)
			}

			if decoded.Preset != tt.preset {
				t.Errorf("expected preset %q, got %q", tt.preset, decoded.Preset)
			}
		})
	}
}

// ============================================================================
// REAL-TIME UPDATE / SUBSCRIPTION TESTS
// ============================================================================

func TestBroadcastOnDashboardUpdate(t *testing.T) {
	svc := newTestService()
	d := &Dashboard{ID: "dash-rt", Name: "Realtime", OwnerID: "u1"}
	mustCreateDashboard(t, svc, d)
	drainBroadcasts(svc)

	// Update the dashboard
	updated := &Dashboard{
		ID:      "dash-rt",
		Name:    "Realtime Updated",
		OwnerID: "u1",
	}
	if err := svc.UpdateDashboard(updated); err != nil {
		t.Fatalf("UpdateDashboard() error = %v", err)
	}

	broadcasts := drainBroadcasts(svc)
	found := false
	for _, b := range broadcasts {
		if b.Type == "dashboard.updated" && b.Target == "dashboard:dash-rt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected dashboard.updated broadcast")
	}
}

func TestBroadcastOnNewNotification(t *testing.T) {
	svc := newTestService()
	drainBroadcasts(svc)

	svc.AddNotification(&Notification{
		UserID:  "user-rt",
		Type:    NotifyError,
		Title:   "Critical",
		Message: "System failure",
	})

	broadcasts := drainBroadcasts(svc)
	found := false
	for _, b := range broadcasts {
		if b.Type == "notification.new" && b.Target == "user:user-rt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected notification.new broadcast")
	}
}

func TestBroadcastStruct(t *testing.T) {
	t.Run("serializes correctly", func(t *testing.T) {
		b := &Broadcast{
			Type:    "test.event",
			Target:  "dashboard:dash-1",
			Payload: map[string]string{"key": "value"},
		}

		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("failed to marshal broadcast: %v", err)
		}

		var decoded Broadcast
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal broadcast: %v", err)
		}

		if decoded.Type != "test.event" {
			t.Errorf("expected type 'test.event', got %q", decoded.Type)
		}
		if decoded.Target != "dashboard:dash-1" {
			t.Errorf("expected target 'dashboard:dash-1', got %q", decoded.Target)
		}
	})
}

// ============================================================================
// CONCURRENCY / RACE SAFETY TESTS
// ============================================================================

func TestConcurrent_DashboardOperations(t *testing.T) {
	svc := newTestService()

	var wg sync.WaitGroup
	concurrency := 50

	// Concurrent creates
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			d := &Dashboard{
				ID:      fmt.Sprintf("dash-%d", i),
				Name:    fmt.Sprintf("Dashboard %d", i),
				OwnerID: fmt.Sprintf("user-%d", i%5),
			}
			if err := svc.CreateDashboard(d); err != nil {
				t.Errorf("concurrent CreateDashboard(%d) error: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	// Verify all dashboards were created
	all := svc.ListDashboards("")
	if len(all) != concurrency {
		t.Errorf("expected %d dashboards, got %d", concurrency, len(all))
	}

	// Concurrent reads
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			_, ok := svc.GetDashboard(fmt.Sprintf("dash-%d", i))
			if !ok {
				t.Errorf("concurrent GetDashboard(%d) not found", i)
			}
		}(i)
	}
	wg.Wait()

	// Concurrent reads + writes (mixed)
	wg.Add(concurrency * 2)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			svc.ListDashboards(fmt.Sprintf("user-%d", i%5))
		}(i)
		go func(i int) {
			defer wg.Done()
			svc.GetDashboard(fmt.Sprintf("dash-%d", i))
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_SessionOperations(t *testing.T) {
	svc := newTestService()
	concurrency := 50

	var wg sync.WaitGroup
	sessionIDs := make([]string, concurrency)
	var mu sync.Mutex

	// Concurrent session creation
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			sess, err := svc.CreateSession(fmt.Sprintf("user-%d", i))
			if err != nil {
				t.Errorf("concurrent CreateSession(%d) error: %v", i, err)
				return
			}
			mu.Lock()
			sessionIDs[i] = sess.ID
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	// Concurrent touch + get
	wg.Add(concurrency * 2)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			mu.Lock()
			sid := sessionIDs[i]
			mu.Unlock()
			svc.TouchSession(sid)
		}(i)
		go func(i int) {
			defer wg.Done()
			mu.Lock()
			sid := sessionIDs[i]
			mu.Unlock()
			svc.GetSession(sid)
		}(i)
	}
	wg.Wait()

	// Concurrent destroys
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			mu.Lock()
			sid := sessionIDs[i]
			mu.Unlock()
			svc.DestroySession(sid)
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_NotificationOperations(t *testing.T) {
	svc := newTestService()
	concurrency := 50

	var wg sync.WaitGroup

	// Concurrent notification adds
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			svc.AddNotification(&Notification{
				UserID:  fmt.Sprintf("user-%d", i%5),
				Type:    NotifyInfo,
				Title:   fmt.Sprintf("Notif %d", i),
				Message: "concurrent test",
			})
		}(i)
	}
	wg.Wait()
	drainBroadcasts(svc)

	// Concurrent reads
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			svc.GetNotifications(fmt.Sprintf("user-%d", i%5), false)
			svc.GetNotifications(fmt.Sprintf("user-%d", i%5), true)
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_Broadcasts(t *testing.T) {
	svc := newTestService()
	concurrency := 50

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			svc.Broadcast(&Broadcast{
				Type:    "concurrent.test",
				Target:  fmt.Sprintf("user:%d", i),
				Payload: i,
			})
		}(i)
	}
	wg.Wait()

	// Drain all broadcasts
	drainBroadcasts(svc)
}

func TestConcurrent_Stats(t *testing.T) {
	svc := newTestService()

	// Pre-populate some data
	for i := 0; i < 10; i++ {
		mustCreateDashboard(t, svc, &Dashboard{ID: fmt.Sprintf("d%d", i), Name: "Test", OwnerID: "u1"})
		mustCreateSession(t, svc, fmt.Sprintf("user-%d", i))
		svc.AddNotification(&Notification{UserID: fmt.Sprintf("user-%d", i), Type: NotifyInfo, Title: "T", Message: "m"})
	}
	drainBroadcasts(svc)

	// Concurrent stats reads while modifying data
	var wg sync.WaitGroup
	concurrency := 50
	wg.Add(concurrency * 2)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			svc.Stats()
		}()
		go func(i int) {
			defer wg.Done()
			svc.CreateDashboard(&Dashboard{Name: "Concurrent", OwnerID: "uc"})
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_MixedOperations(t *testing.T) {
	svc := newTestService()
	concurrency := 20

	// Pre-populate
	mustCreateDashboard(t, svc, &Dashboard{ID: "shared-dash", Name: "Shared", OwnerID: "u1"})

	var wg sync.WaitGroup
	wg.Add(concurrency * 5)

	for i := 0; i < concurrency; i++ {
		// Dashboard operations
		go func(i int) {
			defer wg.Done()
			svc.CreateDashboard(&Dashboard{Name: fmt.Sprintf("D%d", i), OwnerID: "u1"})
		}(i)
		go func() {
			defer wg.Done()
			svc.ListDashboards("u1")
		}()

		// Session operations
		go func(i int) {
			defer wg.Done()
			sess, _ := svc.CreateSession(fmt.Sprintf("user-%d", i))
			if sess != nil {
				svc.TouchSession(sess.ID)
			}
		}(i)

		// Notification operations
		go func(i int) {
			defer wg.Done()
			svc.AddNotification(&Notification{
				UserID:  "u1",
				Type:    NotifyInfo,
				Title:   fmt.Sprintf("N%d", i),
				Message: "test",
			})
		}(i)

		// Widget operations
		go func(i int) {
			defer wg.Done()
			svc.AddWidget("shared-dash", &Widget{
				Type:  WidgetMetric,
				Title: fmt.Sprintf("W%d", i),
			})
		}(i)
	}
	wg.Wait()
	drainBroadcasts(svc)
}

// ============================================================================
// EDGE CASES AND ERROR PATH TESTS
// ============================================================================

func TestEdgeCases_EmptyDashboard(t *testing.T) {
	svc := newTestService()
	d := &Dashboard{Name: "Empty", OwnerID: "u1"}
	mustCreateDashboard(t, svc, d)

	got, ok := svc.GetDashboard(d.ID)
	if !ok {
		t.Fatal("expected dashboard to exist")
	}
	if got.Widgets != nil && len(got.Widgets) != 0 {
		t.Errorf("expected nil or empty widgets, got %d", len(got.Widgets))
	}
}

func TestEdgeCases_DashboardWithEmptyName(t *testing.T) {
	svc := newTestService()
	d := &Dashboard{OwnerID: "u1"}
	mustCreateDashboard(t, svc, d)

	got, ok := svc.GetDashboard(d.ID)
	if !ok {
		t.Fatal("expected dashboard to exist even with empty name")
	}
	if got.Name != "" {
		t.Errorf("expected empty name, got %q", got.Name)
	}
}

func TestEdgeCases_SessionWithEmptyUserID(t *testing.T) {
	svc := newTestService()
	sess, err := svc.CreateSession("")
	if err != nil {
		t.Fatalf("CreateSession('') error = %v", err)
	}
	if sess.UserID != "" {
		t.Errorf("expected empty UserID, got %q", sess.UserID)
	}
}

func TestEdgeCases_NotificationWithLink(t *testing.T) {
	svc := newTestService()
	notif := &Notification{
		UserID:  "user-1",
		Type:    NotifyInfo,
		Title:   "Linked",
		Message: "Click here",
		Link:    "https://example.com/details",
	}
	svc.AddNotification(notif)
	drainBroadcasts(svc)

	notifs := svc.GetNotifications("user-1", false)
	if len(notifs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifs))
	}
	if notifs[0].Link != "https://example.com/details" {
		t.Errorf("expected link to be preserved, got %q", notifs[0].Link)
	}
}

func TestEdgeCases_DefaultDashboard(t *testing.T) {
	svc := newTestService()
	d := &Dashboard{
		Name:      "Default Dashboard",
		OwnerID:   "u1",
		IsDefault: true,
	}
	mustCreateDashboard(t, svc, d)

	got, ok := svc.GetDashboard(d.ID)
	if !ok {
		t.Fatal("expected dashboard to exist")
	}
	if !got.IsDefault {
		t.Error("expected IsDefault to be true")
	}
}

func TestEdgeCases_DoubleDelete(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-dd", Name: "Delete Twice", OwnerID: "u1"})

	err := svc.DeleteDashboard("dash-dd")
	if err != nil {
		t.Fatalf("first delete error: %v", err)
	}

	err = svc.DeleteDashboard("dash-dd")
	if err == nil {
		t.Error("expected error on second delete")
	}
}

func TestEdgeCases_DoubleUpdate(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-du", Name: "Original", OwnerID: "u1"})
	drainBroadcasts(svc)

	// Update twice in sequence
	err := svc.UpdateDashboard(&Dashboard{ID: "dash-du", Name: "First Update", OwnerID: "u1"})
	if err != nil {
		t.Fatalf("first update error: %v", err)
	}
	err = svc.UpdateDashboard(&Dashboard{ID: "dash-du", Name: "Second Update", OwnerID: "u1"})
	if err != nil {
		t.Fatalf("second update error: %v", err)
	}

	got, _ := svc.GetDashboard("dash-du")
	if got.Name != "Second Update" {
		t.Errorf("expected name 'Second Update', got %q", got.Name)
	}

	broadcasts := drainBroadcasts(svc)
	if len(broadcasts) != 2 {
		t.Errorf("expected 2 broadcasts for 2 updates, got %d", len(broadcasts))
	}
}

func TestEdgeCases_WidgetWithZeroSize(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-zs", Name: "Test", OwnerID: "u1"})

	w := &Widget{
		Type:     WidgetText,
		Title:    "Zero Size",
		Position: Position{0, 0},
		Size:     Size{0, 0},
	}
	err := svc.AddWidget("dash-zs", w)
	if err != nil {
		t.Fatalf("AddWidget() error = %v", err)
	}

	d, _ := svc.GetDashboard("dash-zs")
	if d.Widgets[0].Size.Width != 0 || d.Widgets[0].Size.Height != 0 {
		t.Error("expected zero size to be preserved")
	}
}

func TestEdgeCases_WidgetWithNegativePosition(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-np", Name: "Test", OwnerID: "u1"})

	w := &Widget{
		Type:     WidgetText,
		Title:    "Negative",
		Position: Position{-1, -5},
		Size:     Size{3, 3},
	}
	err := svc.AddWidget("dash-np", w)
	if err != nil {
		t.Fatalf("AddWidget() error = %v", err)
	}

	d, _ := svc.GetDashboard("dash-np")
	if d.Widgets[0].Position.X != -1 || d.Widgets[0].Position.Y != -5 {
		t.Error("expected negative position to be preserved")
	}
}

func TestEdgeCases_MarkAlreadyReadNotification(t *testing.T) {
	svc := newTestService()
	svc.AddNotification(&Notification{
		ID:      "notif-ar",
		UserID:  "user-1",
		Type:    NotifyInfo,
		Title:   "Test",
		Message: "msg",
		Read:    true, // already read when created
	})
	drainBroadcasts(svc)

	// Marking it read again should not error
	err := svc.MarkNotificationRead("user-1", "notif-ar")
	if err != nil {
		t.Errorf("MarkNotificationRead() on already-read should not error, got %v", err)
	}
}

func TestEdgeCases_RemoveOnlyWidget(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-row", Name: "Test", OwnerID: "u1"})
	svc.AddWidget("dash-row", &Widget{ID: "only-one", Type: WidgetChart, Title: "Only"})

	err := svc.RemoveWidget("dash-row", "only-one")
	if err != nil {
		t.Fatalf("RemoveWidget() error = %v", err)
	}

	d, _ := svc.GetDashboard("dash-row")
	if len(d.Widgets) != 0 {
		t.Errorf("expected 0 widgets, got %d", len(d.Widgets))
	}
}

func TestEdgeCases_RemoveLastWidget(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-rlw", Name: "Test", OwnerID: "u1"})
	svc.AddWidget("dash-rlw", &Widget{ID: "w1", Type: WidgetChart, Title: "First"})
	svc.AddWidget("dash-rlw", &Widget{ID: "w2", Type: WidgetChart, Title: "Second"})
	svc.AddWidget("dash-rlw", &Widget{ID: "w3", Type: WidgetChart, Title: "Third"})

	err := svc.RemoveWidget("dash-rlw", "w3")
	if err != nil {
		t.Fatalf("RemoveWidget() error = %v", err)
	}

	d, _ := svc.GetDashboard("dash-rlw")
	if len(d.Widgets) != 2 {
		t.Fatalf("expected 2 widgets, got %d", len(d.Widgets))
	}
	if d.Widgets[0].ID != "w1" || d.Widgets[1].ID != "w2" {
		t.Error("remaining widgets are not in expected order")
	}
}

func TestEdgeCases_RemoveFirstWidget(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-rfw", Name: "Test", OwnerID: "u1"})
	svc.AddWidget("dash-rfw", &Widget{ID: "w1", Type: WidgetChart, Title: "First"})
	svc.AddWidget("dash-rfw", &Widget{ID: "w2", Type: WidgetChart, Title: "Second"})
	svc.AddWidget("dash-rfw", &Widget{ID: "w3", Type: WidgetChart, Title: "Third"})

	err := svc.RemoveWidget("dash-rfw", "w1")
	if err != nil {
		t.Fatalf("RemoveWidget() error = %v", err)
	}

	d, _ := svc.GetDashboard("dash-rfw")
	if len(d.Widgets) != 2 {
		t.Fatalf("expected 2 widgets, got %d", len(d.Widgets))
	}
	if d.Widgets[0].ID != "w2" || d.Widgets[1].ID != "w3" {
		t.Error("remaining widgets are not in expected order")
	}
}

// ============================================================================
// JSON SERIALIZATION TESTS
// ============================================================================

func TestDashboard_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	d := &Dashboard{
		ID:          "dash-json",
		Name:        "JSON Test",
		Description: "Testing JSON round-trip",
		OwnerID:     "user-json",
		Widgets: []*Widget{
			{
				ID:     "w1",
				Type:   WidgetChart,
				Title:  "Chart",
				Source: "metrics/test",
				Config: map[string]interface{}{"key": "value"},
				Position: Position{X: 0, Y: 0},
				Size:     Size{Width: 6, Height: 4},
			},
		},
		Layout: &Layout{
			Type:      LayoutGrid,
			Columns:   12,
			RowHeight: 100,
			Margin:    10,
		},
		Theme:     "dark",
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("failed to marshal Dashboard: %v", err)
	}

	var decoded Dashboard
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Dashboard: %v", err)
	}

	if decoded.ID != d.ID {
		t.Errorf("ID: expected %q, got %q", d.ID, decoded.ID)
	}
	if decoded.Name != d.Name {
		t.Errorf("Name: expected %q, got %q", d.Name, decoded.Name)
	}
	if decoded.Description != d.Description {
		t.Errorf("Description: expected %q, got %q", d.Description, decoded.Description)
	}
	if decoded.OwnerID != d.OwnerID {
		t.Errorf("OwnerID: expected %q, got %q", d.OwnerID, decoded.OwnerID)
	}
	if len(decoded.Widgets) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(decoded.Widgets))
	}
	if decoded.Layout == nil {
		t.Fatal("expected Layout to be non-nil")
	}
	if decoded.Layout.Type != LayoutGrid {
		t.Errorf("Layout.Type: expected %q, got %q", LayoutGrid, decoded.Layout.Type)
	}
	if decoded.Theme != "dark" {
		t.Errorf("Theme: expected 'dark', got %q", decoded.Theme)
	}
	if !decoded.IsDefault {
		t.Error("expected IsDefault to be true")
	}
}

func TestSession_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	s := &Session{
		ID:          "sess-json",
		UserID:      "user-json",
		DashboardID: "dash-json",
		Theme:       "light",
		Preferences: map[string]interface{}{
			"sidebar": "collapsed",
			"density": "compact",
		},
		CreatedAt:    now,
		LastActivity: now,
		ExpiresAt:    now.Add(24 * time.Hour),
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("failed to marshal Session: %v", err)
	}

	var decoded Session
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Session: %v", err)
	}

	if decoded.ID != s.ID {
		t.Errorf("ID: expected %q, got %q", s.ID, decoded.ID)
	}
	if decoded.UserID != s.UserID {
		t.Errorf("UserID: expected %q, got %q", s.UserID, decoded.UserID)
	}
	if decoded.DashboardID != s.DashboardID {
		t.Errorf("DashboardID: expected %q, got %q", s.DashboardID, decoded.DashboardID)
	}
	if decoded.Theme != s.Theme {
		t.Errorf("Theme: expected %q, got %q", s.Theme, decoded.Theme)
	}
	if decoded.Preferences["sidebar"] != "collapsed" {
		t.Errorf("Preferences[sidebar]: expected 'collapsed', got %v", decoded.Preferences["sidebar"])
	}
}

func TestNotification_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	n := &Notification{
		ID:        "notif-json",
		UserID:    "user-json",
		Type:      NotifyWarning,
		Title:     "Warning",
		Message:   "Disk space low",
		Link:      "/admin/storage",
		Read:      false,
		CreatedAt: now,
	}

	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("failed to marshal Notification: %v", err)
	}

	var decoded Notification
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Notification: %v", err)
	}

	if decoded.ID != n.ID {
		t.Errorf("ID: expected %q, got %q", n.ID, decoded.ID)
	}
	if decoded.Type != NotifyWarning {
		t.Errorf("Type: expected 'warning', got %q", decoded.Type)
	}
	if decoded.Link != "/admin/storage" {
		t.Errorf("Link: expected '/admin/storage', got %q", decoded.Link)
	}
	if decoded.Read != false {
		t.Error("Read: expected false")
	}
}

// ============================================================================
// WIDGET RENDERING LOGIC TESTS
// ============================================================================

func TestWidgetRendering_AllTypes(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-render", Name: "Render Test", OwnerID: "u1"})

	allTypes := []struct {
		wtype  WidgetType
		title  string
		source string
		config map[string]interface{}
	}{
		{WidgetChart, "Chart Widget", "metrics/chart", map[string]interface{}{"chartType": "bar"}},
		{WidgetTable, "Table Widget", "data/table", map[string]interface{}{"columns": 5}},
		{WidgetMetric, "Metric Widget", "metrics/single", map[string]interface{}{"unit": "ms"}},
		{WidgetLog, "Log Widget", "logs/stream", map[string]interface{}{"maxLines": 100}},
		{WidgetAlert, "Alert Widget", "alerts/active", nil},
		{WidgetTopology, "Topology Widget", "topology/graph", map[string]interface{}{"layout": "force"}},
		{WidgetText, "Text Widget", "", map[string]interface{}{"content": "Hello"}},
		{WidgetIframe, "Iframe Widget", "https://example.com", map[string]interface{}{"sandbox": true}},
	}

	for _, wt := range allTypes {
		err := svc.AddWidget("dash-render", &Widget{
			Type:     wt.wtype,
			Title:    wt.title,
			Source:   wt.source,
			Config:   wt.config,
			Position: Position{0, 0},
			Size:     Size{6, 4},
		})
		if err != nil {
			t.Errorf("AddWidget(%q) error = %v", wt.title, err)
		}
	}

	d, _ := svc.GetDashboard("dash-render")
	if len(d.Widgets) != len(allTypes) {
		t.Errorf("expected %d widgets, got %d", len(allTypes), len(d.Widgets))
	}

	// Verify each widget type was stored correctly
	typeMap := make(map[WidgetType]bool)
	for _, w := range d.Widgets {
		typeMap[w.Type] = true
	}
	for _, wt := range allTypes {
		if !typeMap[wt.wtype] {
			t.Errorf("widget type %q not found in dashboard", wt.wtype)
		}
	}
}

func TestWidgetRendering_LayoutGrid(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{
		ID:      "dash-grid",
		Name:    "Grid Layout",
		OwnerID: "u1",
		Layout: &Layout{
			Type:      LayoutGrid,
			Columns:   12,
			RowHeight: 100,
			Margin:    10,
		},
	})

	// Add widgets in a grid pattern
	gridWidgets := []struct {
		id  string
		pos Position
		sz  Size
	}{
		{"tl", Position{0, 0}, Size{6, 4}},   // top-left
		{"tr", Position{6, 0}, Size{6, 4}},   // top-right
		{"bl", Position{0, 4}, Size{4, 3}},   // bottom-left
		{"bm", Position{4, 4}, Size{4, 3}},   // bottom-middle
		{"br", Position{8, 4}, Size{4, 3}},   // bottom-right
	}

	for _, gw := range gridWidgets {
		svc.AddWidget("dash-grid", &Widget{
			ID:       gw.id,
			Type:     WidgetMetric,
			Title:    gw.id,
			Position: gw.pos,
			Size:     gw.sz,
		})
	}

	d, _ := svc.GetDashboard("dash-grid")
	if len(d.Widgets) != 5 {
		t.Fatalf("expected 5 grid widgets, got %d", len(d.Widgets))
	}

	// Verify all positions are within grid bounds
	for _, w := range d.Widgets {
		if w.Position.X < 0 || w.Position.X >= d.Layout.Columns {
			t.Errorf("widget %q position X=%d out of grid bounds (0-%d)", w.ID, w.Position.X, d.Layout.Columns-1)
		}
		if w.Position.X+w.Size.Width > d.Layout.Columns {
			t.Errorf("widget %q extends beyond grid: X=%d + Width=%d > Columns=%d", w.ID, w.Position.X, w.Size.Width, d.Layout.Columns)
		}
	}
}

func TestWidgetRendering_RefreshRates(t *testing.T) {
	svc := newTestService()
	mustCreateDashboard(t, svc, &Dashboard{ID: "dash-rr", Name: "Refresh", OwnerID: "u1"})

	rates := []time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
		30 * time.Second,
		1 * time.Minute,
		0, // no refresh
	}

	for i, rate := range rates {
		svc.AddWidget("dash-rr", &Widget{
			ID:          fmt.Sprintf("w%d", i),
			Type:        WidgetMetric,
			Title:       fmt.Sprintf("Rate %v", rate),
			RefreshRate: rate,
		})
	}

	d, _ := svc.GetDashboard("dash-rr")
	for i, w := range d.Widgets {
		if w.RefreshRate != rates[i] {
			t.Errorf("widget %d: expected refresh rate %v, got %v", i, rates[i], w.RefreshRate)
		}
	}
}

// ============================================================================
// DASHBOARD LIFECYCLE INTEGRATION TEST
// ============================================================================

func TestDashboardLifecycle(t *testing.T) {
	svc := newTestService()

	// 1. Create dashboard
	d := &Dashboard{
		Name:        "Lifecycle Dashboard",
		Description: "Full lifecycle test",
		OwnerID:     "user-lifecycle",
	}
	mustCreateDashboard(t, svc, d)
	dashID := d.ID

	// 2. Add widgets
	widgets := []*Widget{
		{Type: WidgetChart, Title: "CPU", Source: "metrics/cpu", Position: Position{0, 0}, Size: Size{6, 4}},
		{Type: WidgetTable, Title: "Requests", Source: "metrics/requests", Position: Position{6, 0}, Size: Size{6, 4}},
		{Type: WidgetMetric, Title: "Uptime", Source: "metrics/uptime", Position: Position{0, 4}, Size: Size{12, 2}},
	}
	for _, w := range widgets {
		if err := svc.AddWidget(dashID, w); err != nil {
			t.Fatalf("AddWidget error: %v", err)
		}
	}

	// 3. Verify dashboard state
	got, ok := svc.GetDashboard(dashID)
	if !ok {
		t.Fatal("dashboard not found")
	}
	if len(got.Widgets) != 3 {
		t.Fatalf("expected 3 widgets, got %d", len(got.Widgets))
	}

	// 4. Remove a widget
	if err := svc.RemoveWidget(dashID, widgets[1].ID); err != nil {
		t.Fatalf("RemoveWidget error: %v", err)
	}
	got, _ = svc.GetDashboard(dashID)
	if len(got.Widgets) != 2 {
		t.Fatalf("expected 2 widgets after removal, got %d", len(got.Widgets))
	}

	// 5. Update dashboard
	drainBroadcasts(svc)
	updated := &Dashboard{
		ID:          dashID,
		Name:        "Updated Lifecycle",
		Description: "Updated description",
		OwnerID:     "user-lifecycle",
	}
	if err := svc.UpdateDashboard(updated); err != nil {
		t.Fatalf("UpdateDashboard error: %v", err)
	}

	// 6. Verify update broadcast
	broadcasts := drainBroadcasts(svc)
	if len(broadcasts) == 0 {
		t.Error("expected update broadcast")
	}

	// 7. Delete dashboard
	if err := svc.DeleteDashboard(dashID); err != nil {
		t.Fatalf("DeleteDashboard error: %v", err)
	}
	_, ok = svc.GetDashboard(dashID)
	if ok {
		t.Error("expected dashboard to be deleted")
	}
}

// ============================================================================
// SESSION LIFECYCLE INTEGRATION TEST
// ============================================================================

func TestSessionLifecycle(t *testing.T) {
	cfg := &Config{
		SessionTimeout:   1 * time.Hour,
		DefaultTheme:     "dark",
		DefaultLayout:    LayoutGrid,
		BroadcastBuffer:  100,
		MaxNotifications: 100,
	}
	svc := newTestServiceWithConfig(cfg)

	// 1. Create session
	sess, err := svc.CreateSession("user-lifecycle")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	// 2. Verify session is accessible
	got, ok := svc.GetSession(sess.ID)
	if !ok {
		t.Fatal("session not found")
	}
	if got.UserID != "user-lifecycle" {
		t.Errorf("expected UserID 'user-lifecycle', got %q", got.UserID)
	}

	// 3. Touch session
	originalActivity := sess.LastActivity
	time.Sleep(2 * time.Millisecond)
	if err := svc.TouchSession(sess.ID); err != nil {
		t.Fatalf("TouchSession error: %v", err)
	}
	got, _ = svc.GetSession(sess.ID)
	if !got.LastActivity.After(originalActivity) {
		t.Error("expected LastActivity to be updated after touch")
	}

	// 4. Destroy session
	if err := svc.DestroySession(sess.ID); err != nil {
		t.Fatalf("DestroySession error: %v", err)
	}
	_, ok = svc.GetSession(sess.ID)
	if ok {
		t.Error("expected session to be destroyed")
	}
}

// ============================================================================
// NOTIFICATION LIFECYCLE INTEGRATION TEST
// ============================================================================

func TestNotificationLifecycle(t *testing.T) {
	svc := newTestService()

	userID := "user-notif-lc"

	// 1. Add notifications of all types
	types := []NotificationType{NotifyInfo, NotifySuccess, NotifyWarning, NotifyError}
	for _, nt := range types {
		svc.AddNotification(&Notification{
			UserID:  userID,
			Type:    nt,
			Title:   fmt.Sprintf("%s notification", nt),
			Message: fmt.Sprintf("This is a %s", nt),
		})
	}
	drainBroadcasts(svc)

	// 2. Get all
	all := svc.GetNotifications(userID, false)
	if len(all) != 4 {
		t.Fatalf("expected 4 notifications, got %d", len(all))
	}

	// 3. All should be unread
	unread := svc.GetNotifications(userID, true)
	if len(unread) != 4 {
		t.Fatalf("expected 4 unread, got %d", len(unread))
	}

	// 4. Mark some as read
	svc.MarkNotificationRead(userID, all[0].ID)
	svc.MarkNotificationRead(userID, all[1].ID)

	// 5. Verify unread count
	unread = svc.GetNotifications(userID, true)
	if len(unread) != 2 {
		t.Errorf("expected 2 unread after marking 2 read, got %d", len(unread))
	}
}

// ============================================================================
// FULL HTTP HANDLER INTEGRATION
// ============================================================================

func TestHTTPHandler_FullIntegration(t *testing.T) {
	svc := newTestService()

	// Set up some data
	mustCreateDashboard(t, svc, &Dashboard{ID: "http-d1", Name: "HTTP Dash", OwnerID: "u1"})
	mustCreateSession(t, svc, "user-http")
	svc.AddNotification(&Notification{UserID: "user-http", Type: NotifyInfo, Title: "T", Message: "m"})
	drainBroadcasts(svc)

	handler := svc.HTTPHandler()

	endpoints := []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, "/", http.StatusOK},
		{http.MethodGet, "/api/dashboards", http.StatusOK},
		{http.MethodGet, "/api/sessions", http.StatusOK},
		{http.MethodGet, "/api/notifications", http.StatusOK},
	}

	for _, ep := range endpoints {
		t.Run(fmt.Sprintf("%s %s", ep.method, ep.path), func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != ep.status {
				t.Errorf("expected status %d, got %d", ep.status, rec.Code)
			}
		})
	}
}
