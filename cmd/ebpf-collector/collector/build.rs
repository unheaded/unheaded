// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis.

fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure()
        // The client publishes; the server half backs the tests' mock Wotan.
        .build_server(true)
        .compile_protos(&["proto/topic.proto"], &["proto"])?;
    Ok(())
}
