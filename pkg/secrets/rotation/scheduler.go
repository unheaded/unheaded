// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rotation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/secrets"
)

// Scheduler manages automatic secret rotation on a schedule.
type Scheduler struct {
	manager   *RotationManager
	store     secrets.SecretStore
	schedules map[string]*Schedule
	log       *logger.Logger
	mu        sync.RWMutex
	stopCh    chan struct{}
	wg        sync.WaitGroup
	running   bool

	// State persistence
	stateFile string
}

// Package-level scheduler metrics (registered once)
var (
	schedulerMetricsOnce   sync.Once
	scheduledRotations     *metrics.GaugeVec
	nextRotationTimestamp  *metrics.GaugeVec
)

// Schedule defines when and how to rotate a secret.
type Schedule struct {
	// Path is the secret path.
	Path string `json:"path"`

	// Interval is how often to rotate.
	Interval time.Duration `json:"interval"`

	// Config is the rotation configuration.
	Config RotationConfig `json:"config"`

	// LastRotation is when the secret was last rotated.
	LastRotation time.Time `json:"last_rotation"`

	// NextRotation is when the next rotation is scheduled.
	NextRotation time.Time `json:"next_rotation"`

	// Enabled indicates if this schedule is active.
	Enabled bool `json:"enabled"`

	// FailedAttempts is the number of consecutive failed rotations.
	FailedAttempts int `json:"failed_attempts"`

	// MaxFailedAttempts is the maximum failed attempts before disabling.
	MaxFailedAttempts int `json:"max_failed_attempts"`

	// NotifyOnFailure indicates if notifications should be sent on failure.
	NotifyOnFailure bool `json:"notify_on_failure"`

	// RotateBeforeExpiry is how long before expiry to trigger rotation.
	RotateBeforeExpiry time.Duration `json:"rotate_before_expiry"`
}

// SchedulerConfig configures the scheduler.
type SchedulerConfig struct {
	// Manager is the rotation manager.
	Manager *RotationManager

	// Store is the secret store.
	Store secrets.SecretStore

	// Logger is the logger to use.
	Logger *logger.Logger

	// StateFile is the path to persist scheduler state.
	StateFile string

	// CheckInterval is how often to check for due rotations.
	CheckInterval time.Duration
}

// NewScheduler creates a new rotation scheduler.
func NewScheduler(config SchedulerConfig) (*Scheduler, error) {
	if config.Manager == nil {
		return nil, fmt.Errorf("rotation manager is required")
	}
	if config.Store == nil {
		return nil, fmt.Errorf("secret store is required")
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	s := &Scheduler{
		manager:   config.Manager,
		store:     config.Store,
		schedules: make(map[string]*Schedule),
		log:       log.With().Str("component", "rotation-scheduler").Logger(),
		stopCh:    make(chan struct{}),
		stateFile: config.StateFile,
	}

	// Initialize metrics (only once per process)
	schedulerMetricsOnce.Do(func() {
		scheduledRotations = metrics.NewGaugeVec(
			"unheaded_secrets_scheduled_rotations",
			"Number of scheduled rotations",
			nil,
			[]string{"status"},
		)
		_ = metrics.Register(scheduledRotations)

		nextRotationTimestamp = metrics.NewGaugeVec(
			"unheaded_secrets_next_rotation_timestamp",
			"Timestamp of next scheduled rotation",
			nil,
			[]string{"path"},
		)
		_ = metrics.Register(nextRotationTimestamp)
	})

	// Load state if file exists
	if config.StateFile != "" {
		if err := s.loadState(); err != nil {
			s.log.Warn().Err(err).Msg("failed to load scheduler state")
		}
	}

	return s, nil
}

// AddSchedule adds a rotation schedule.
func (s *Scheduler) AddSchedule(schedule *Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if schedule.Path == "" {
		return fmt.Errorf("schedule path is required")
	}
	if schedule.Interval <= 0 {
		return fmt.Errorf("schedule interval must be positive")
	}

	// Set defaults
	if schedule.MaxFailedAttempts <= 0 {
		schedule.MaxFailedAttempts = 3
	}

	// Calculate next rotation
	if schedule.NextRotation.IsZero() {
		if schedule.LastRotation.IsZero() {
			// First rotation - check secret expiry
			if schedule.RotateBeforeExpiry > 0 {
				secret, err := s.store.Get(context.Background(), schedule.Path)
				if err == nil && !secret.ExpiresAt.IsZero() {
					rotateAt := secret.ExpiresAt.Add(-schedule.RotateBeforeExpiry)
					if rotateAt.After(time.Now()) {
						schedule.NextRotation = rotateAt
					} else {
						schedule.NextRotation = time.Now().Add(time.Minute)
					}
				} else {
					schedule.NextRotation = time.Now().Add(schedule.Interval)
				}
			} else {
				schedule.NextRotation = time.Now().Add(schedule.Interval)
			}
		} else {
			schedule.NextRotation = schedule.LastRotation.Add(schedule.Interval)
		}
	}

	s.schedules[schedule.Path] = schedule

	s.log.Info().
		Str("path", schedule.Path).
		Dur("interval", schedule.Interval).
		Time("next_rotation", schedule.NextRotation).
		Msg("schedule added")

	s.updateMetrics()
	s.saveState()

	return nil
}

// RemoveSchedule removes a rotation schedule.
func (s *Scheduler) RemoveSchedule(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.schedules, path)
	s.log.Info().Str("path", path).Msg("schedule removed")

	s.updateMetrics()
	s.saveState()
}

// GetSchedule returns a schedule by path.
func (s *Scheduler) GetSchedule(path string) (*Schedule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schedule, ok := s.schedules[path]
	return schedule, ok
}

// ListSchedules returns all schedules.
func (s *Scheduler) ListSchedules() []*Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schedules := make([]*Schedule, 0, len(s.schedules))
	for _, schedule := range s.schedules {
		schedules = append(schedules, schedule)
	}
	return schedules
}

// Start starts the scheduler.
func (s *Scheduler) Start(checkInterval time.Duration) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	if checkInterval <= 0 {
		checkInterval = time.Minute
	}

	s.wg.Add(1)
	go s.run(checkInterval)

	s.log.Info().Dur("check_interval", checkInterval).Msg("scheduler started")
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()

	s.wg.Wait()
	s.saveState()

	s.log.Info().Msg("scheduler stopped")
}

// run is the main scheduler loop.
func (s *Scheduler) run(checkInterval time.Duration) {
	defer s.wg.Done()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// Initial check
	s.checkRotations()

	for {
		select {
		case <-ticker.C:
			s.checkRotations()
		case <-s.stopCh:
			return
		}
	}
}

// checkRotations checks for due rotations and executes them.
func (s *Scheduler) checkRotations() {
	s.mu.Lock()
	now := time.Now()
	dueSchedules := make([]*Schedule, 0)

	for _, schedule := range s.schedules {
		if schedule.Enabled && now.After(schedule.NextRotation) {
			dueSchedules = append(dueSchedules, schedule)
		}
	}
	s.mu.Unlock()

	for _, schedule := range dueSchedules {
		s.executeRotation(schedule)
	}
}

// executeRotation executes a scheduled rotation.
func (s *Scheduler) executeRotation(schedule *Schedule) {
	ctx := context.Background()

	s.log.Info().
		Str("path", schedule.Path).
		Time("scheduled_for", schedule.NextRotation).
		Msg("executing scheduled rotation")

	result, err := s.manager.Rotate(ctx, schedule.Path, schedule.Config)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err != nil || (result != nil && !result.Success) {
		schedule.FailedAttempts++

		errMsg := "unknown error"
		if err != nil {
			errMsg = err.Error()
		} else if result != nil && result.Error != nil {
			errMsg = result.Error.Error()
		}

		s.log.Error().
			Str("path", schedule.Path).
			Int("failed_attempts", schedule.FailedAttempts).
			Str("error", errMsg).
			Msg("scheduled rotation failed")

		if schedule.FailedAttempts >= schedule.MaxFailedAttempts {
			schedule.Enabled = false
			s.log.Warn().
				Str("path", schedule.Path).
				Int("max_attempts", schedule.MaxFailedAttempts).
				Msg("schedule disabled due to repeated failures")
		}

		// Retry sooner on failure
		schedule.NextRotation = time.Now().Add(schedule.Interval / 4)
		if schedule.NextRotation.Sub(time.Now()) < time.Minute {
			schedule.NextRotation = time.Now().Add(time.Minute)
		}
	} else {
		schedule.FailedAttempts = 0
		schedule.LastRotation = time.Now()

		// Calculate next rotation based on new secret expiry if configured
		if schedule.RotateBeforeExpiry > 0 && result.NewSecret != nil && !result.NewSecret.ExpiresAt.IsZero() {
			schedule.NextRotation = result.NewSecret.ExpiresAt.Add(-schedule.RotateBeforeExpiry)
		} else {
			schedule.NextRotation = schedule.LastRotation.Add(schedule.Interval)
		}

		s.log.Info().
			Str("path", schedule.Path).
			Time("next_rotation", schedule.NextRotation).
			Msg("scheduled rotation successful")
	}

	s.updateMetrics()
	s.saveState()
}

// updateMetrics updates scheduler metrics.
func (s *Scheduler) updateMetrics() {
	enabled := 0
	disabled := 0

	for _, schedule := range s.schedules {
		if schedule.Enabled {
			enabled++
			nextRotationTimestamp.WithLabels(metrics.Labels{"path": schedule.Path}).Set(float64(schedule.NextRotation.Unix()))
		} else {
			disabled++
		}
	}

	scheduledRotations.WithLabels(metrics.Labels{"status": "enabled"}).Set(float64(enabled))
	scheduledRotations.WithLabels(metrics.Labels{"status": "disabled"}).Set(float64(disabled))
}

// loadState loads scheduler state from file.
func (s *Scheduler) loadState() error {
	if s.stateFile == "" {
		return nil
	}

	data, err := os.ReadFile(s.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var state schedulerState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	for _, schedule := range state.Schedules {
		s.schedules[schedule.Path] = schedule
	}

	s.log.Info().
		Int("schedules", len(state.Schedules)).
		Msg("loaded scheduler state")

	return nil
}

// saveState saves scheduler state to file.
func (s *Scheduler) saveState() error {
	if s.stateFile == "" {
		return nil
	}

	schedules := make([]*Schedule, 0, len(s.schedules))
	for _, schedule := range s.schedules {
		schedules = append(schedules, schedule)
	}

	state := schedulerState{
		Schedules: schedules,
		UpdatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.stateFile, data, 0600)
}

type schedulerState struct {
	Schedules []*Schedule `json:"schedules"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// TriggerRotation manually triggers a rotation for a scheduled secret.
func (s *Scheduler) TriggerRotation(path string) (*RotationResult, error) {
	s.mu.RLock()
	schedule, ok := s.schedules[path]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no schedule found for path: %s", path)
	}

	return s.manager.Rotate(context.Background(), path, schedule.Config)
}

// EnableSchedule enables a disabled schedule.
func (s *Scheduler) EnableSchedule(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, ok := s.schedules[path]
	if !ok {
		return fmt.Errorf("no schedule found for path: %s", path)
	}

	schedule.Enabled = true
	schedule.FailedAttempts = 0

	s.log.Info().Str("path", path).Msg("schedule enabled")
	s.updateMetrics()
	s.saveState()

	return nil
}

// DisableSchedule disables a schedule.
func (s *Scheduler) DisableSchedule(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, ok := s.schedules[path]
	if !ok {
		return fmt.Errorf("no schedule found for path: %s", path)
	}

	schedule.Enabled = false

	s.log.Info().Str("path", path).Msg("schedule disabled")
	s.updateMetrics()
	s.saveState()

	return nil
}

// NextDueRotation returns the next rotation that will occur.
func (s *Scheduler) NextDueRotation() (*Schedule, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var nextSchedule *Schedule
	var nextTime time.Time

	for _, schedule := range s.schedules {
		if !schedule.Enabled {
			continue
		}
		if nextSchedule == nil || schedule.NextRotation.Before(nextTime) {
			nextSchedule = schedule
			nextTime = schedule.NextRotation
		}
	}

	return nextSchedule, nextTime
}

// CheckExpiringSecrets checks for secrets that will expire soon.
func (s *Scheduler) CheckExpiringSecrets(ctx context.Context, threshold time.Duration) ([]*secrets.Secret, error) {
	// List all secrets
	paths, err := s.store.List(ctx, "")
	if err != nil {
		return nil, err
	}

	var expiring []*secrets.Secret
	now := time.Now()

	for _, path := range paths {
		secret, err := s.store.Get(ctx, path)
		if err != nil {
			continue
		}

		if !secret.ExpiresAt.IsZero() && secret.ExpiresAt.Before(now.Add(threshold)) {
			expiring = append(expiring, secret)
		}
	}

	return expiring, nil
}
