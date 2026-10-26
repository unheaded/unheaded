// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"strings"
	"testing"
)

// D-22: http_error refuses with guidance toward the workload's own error
// knob; the other types that have no injector refuse with "not implemented"
// rather than ever reporting success.
func TestUnimplementedTypesRefuse(t *testing.T) {
	fx := newFixture(t, func(_ *Config, _ *Deps) {})
	fx.s.RefuseType(ExpTypeHTTPError, "not supported: deploy a faulty build instead (steel-echo STEEL_ERROR_RATE)")

	t.Run("http_error", func(t *testing.T) {
		_, err := fx.s.StartExperiment(context.Background(), &Experiment{
			Name: "x", Type: ExpTypeHTTPError, Target: Target{Kind: KindLXD, ID: "steel-echo-v2-0"}})
		if !isRefusal(err, RefuseNoInjector) || !strings.Contains(err.Error(), "deploy a faulty build") {
			t.Fatalf("err = %v", err)
		}
	})

	for _, ty := range []ExperimentType{
		ExpTypeCPUStress, ExpTypeMemoryStress, ExpTypeDiskStress, ExpTypeDNSFailure,
		ExpTypeTimeSkew, ExpTypeNodeDrain, ExpTypeResourceExhaust, ExpTypeCustom,
	} {
		t.Run(string(ty), func(t *testing.T) {
			_, err := fx.s.StartExperiment(context.Background(), &Experiment{
				Name: "x", Type: ty, Target: Target{Kind: KindLXD, ID: "steel-echo-v2-0"}})
			if !isRefusal(err, RefuseNoInjector) || !strings.Contains(err.Error(), "not implemented") {
				t.Fatalf("%s: err = %v, want a not-implemented refusal", ty, err)
			}
		})
	}
}
