// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.
//! Prints one service token: `mint <key-file> <sub> <aud> <iat>`.
//!
//! The Rust half of the cross-language check in
//! tests/svctoken/crosslang_test.go: Go makes the key, this mints, Go's
//! pkg/auth verifies it and compares it with its own token for the same
//! inputs.

use std::path::Path;
use std::process::ExitCode;

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.len() != 4 {
        eprintln!("usage: mint <key-file> <sub> <aud> <iat>");
        return ExitCode::from(2);
    }
    let Ok(iat) = args[3].parse::<u64>() else {
        eprintln!("mint: iat must be Unix seconds");
        return ExitCode::from(2);
    };
    let tok = svc_token::Minter::load(&args[1], Path::new(&args[0]))
        .and_then(|m| m.mint_at(&args[2], iat));
    match tok {
        Ok(t) => {
            println!("{t}");
            ExitCode::SUCCESS
        }
        Err(e) => {
            eprintln!("mint: {e}");
            ExitCode::FAILURE
        }
    }
}
