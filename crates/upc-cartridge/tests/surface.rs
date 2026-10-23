// SPDX-License-Identifier: GPL-3.0-or-later
//! S5 Step 264: the cartridge surface is a registered ADR-080 surface whose
//! syscall numbers are exactly the ABI v0 ones, gated to its own object.

use monad_common::cart::{sys, STAGE_TICK, STAGE_XDP};
use upc_api::{FeatureGate, SurfaceRegistry};
use upc_cartridge::{allowed_syscalls, register_surface, CARTRIDGE_SURFACE};

#[test]
fn cartridge_surface_registers_beside_doom_and_xv6() {
    let mut r = SurfaceRegistry::new();
    register_surface(&mut r);
    let s = r.get("cartridge").expect("registered");
    assert_eq!(s.feature_gate, FeatureGate::Cartridge);
    assert_ne!(s.feature_gate, FeatureGate::Default);
    assert_ne!(s.feature_gate, FeatureGate::AscendLinux);
    let nums: Vec<u32> = s.syscalls.iter().map(|d| d.number).collect();
    assert_eq!(
        nums,
        vec![
            sys::FIELD as u32,
            sys::NOW_NS_LO as u32,
            sys::VERDICT as u32,
            sys::EMIT as u32,
            sys::QUEUE as u32,
            sys::YIELD as u32,
        ]
    );
    let names: Vec<&str> = s.syscalls.iter().map(|d| d.name).collect();
    assert_eq!(
        names,
        vec!["FIELD", "NOW_NS_LO", "VERDICT", "EMIT", "QUEUE", "YIELD"]
    );
    // Every syscall a stage allows is on the surface (the allowlist covers both).
    for stage in [STAGE_XDP, STAGE_TICK] {
        for n in allowed_syscalls(stage) {
            assert!(
                nums.contains(&(*n as u32)),
                "stage {stage} syscall {n} missing"
            );
        }
    }
    assert_eq!(CARTRIDGE_SURFACE.name, "cartridge");
}
