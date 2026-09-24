// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

// PQC Authentication Metrics — Vambraces PQC Observability

// PQCVerificationsTotal counts all PQC verifications by result and algorithm.
var PQCVerificationsTotal = NewCounterVec(
	"unheaded_pqc_verifications_total",
	"Total PQC signature verifications",
	nil,
	[]string{"result", "algo", "tier"},
)

// PQCVerifyDuration tracks PQC verification latency.
var PQCVerifyDuration = NewHistogram(HistogramOpts{
	Name:    "unheaded_pqc_verify_duration_seconds",
	Help:    "PQC signature verification duration",
	Buckets: []float64{0.0000001, 0.0000003, 0.000001, 0.000003, 0.00001, 0.00003, 0.0001, 0.0003, 0.001, 0.005},
})

// PQCActiveKeys tracks the number of active PQC keys.
var PQCActiveKeys = NewGauge(
	"unheaded_pqc_active_keys",
	"Number of active PQC keys in Sophia maps",
	nil,
)

// PQCActiveSignatures tracks the number of active PQC signatures.
var PQCActiveSignatures = NewGauge(
	"unheaded_pqc_active_signatures",
	"Number of active PQC signatures in Sophia maps",
	nil,
)

// PQCKeyRotations counts key rotation events by algorithm.
var PQCKeyRotations = NewCounterVec(
	"unheaded_pqc_key_rotations_total",
	"Total PQC key rotation events",
	nil,
	[]string{"algo"},
)

// PQCMapUtilization tracks BPF map utilization as a percentage.
var PQCMapUtilization = NewGaugeVec(
	"unheaded_pqc_map_utilization_ratio",
	"PQC BPF map utilization (0.0-1.0)",
	nil,
	[]string{"map_name"},
)

// PQCKEMTunnels tracks active KEM tunnels.
var PQCKEMTunnels = NewGauge(
	"unheaded_pqc_kem_tunnels_active",
	"Number of active KEM tunnels",
	nil,
)

// PQCSovereignValidations counts SOVEREIGN tier multi-sig validations.
var PQCSovereignValidations = NewCounterVec(
	"unheaded_pqc_sovereign_validations_total",
	"Total SOVEREIGN tier multi-sig validations",
	nil,
	[]string{"result"},
)

// These are deliberately NOT registered anywhere.
//
// Until 2026-09-24 an init() registered all eight into DefaultRegistry.
// Nothing served that registry, so they were invisible; and nothing in the
// tree has ever set or observed any of them (checked with git grep across
// every package). When ADR-094 step 4 made DefaultRegistry the one served by
// architect, micromanager, wotan, zhen-agentd, trace-collector-go and
// sophia-gateway, the four scalar ones began publishing zeros from services
// that do no PQC work: "unheaded_pqc_active_keys 0" from micromanager is a
// false reading, not an absent one.
//
// Whatever code actually performs PQC verification should register the ones
// it updates, with the registry its service serves. See ADR-094.
