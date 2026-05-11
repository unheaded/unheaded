// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cloak provides user-facing dashboard capabilities.
// Cloak the User Dashboard is the Kingdom's outer garment - what users see and interact with.
// Implements dashboard rendering, user sessions, real-time updates, and UI state management.
package cloak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// Dashboard represents a user dashboard.
type Dashboard struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	OwnerID     string    `json:"owner_id"`
	Widgets     []*Widget `json:"widgets"`
	Layout      *Layout   `json:"layout"`
	Theme       string    `json:"theme,omitempty"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Widget represents a dashboard widget.
type Widget struct {
	ID          string                 `json:"id"`
	Type        WidgetType             `json:"type"`
	Title       string                 `json:"title"`
	Source      string                 `json:"source"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Position    Position               `json:"position"`
	Size        Size                   `json:"size"`
	RefreshRate time.Duration          `json:"refresh_rate,omitempty"`
}

// WidgetType defines widget types.
type WidgetType string

const (
	WidgetChart    WidgetType = "chart"
	WidgetTable    WidgetType = "table"
	WidgetMetric   WidgetType = "metric"
	WidgetLog      WidgetType = "log"
	WidgetAlert    WidgetType = "alert"
	WidgetTopology WidgetType = "topology"
	WidgetText     WidgetType = "text"
	WidgetIframe   WidgetType = "iframe"
)

// Position represents widget position.
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Size represents widget size.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Layout represents dashboard layout.
type Layout struct {
	Type      LayoutType `json:"type"`
	Columns   int        `json:"columns"`
	RowHeight int        `json:"row_height"`
	Margin    int        `json:"margin"`
}

// LayoutType defines layout types.
type LayoutType string

const (
	LayoutGrid     LayoutType = "grid"
	LayoutFreeform LayoutType = "freeform"
	LayoutFixed    LayoutType = "fixed"
)

// Session represents a user session.
type Session struct {
	ID           string                 `json:"id"`
	UserID       string                 `json:"user_id"`
	DashboardID  string                 `json:"dashboard_id,omitempty"`
	Theme        string                 `json:"theme"`
	Preferences  map[string]interface{} `json:"preferences,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	LastActivity time.Time              `json:"last_activity"`
	ExpiresAt    time.Time              `json:"expires_at"`
}

// Notification represents a user notification.
type Notification struct {
	ID        string           `json:"id"`
	UserID    string           `json:"user_id"`
	Type      NotificationType `json:"type"`
	Title     string           `json:"title"`
	Message   string           `json:"message"`
	Link      string           `json:"link,omitempty"`
	Read      bool             `json:"read"`
	CreatedAt time.Time        `json:"created_at"`
}

// NotificationType defines notification types.
type NotificationType string

const (
	NotifyInfo    NotificationType = "info"
	NotifySuccess NotificationType = "success"
	NotifyWarning NotificationType = "warning"
	NotifyError   NotificationType = "error"
)

// UIState represents client UI state.
type UIState struct {
	SessionID     string            `json:"session_id"`
	DashboardID   string            `json:"dashboard_id,omitempty"`
	ActiveWidgets []string          `json:"active_widgets,omitempty"`
	Filters       map[string]string `json:"filters,omitempty"`
	TimeRange     *TimeRange        `json:"time_range,omitempty"`
	Zoom          float64           `json:"zoom"`
}

// TimeRange represents a time range selection.
type TimeRange struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Preset string    `json:"preset,omitempty"` // "1h", "24h", "7d", "30d", "custom"
}

// Broadcast represents a real-time broadcast.
type Broadcast struct {
	Type    string      `json:"type"`
	Target  string      `json:"target"` // "all", "dashboard:{id}", "user:{id}"
	Payload interface{} `json:"payload"`
}

// Service is the main Cloak user dashboard service.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu            sync.RWMutex
	dashboards    map[string]*Dashboard
	sessions      map[string]*Session
	notifications map[string][]*Notification // userID -> notifications
	broadcasts    chan *Broadcast

	idCounter int64
}

// Config holds Cloak service configuration.
type Config struct {
	SessionTimeout   time.Duration `json:"session_timeout"`
	MaxNotifications int           `json:"max_notifications"`
	DefaultTheme     string        `json:"default_theme"`
	DefaultLayout    LayoutType    `json:"default_layout"`
	TemplateDir      string        `json:"template_dir"`
	StaticDir        string        `json:"static_dir"`
	BroadcastBuffer  int           `json:"broadcast_buffer"`
	WotanTopic       string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		SessionTimeout:   24 * time.Hour,
		MaxNotifications: 100,
		DefaultTheme:     "dark",
		DefaultLayout:    LayoutGrid,
		TemplateDir:      "./templates",
		StaticDir:        "./static",
		BroadcastBuffer:  1000,
		WotanTopic:       "cloak.dashboard",
	}
}

// NewService creates a new Cloak user dashboard service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:           log,
		wotan:         wotan,
		config:        cfg,
		dashboards:    make(map[string]*Dashboard),
		sessions:      make(map[string]*Session),
		notifications: make(map[string][]*Notification),
		broadcasts:    make(chan *Broadcast, cfg.BroadcastBuffer),
	}
}

// Start initializes the Cloak service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Cloak donned - user dashboard active")
	go s.broadcastLoop(ctx)
	go s.sessionCleanupLoop(ctx)
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Cloak removed - user dashboard suspended")
	close(s.broadcasts)
	return nil
}

// CreateDashboard creates a new dashboard.
func (s *Service) CreateDashboard(dashboard *Dashboard) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if dashboard.ID == "" {
		dashboard.ID = fmt.Sprintf("dash-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&s.idCounter, 1))
	}

	now := time.Now()
	dashboard.CreatedAt = now
	dashboard.UpdatedAt = now

	if dashboard.Layout == nil {
		dashboard.Layout = &Layout{
			Type:      s.config.DefaultLayout,
			Columns:   12,
			RowHeight: 100,
			Margin:    10,
		}
	}

	if dashboard.Theme == "" {
		dashboard.Theme = s.config.DefaultTheme
	}

	s.dashboards[dashboard.ID] = dashboard

	s.log.Info().Str("id", dashboard.ID).Str("name", dashboard.Name).Msg("Dashboard created")
	return nil
}

// GetDashboard retrieves a dashboard by ID.
func (s *Service) GetDashboard(dashboardID string) (*Dashboard, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dashboard, ok := s.dashboards[dashboardID]
	return dashboard, ok
}

// UpdateDashboard updates a dashboard.
func (s *Service) UpdateDashboard(dashboard *Dashboard) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.dashboards[dashboard.ID]; !ok {
		return fmt.Errorf("dashboard not found: %s", dashboard.ID)
	}

	dashboard.UpdatedAt = time.Now()
	s.dashboards[dashboard.ID] = dashboard

	// Notify connected clients
	s.broadcasts <- &Broadcast{
		Type:    "dashboard.updated",
		Target:  fmt.Sprintf("dashboard:%s", dashboard.ID),
		Payload: dashboard,
	}

	return nil
}

// DeleteDashboard deletes a dashboard.
func (s *Service) DeleteDashboard(dashboardID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.dashboards[dashboardID]; !ok {
		return fmt.Errorf("dashboard not found: %s", dashboardID)
	}

	delete(s.dashboards, dashboardID)
	return nil
}

// ListDashboards returns dashboards for a user.
func (s *Service) ListDashboards(ownerID string) []*Dashboard {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Dashboard
	for _, d := range s.dashboards {
		if ownerID == "" || d.OwnerID == ownerID {
			results = append(results, d)
		}
	}
	return results
}

// AddWidget adds a widget to a dashboard.
func (s *Service) AddWidget(dashboardID string, widget *Widget) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dashboard, ok := s.dashboards[dashboardID]
	if !ok {
		return fmt.Errorf("dashboard not found: %s", dashboardID)
	}

	if widget.ID == "" {
		widget.ID = fmt.Sprintf("widget-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&s.idCounter, 1))
	}

	dashboard.Widgets = append(dashboard.Widgets, widget)
	dashboard.UpdatedAt = time.Now()

	s.log.Debug().Str("dashboard", dashboardID).Str("widget", widget.ID).Msg("Widget added")
	return nil
}

// RemoveWidget removes a widget from a dashboard.
func (s *Service) RemoveWidget(dashboardID, widgetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dashboard, ok := s.dashboards[dashboardID]
	if !ok {
		return fmt.Errorf("dashboard not found: %s", dashboardID)
	}

	for i, w := range dashboard.Widgets {
		if w.ID == widgetID {
			dashboard.Widgets = append(dashboard.Widgets[:i], dashboard.Widgets[i+1:]...)
			dashboard.UpdatedAt = time.Now()
			return nil
		}
	}

	return fmt.Errorf("widget not found: %s", widgetID)
}

// CreateSession creates a new session.
func (s *Service) CreateSession(userID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	session := &Session{
		ID:           fmt.Sprintf("sess-%d-%d", now.UnixNano(), atomic.AddInt64(&s.idCounter, 1)),
		UserID:       userID,
		Theme:        s.config.DefaultTheme,
		Preferences:  make(map[string]interface{}),
		CreatedAt:    now,
		LastActivity: now,
		ExpiresAt:    now.Add(s.config.SessionTimeout),
	}

	s.sessions[session.ID] = session

	s.log.Debug().Str("session", session.ID).Str("user", userID).Msg("Session created")
	return session, nil
}

// GetSession retrieves a session.
func (s *Service) GetSession(sessionID string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, false
	}

	// Check expiration
	if time.Now().After(session.ExpiresAt) {
		return nil, false
	}

	return session, true
}

// TouchSession updates session activity.
func (s *Service) TouchSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.LastActivity = time.Now()
	session.ExpiresAt = time.Now().Add(s.config.SessionTimeout)
	return nil
}

// DestroySession destroys a session.
func (s *Service) DestroySession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionID)
	return nil
}

// AddNotification adds a notification for a user.
func (s *Service) AddNotification(notification *Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if notification.ID == "" {
		notification.ID = fmt.Sprintf("notif-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&s.idCounter, 1))
	}
	notification.CreatedAt = time.Now()

	// Add to user's notifications
	userNotifs := s.notifications[notification.UserID]
	userNotifs = append([]*Notification{notification}, userNotifs...)

	// Limit notifications
	if len(userNotifs) > s.config.MaxNotifications {
		userNotifs = userNotifs[:s.config.MaxNotifications]
	}

	s.notifications[notification.UserID] = userNotifs

	// Broadcast notification
	s.broadcasts <- &Broadcast{
		Type:    "notification.new",
		Target:  fmt.Sprintf("user:%s", notification.UserID),
		Payload: notification,
	}

	return nil
}

// GetNotifications returns notifications for a user.
func (s *Service) GetNotifications(userID string, unreadOnly bool) []*Notification {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notifs := s.notifications[userID]
	if !unreadOnly {
		return notifs
	}

	var unread []*Notification
	for _, n := range notifs {
		if !n.Read {
			unread = append(unread, n)
		}
	}
	return unread
}

// MarkNotificationRead marks a notification as read.
func (s *Service) MarkNotificationRead(userID, notificationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	notifs := s.notifications[userID]
	for _, n := range notifs {
		if n.ID == notificationID {
			n.Read = true
			return nil
		}
	}

	return fmt.Errorf("notification not found: %s", notificationID)
}

// Broadcast sends a broadcast message.
func (s *Service) Broadcast(b *Broadcast) {
	select {
	case s.broadcasts <- b:
	default:
		s.log.Warn().Str("type", b.Type).Msg("Broadcast buffer full, dropping message")
	}
}

// HTTPHandler returns the HTTP handler for the dashboard.
func (s *Service) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// Health, ready, and metrics endpoints (required by CLAUDE.md)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)

	// Dashboard API
	mux.HandleFunc("/api/dashboards", s.handleDashboards)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/notifications", s.handleNotifications)

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.config.StaticDir))))

	// Dashboard view
	mux.HandleFunc("/", s.handleIndex)

	return mux
}

// handleHealth returns health status for the cloak service.
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "cloak",
	})
}

// handleReady returns readiness status for the cloak service.
func (s *Service) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":   true,
		"service": "cloak",
	})
}

// handleMetrics returns Prometheus-style metrics for the cloak service.
func (s *Service) handleMetrics(w http.ResponseWriter, r *http.Request) {
	stats := s.Stats()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP cloak_dashboards_total Total dashboards\n")
	fmt.Fprintf(w, "# TYPE cloak_dashboards_total gauge\n")
	fmt.Fprintf(w, "cloak_dashboards_total %v\n", stats["total_dashboards"])
	fmt.Fprintf(w, "# HELP cloak_active_sessions Total active sessions\n")
	fmt.Fprintf(w, "# TYPE cloak_active_sessions gauge\n")
	fmt.Fprintf(w, "cloak_active_sessions %v\n", stats["active_sessions"])
	fmt.Fprintf(w, "# HELP cloak_notifications_total Total notifications\n")
	fmt.Fprintf(w, "# TYPE cloak_notifications_total gauge\n")
	fmt.Fprintf(w, "cloak_notifications_total %v\n", stats["total_notifications"])
	fmt.Fprintf(w, "# HELP cloak_users_with_notifications Users with notifications\n")
	fmt.Fprintf(w, "# TYPE cloak_users_with_notifications gauge\n")
	fmt.Fprintf(w, "cloak_users_with_notifications %v\n", stats["users_with_notifications"])
}

func (s *Service) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// In production, would serve template
	_, _ = w.Write([]byte("<!DOCTYPE html><html><head><title>Unheaded Kingdom Dashboard</title></head><body><h1>The Cloak Awaits</h1></body></html>"))
}

func (s *Service) handleDashboards(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.ListDashboards(""))
}

func (s *Service) handleSessions(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	count := len(s.sessions)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active_sessions": count,
	})
}

func (s *Service) handleNotifications(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Would get user from auth context
	json.NewEncoder(w).Encode([]interface{}{})
}

func (s *Service) broadcastLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case b, ok := <-s.broadcasts:
			if !ok {
				return
			}
			// In production, would send to connected WebSocket clients
			s.log.Debug().Str("type", b.Type).Str("target", b.Target).Msg("Broadcasting")
		}
	}
}

func (s *Service) sessionCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupExpiredSessions()
		}
	}
}

func (s *Service) cleanupExpiredSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, id)
			s.log.Debug().Str("session", id).Msg("Session expired and cleaned up")
		}
	}
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalNotifications := 0
	for _, notifs := range s.notifications {
		totalNotifications += len(notifs)
	}

	return map[string]interface{}{
		"total_dashboards":         len(s.dashboards),
		"active_sessions":          len(s.sessions),
		"total_notifications":      totalNotifications,
		"users_with_notifications": len(s.notifications),
	}
}
