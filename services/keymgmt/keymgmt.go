// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package keymgmt provides the Key Management Service for PQC authentication.
// This service manages the lifecycle of post-quantum cryptographic keys:
// generation, rotation (with 60-second grace periods), and revocation.
// Keys are stored in Sophia BPF maps and referenced by 2-byte KeyRefs
// in the frozen Monad wire format.
//
// Port: 19007 (Doom Range)
// Wotan topics: pqc.key.generated, pqc.key.rotated, pqc.key.revoked
package keymgmt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
)

// KeyStatus represents the lifecycle state of a key.
type KeyStatus uint8

const (
	// KeyActive means the key is valid for signing and verification.
	KeyActive KeyStatus = iota + 1
	// KeyRotating means a new key has been generated; both old and new
	// are valid during the grace period.
	KeyRotating
	// KeyRevoked means the key has been immediately invalidated.
	KeyRevoked
	// KeyExpired means the key's TTL has elapsed.
	KeyExpired
)

// String returns a human-readable name.
func (s KeyStatus) String() string {
	switch s {
	case KeyActive:
		return "active"
	case KeyRotating:
		return "rotating"
	case KeyRevoked:
		return "revoked"
	case KeyExpired:
		return "expired"
	default:
		return "unknown"
	}
}

// KeyRecord holds metadata for a managed key.
type KeyRecord struct {
	ID           string    `json:"id"`
	AlgoID       uint8     `json:"algo_id"`
	ParamSet     uint8     `json:"param_set"`
	KeyRef       uint16    `json:"key_ref"`
	ServiceID    uint16    `json:"service_id"`
	Status       KeyStatus `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	RotatedAt    *time.Time `json:"rotated_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	ReplacedBy   string    `json:"replaced_by,omitempty"`
	PublicKeyHex string    `json:"public_key_hex"`
}

// Config holds keymgmt service configuration.
type Config struct {
	ListenAddr       string        `json:"listen_addr"`
	DefaultTTL       time.Duration `json:"default_ttl"`
	GracePeriod      time.Duration `json:"grace_period"`
	MaxKeysPerService int          `json:"max_keys_per_service"`
	WotanTopic       string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ListenAddr:       ports.DefaultAddr(ports.KeyMgmt),
		DefaultTTL:       24 * time.Hour,
		GracePeriod:      60 * time.Second,
		MaxKeysPerService: 64,
		WotanTopic:       "pqc.key",
	}
}

// Service is the Key Management service.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu       sync.RWMutex
	keys     map[string]*KeyRecord   // id → record
	byKeyRef map[uint16]*KeyRecord   // keyRef → record
	bySvcID  map[uint16][]*KeyRecord // serviceID → records

	nextKeyRef uint32 // atomic
	server     *http.Server

	// Degraded mode
	wotanDrops int64

	// Grace period timers
	graceTimers sync.Map // keyID → *time.Timer
}

// WotanDrops returns dropped message count.
func (s *Service) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// NewService creates a new keymgmt service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, config *Config) *Service {
	if config == nil {
		config = DefaultConfig()
	}
	return &Service{
		log:      log,
		wotan:    wotan,
		config:   config,
		keys:     make(map[string]*KeyRecord),
		byKeyRef: make(map[uint16]*KeyRecord),
		bySvcID:  make(map[uint16][]*KeyRecord),
	}
}

// Start initializes the keymgmt service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("KeyMgmt service starting — PQC key lifecycle management")

	// Subscribe to Wotan
	if s.wotan != nil {
		if _, err := s.wotan.Subscribe(ctx, "alerts.critical", "keymgmt-service"); err != nil {
			s.log.Warn().Err(err).Msg("failed to subscribe to alerts.critical")
		}
		go s.listenForAlerts(ctx)
	}

	// Start HTTP server
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.server = &http.Server{
		Addr:              s.config.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		s.log.Info().Str("addr", s.config.ListenAddr).Msg("keymgmt HTTP server starting")
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("keymgmt HTTP server error")
		}
	}()

	// Start expiration checker
	go s.expirationLoop(ctx)

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("KeyMgmt service stopping — keys preserved")

	// Cancel all grace period timers
	s.graceTimers.Range(func(key, value interface{}) bool {
		if timer, ok := value.(*time.Timer); ok {
			timer.Stop()
		}
		return true
	})

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// GenerateKey creates a new PQC key pair.
func (s *Service) GenerateKey(algoID, paramSet uint8, serviceID uint16) (*KeyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check per-service limit
	if len(s.bySvcID[serviceID]) >= s.config.MaxKeysPerService {
		return nil, fmt.Errorf("keymgmt: service %d has reached max key limit (%d)", serviceID, s.config.MaxKeysPerService)
	}

	// Allocate KeyRef
	ref := uint16(atomic.AddUint32(&s.nextKeyRef, 1))

	now := time.Now()
	record := &KeyRecord{
		ID:        fmt.Sprintf("key-%d-%d", now.UnixNano(), ref),
		AlgoID:    algoID,
		ParamSet:  paramSet,
		KeyRef:    ref,
		ServiceID: serviceID,
		Status:    KeyActive,
		CreatedAt: now,
		ExpiresAt: now.Add(s.config.DefaultTTL),
	}

	s.keys[record.ID] = record
	s.byKeyRef[ref] = record
	s.bySvcID[serviceID] = append(s.bySvcID[serviceID], record)

	s.log.Info().
		Str("key_id", record.ID).
		Uint16("key_ref", ref).
		Uint8("algo_id", algoID).
		Uint16("service_id", serviceID).
		Msg("PQC key generated")

	s.publishEvent(context.Background(), "pqc.key.generated", map[string]interface{}{
		"key_id":     record.ID,
		"key_ref":    ref,
		"algo_id":    algoID,
		"service_id": serviceID,
	})

	return record, nil
}

// RotateKey generates a new key to replace an existing one.
// Both old and new keys are valid during the grace period.
func (s *Service) RotateKey(keyID string) (*KeyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, ok := s.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("keymgmt: key not found: %s", keyID)
	}
	if old.Status != KeyActive {
		return nil, fmt.Errorf("keymgmt: cannot rotate key in status %s", old.Status)
	}

	// Mark old key as rotating
	now := time.Now()
	old.Status = KeyRotating
	old.RotatedAt = &now

	// Generate replacement
	ref := uint16(atomic.AddUint32(&s.nextKeyRef, 1))
	newRecord := &KeyRecord{
		ID:        fmt.Sprintf("key-%d-%d", now.UnixNano(), ref),
		AlgoID:    old.AlgoID,
		ParamSet:  old.ParamSet,
		KeyRef:    ref,
		ServiceID: old.ServiceID,
		Status:    KeyActive,
		CreatedAt: now,
		ExpiresAt: now.Add(s.config.DefaultTTL),
	}
	old.ReplacedBy = newRecord.ID

	s.keys[newRecord.ID] = newRecord
	s.byKeyRef[ref] = newRecord
	s.bySvcID[old.ServiceID] = append(s.bySvcID[old.ServiceID], newRecord)

	// Start grace period timer — old key expires after grace period
	timer := time.AfterFunc(s.config.GracePeriod, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if old.Status == KeyRotating {
			old.Status = KeyExpired
			s.log.Info().Str("key_id", keyID).Msg("grace period ended — old key expired")
		}
		s.graceTimers.Delete(keyID)
	})
	s.graceTimers.Store(keyID, timer)

	s.log.Info().
		Str("old_key_id", keyID).
		Str("new_key_id", newRecord.ID).
		Dur("grace_period", s.config.GracePeriod).
		Msg("PQC key rotated — grace period started")

	s.publishEvent(context.Background(), "pqc.key.rotated", map[string]interface{}{
		"old_key_id": keyID,
		"new_key_id": newRecord.ID,
		"key_ref":    ref,
		"grace_s":    s.config.GracePeriod.Seconds(),
	})

	return newRecord, nil
}

// RevokeKey immediately invalidates a key and cascades to dependent SigRefs.
func (s *Service) RevokeKey(keyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.keys[keyID]
	if !ok {
		return fmt.Errorf("keymgmt: key not found: %s", keyID)
	}
	if record.Status == KeyRevoked {
		return fmt.Errorf("keymgmt: key already revoked: %s", keyID)
	}

	now := time.Now()
	record.Status = KeyRevoked
	record.RevokedAt = &now

	// Cancel any pending grace timer
	if timer, ok := s.graceTimers.LoadAndDelete(keyID); ok {
		timer.(*time.Timer).Stop()
	}

	s.log.Warn().
		Str("key_id", keyID).
		Uint16("key_ref", record.KeyRef).
		Msg("PQC key revoked — immediate invalidation")

	s.publishEvent(context.Background(), "pqc.key.revoked", map[string]interface{}{
		"key_id":  keyID,
		"key_ref": record.KeyRef,
	})

	return nil
}

// GetKey retrieves a key record by ID.
func (s *Service) GetKey(keyID string) (*KeyRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.keys[keyID]
	if !ok {
		return nil, false
	}
	copy := *r
	return &copy, true
}

// GetKeyByRef retrieves a key record by KeyRef.
func (s *Service) GetKeyByRef(keyRef uint16) (*KeyRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.byKeyRef[keyRef]
	return r, ok
}

// ListKeys returns all keys for a service.
func (s *Service) ListKeys(serviceID uint16) []*KeyRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bySvcID[serviceID]
}

// ActiveKeyCount returns the number of active keys.
func (s *Service) ActiveKeyCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, r := range s.keys {
		if r.Status == KeyActive || r.Status == KeyRotating {
			count++
		}
	}
	return count
}

// registerRoutes sets up HTTP endpoints.
func (s *Service) registerRoutes(mux *http.ServeMux) {
	// Health/ready/metrics (required by CLAUDE.md)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "keymgmt",
		})
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":   true,
			"service": "keymgmt",
		})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		s.mu.RLock()
		totalKeys := len(s.keys)
		activeKeys := 0
		revokedKeys := 0
		for _, rec := range s.keys {
			switch rec.Status {
			case KeyActive, KeyRotating:
				activeKeys++
			case KeyRevoked:
				revokedKeys++
			}
		}
		s.mu.RUnlock()
		fmt.Fprintf(w, "# HELP keymgmt_keys_total Total managed keys\n")
		fmt.Fprintf(w, "# TYPE keymgmt_keys_total gauge\n")
		fmt.Fprintf(w, "keymgmt_keys_total %d\n", totalKeys)
		fmt.Fprintf(w, "# HELP keymgmt_keys_active Active keys\n")
		fmt.Fprintf(w, "# TYPE keymgmt_keys_active gauge\n")
		fmt.Fprintf(w, "keymgmt_keys_active %d\n", activeKeys)
		fmt.Fprintf(w, "# HELP keymgmt_keys_revoked Revoked keys\n")
		fmt.Fprintf(w, "# TYPE keymgmt_keys_revoked counter\n")
		fmt.Fprintf(w, "keymgmt_keys_revoked %d\n", revokedKeys)
		fmt.Fprintf(w, "# HELP keymgmt_wotan_drops Messages dropped (Wotan unavailable)\n")
		fmt.Fprintf(w, "# TYPE keymgmt_wotan_drops counter\n")
		fmt.Fprintf(w, "keymgmt_wotan_drops %d\n", s.WotanDrops())
	})

	// API endpoints
	mux.HandleFunc("/api/v1/keys", s.handleKeys)
	mux.HandleFunc("/api/v1/keys/", s.handleKeyByID)
}

// handleKeys handles POST /api/v1/keys (generate).
func (s *Service) handleKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AlgoID    uint8  `json:"algo_id"`
		ParamSet  uint8  `json:"param_set"`
		ServiceID uint16 `json:"service_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	record, err := s.GenerateKey(req.AlgoID, req.ParamSet, req.ServiceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(record)
}

// handleKeyByID handles key operations by ID.
func (s *Service) handleKeyByID(w http.ResponseWriter, r *http.Request) {
	// Extract key ID from path: /api/v1/keys/{id}[/action]
	path := r.URL.Path[len("/api/v1/keys/"):]

	switch r.Method {
	case http.MethodGet:
		record, ok := s.GetKey(path)
		if !ok {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(record)

	case http.MethodPost:
		// Check for action suffix
		if len(path) > 0 {
			// Find the key ID and action
			keyID := path
			action := ""
			for i := len(path) - 1; i >= 0; i-- {
				if path[i] == '/' {
					keyID = path[:i]
					action = path[i+1:]
					break
				}
			}

			switch action {
			case "rotate":
				newRecord, err := s.RotateKey(keyID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(newRecord)
				return

			case "revoke":
				if err := s.RevokeKey(keyID); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		http.Error(w, "unknown action", http.StatusBadRequest)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listenForAlerts handles critical alerts from Wotan.
func (s *Service) listenForAlerts(ctx context.Context) {
	if s.wotan == nil {
		return
	}

	msgCh, err := s.wotan.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to stream alerts.critical")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			s.log.Warn().
				Str("message_id", msg.MessageID).
				Msg("keymgmt received critical alert")
		}
	}
}

// expirationLoop checks for expired keys.
func (s *Service) expirationLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkExpirations()
		}
	}
}

// checkExpirations marks expired keys.
func (s *Service) checkExpirations() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, record := range s.keys {
		if record.Status == KeyActive && now.After(record.ExpiresAt) {
			record.Status = KeyExpired
			s.log.Info().Str("key_id", record.ID).Msg("key expired")
		}
	}
}

// publishEvent sends a PQC key event to Wotan.
func (s *Service) publishEvent(ctx context.Context, topic string, data map[string]interface{}) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		return
	}

	event := map[string]interface{}{
		"event_type": topic,
		"timestamp":  time.Now().UnixMilli(),
		"data":       data,
		"service":    "keymgmt",
	}

	payload, err := json.Marshal(event)
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to marshal event")
		return
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.wotan.Publish(pubCtx, topic, payload); err != nil {
		s.log.Warn().Err(err).Msg("failed to publish event")
	}
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statusCounts := make(map[string]int)
	algoCounts := make(map[uint8]int)
	for _, r := range s.keys {
		statusCounts[r.Status.String()]++
		algoCounts[r.AlgoID]++
	}

	return map[string]interface{}{
		"total_keys":     len(s.keys),
		"by_status":      statusCounts,
		"by_algorithm":   algoCounts,
		"wotan_drops":    s.WotanDrops(),
	}
}
