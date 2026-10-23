// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cuirassdriver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"unheaded/pkg/deploy/strategy"
)

// rollback.Manager adapters over the driver: Updater brings a service back
// to a version with a rolling update (max surge 1, nothing unavailable),
// Shifter moves its stable endpoint. Specs supplies each service's health,
// proxy and labels (Sword keeps them with its deployment history).

// Specs returns the ServiceSpec for a service.
type Specs func(service string) (ServiceSpec, error)

// Updater implements rollback.InstanceUpdater.
type Updater struct {
	Driver *Driver
	Specs  Specs
}

// Update ends with replicas healthy instances of version and no other
// version of the service.
func (u *Updater) Update(ctx context.Context, service, version, _ string, replicas int) error {
	if u == nil || u.Driver == nil || u.Specs == nil {
		return errors.New("cuirassdriver: updater has no driver")
	}
	if replicas < 1 {
		return fmt.Errorf("cuirassdriver: rollback of %s to %s with %d replicas", service, version, replicas)
	}
	spec, err := u.Specs(service)
	if err != nil {
		return err
	}
	insts, err := u.Driver.Instances(ctx, service)
	if err != nil {
		return err
	}
	cb, err := u.Driver.Callbacks(spec, "rollback-"+cleanVersion(version), version)
	if err != nil {
		return err
	}
	cfg := strategy.DefaultConfig(strategy.TypeRolling)
	cfg.HealthCheck.Interval = u.Driver.poll
	// Each create and delete may take up to the driver's timeout.
	cfg.ProgressDeadline = u.Driver.timeout * time.Duration(2*(replicas+len(insts))+1)
	res, err := strategy.NewRollingStrategy().Execute(ctx, &strategy.ExecuteParams{
		DeploymentID:   "rollback-" + cleanVersion(version),
		ServiceName:    service,
		CurrentVersion: currentVersion(insts, version),
		TargetVersion:  cleanVersion(version),
		Replicas:       replicas,
		Config:         cfg,
		Instances:      insts,
		Callbacks:      cb,
	})
	if err != nil {
		return err
	}
	if !res.Success {
		return errors.New(res.Message)
	}
	return nil
}

func currentVersion(insts []strategy.Instance, except string) string {
	for _, in := range insts {
		if in.Version != cleanVersion(except) {
			return in.Version
		}
	}
	return ""
}

// Shifter implements rollback.TrafficShifter.
type Shifter struct {
	Driver *Driver
	Specs  Specs
}

// Shift moves the stable endpoint to a healthy instance of toVersion. When
// none exists yet there is nothing to move it to: the Updater creates them,
// and deleting the old holder moves the endpoint onto them.
func (s *Shifter) Shift(ctx context.Context, service, fromVersion, toVersion string, weight int) error {
	if s == nil || s.Driver == nil || s.Specs == nil {
		return errors.New("cuirassdriver: shifter has no driver")
	}
	spec, err := s.Specs(service)
	if err != nil {
		return err
	}
	insts, err := s.Driver.Instances(ctx, service)
	if err != nil {
		return err
	}
	ready := false
	for _, in := range insts {
		if in.Version == cleanVersion(toVersion) && in.Healthy {
			ready = true
			break
		}
	}
	if !ready {
		return nil
	}
	cb, err := s.Driver.Callbacks(spec, "rollback-"+cleanVersion(toVersion), toVersion)
	if err != nil {
		return err
	}
	return cb.OnTrafficShift(ctx, &strategy.TrafficShiftParams{OldVersion: fromVersion, NewVersion: toVersion, Weight: weight})
}
