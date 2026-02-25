// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package rules provides rule evaluation functionality.
package rules

import (
	"context"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/alerting"
)

// EvaluationResult represents the result of evaluating a rule.
type EvaluationResult struct {
	RuleID      string
	Firing      bool
	Value       float64
	Labels      map[string]string
	Error       error
	EvaluatedAt time.Time
}

// Evaluator handles rule evaluation.
type Evaluator struct {
	provider alerting.MetricProvider
	mu       sync.RWMutex
	cache    map[string]*evaluationState
}

// evaluationState tracks the state of rule evaluation over time.
type evaluationState struct {
	ruleID         string
	pendingSince   time.Time
	lastValue      float64
	lastLabels     map[string]string
	firingState    bool
	consecutiveHits int
}

// NewEvaluator creates a new rule evaluator.
func NewEvaluator(provider alerting.MetricProvider) *Evaluator {
	return &Evaluator{
		provider: provider,
		cache:    make(map[string]*evaluationState),
	}
}

// Evaluate evaluates a single rule and returns the result.
func (e *Evaluator) Evaluate(ctx context.Context, rule *alerting.AlertRule) (*EvaluationResult, error) {
	result := &EvaluationResult{
		RuleID:      rule.ID,
		EvaluatedAt: time.Now(),
		Labels:      make(map[string]string),
	}

	var value float64
	var err error

	switch rule.Type {
	case alerting.RuleTypeThreshold:
		value, err = e.evaluateThreshold(ctx, rule)
	case alerting.RuleTypeRate:
		value, err = e.evaluateRate(ctx, rule)
	case alerting.RuleTypeAbsence:
		value, err = e.evaluateAbsence(ctx, rule)
	default:
		return nil, fmt.Errorf("%w: unsupported rule type: %s", alerting.ErrEvaluationFailed, rule.Type)
	}

	if err != nil {
		result.Error = err
		return result, err
	}

	result.Value = value
	result.Firing = e.checkFiringCondition(rule, value)

	// Handle for period
	if rule.ForPeriod > 0 {
		result.Firing = e.checkForPeriod(rule, result.Firing, value)
	}

	// Copy labels
	for k, v := range rule.Labels {
		result.Labels[k] = v
	}

	return result, nil
}

// evaluateThreshold evaluates a threshold rule.
func (e *Evaluator) evaluateThreshold(ctx context.Context, rule *alerting.AlertRule) (float64, error) {
	value, err := e.provider.Query(ctx, rule.Expression, time.Now())
	if err != nil {
		return 0, fmt.Errorf("%w: query failed: %v", alerting.ErrEvaluationFailed, err)
	}
	return value, nil
}

// evaluateRate evaluates a rate-of-change rule.
func (e *Evaluator) evaluateRate(ctx context.Context, rule *alerting.AlertRule) (float64, error) {
	end := time.Now()
	start := end.Add(-rule.RateWindow)
	step := rule.RateWindow / 10
	if step < time.Minute {
		step = time.Minute
	}

	values, err := e.provider.QueryRange(ctx, rule.Expression, start, end, step)
	if err != nil {
		return 0, fmt.Errorf("%w: range query failed: %v", alerting.ErrEvaluationFailed, err)
	}

	if len(values) < 2 {
		return 0, nil
	}

	// Calculate rate of change
	first := values[0]
	last := values[len(values)-1]
	rate := (last - first) / rule.RateWindow.Seconds()

	return rate, nil
}

// evaluateAbsence evaluates an absence rule.
func (e *Evaluator) evaluateAbsence(ctx context.Context, rule *alerting.AlertRule) (float64, error) {
	end := time.Now()
	start := end.Add(-rule.AbsenceWindow)
	step := rule.AbsenceWindow / 10
	if step < time.Minute {
		step = time.Minute
	}

	values, err := e.provider.QueryRange(ctx, rule.Expression, start, end, step)
	if err != nil {
		return 0, fmt.Errorf("%w: range query failed: %v", alerting.ErrEvaluationFailed, err)
	}

	// Return 1 if data is absent (no values or all zeros/NaN)
	if len(values) == 0 {
		return 1, nil
	}

	// Check if any valid data exists
	for _, v := range values {
		if v != 0 {
			return 0, nil
		}
	}

	return 1, nil
}

// checkFiringCondition checks if the value meets the firing condition.
func (e *Evaluator) checkFiringCondition(rule *alerting.AlertRule, value float64) bool {
	switch rule.Type {
	case alerting.RuleTypeThreshold:
		return e.compareThreshold(value, rule.Threshold, rule.Operator)
	case alerting.RuleTypeRate:
		return value > rule.RateIncrease
	case alerting.RuleTypeAbsence:
		return value == 1
	default:
		return false
	}
}

// compareThreshold compares a value against a threshold.
func (e *Evaluator) compareThreshold(value, threshold float64, op alerting.ComparisonOperator) bool {
	switch op {
	case alerting.OpGreaterThan:
		return value > threshold
	case alerting.OpGreaterThanEqual:
		return value >= threshold
	case alerting.OpLessThan:
		return value < threshold
	case alerting.OpLessThanEqual:
		return value <= threshold
	case alerting.OpEqual:
		return value == threshold
	case alerting.OpNotEqual:
		return value != threshold
	default:
		return false
	}
}

// checkForPeriod checks if the condition has been true for the required period.
func (e *Evaluator) checkForPeriod(rule *alerting.AlertRule, currentlyFiring bool, value float64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	state, exists := e.cache[rule.ID]
	if !exists {
		state = &evaluationState{ruleID: rule.ID}
		e.cache[rule.ID] = state
	}

	now := time.Now()

	if currentlyFiring {
		if state.pendingSince.IsZero() {
			state.pendingSince = now
		}
		state.lastValue = value
		state.consecutiveHits++

		// Check if we've been firing long enough
		if now.Sub(state.pendingSince) >= rule.ForPeriod {
			state.firingState = true
			return true
		}
		return false
	}

	// Reset state if not firing
	state.pendingSince = time.Time{}
	state.firingState = false
	state.consecutiveHits = 0
	return false
}

// GetState returns the evaluation state for a rule.
func (e *Evaluator) GetState(ruleID string) (*evaluationState, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	state, exists := e.cache[ruleID]
	return state, exists
}

// ResetState resets the evaluation state for a rule.
func (e *Evaluator) ResetState(ruleID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.cache, ruleID)
}

// EvaluateAll evaluates all provided rules and returns results.
func (e *Evaluator) EvaluateAll(ctx context.Context, rules []*alerting.AlertRule) []*EvaluationResult {
	results := make([]*EvaluationResult, 0, len(rules))

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		result, _ := e.Evaluate(ctx, rule)
		results = append(results, result)
	}

	return results
}

// BatchEvaluator provides concurrent rule evaluation.
type BatchEvaluator struct {
	evaluator   *Evaluator
	concurrency int
}

// NewBatchEvaluator creates a new batch evaluator.
func NewBatchEvaluator(evaluator *Evaluator, concurrency int) *BatchEvaluator {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &BatchEvaluator{
		evaluator:   evaluator,
		concurrency: concurrency,
	}
}

// EvaluateBatch evaluates rules concurrently.
func (be *BatchEvaluator) EvaluateBatch(ctx context.Context, rules []*alerting.AlertRule) []*EvaluationResult {
	if len(rules) == 0 {
		return nil
	}

	resultCh := make(chan *EvaluationResult, len(rules))
	ruleCh := make(chan *alerting.AlertRule, len(rules))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < be.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rule := range ruleCh {
				result, _ := be.evaluator.Evaluate(ctx, rule)
				resultCh <- result
			}
		}()
	}

	// Send rules to workers
	go func() {
		for _, rule := range rules {
			if rule.Enabled {
				ruleCh <- rule
			}
		}
		close(ruleCh)
	}()

	// Wait and close results
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	results := make([]*EvaluationResult, 0, len(rules))
	for result := range resultCh {
		results = append(results, result)
	}

	return results
}
