// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // src/lib.rs includes the generated code (tonic::include_proto!), so the
    // crate cannot build without it. Skipping here when protoc is missing only
    // moved the failure to a confusing "couldn't read" error in lib.rs; fail
    // here instead, naming the fix.
    if std::path::Path::new("proto/zhen.proto").exists() {
        tonic_build::compile_protos("proto/zhen.proto").map_err(|e| {
            format!(
                "compiling proto/zhen.proto: {e} (install protoc: apt install protobuf-compiler)"
            )
        })?;
    }
    Ok(())
}
