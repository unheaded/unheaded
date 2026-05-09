// SPDX-License-Identifier: GPL-3.0-or-later
//
// asm_fixtures.rs — Host-runnable regression coverage for the .asm test
// programs in tests/mbc-pipeline/.
//
// These fixtures previously had ZERO host-runnable coverage — only
// scripts/verify-mbc-pipeline.sh exercised them, and that script requires
// sudo + pinned BPF maps so it can't run in CI / unattended.
//
// This test walks every *.asm under tests/mbc-pipeline/ and asserts that
// the assembler accepts each file cleanly. Doesn't execute the resulting
// MBC program — that's the BPF-side script's job — but catches assembler
// regressions and fixture-file corruption immediately.

use monad_mbc::assemble;
use std::fs;
use std::path::PathBuf;

/// Locate the workspace's tests/mbc-pipeline/ directory relative to this crate.
fn fixtures_dir() -> PathBuf {
    let manifest = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    manifest.join("..").join("..").join("tests").join("mbc-pipeline")
}

#[test]
fn mbc_pipeline_fixture_directory_exists() {
    let dir = fixtures_dir();
    assert!(
        dir.is_dir(),
        "tests/mbc-pipeline/ directory not found at {:?} — repo layout changed?",
        dir
    );
}

#[test]
fn every_mbc_pipeline_asm_fixture_assembles_cleanly() {
    let dir = fixtures_dir();
    let mut checked = 0usize;
    let mut failures: Vec<(String, String)> = Vec::new();

    let entries = fs::read_dir(&dir)
        .unwrap_or_else(|e| panic!("read tests/mbc-pipeline/: {}", e));

    for entry in entries.flatten() {
        let path = entry.path();
        if path.extension().and_then(|s| s.to_str()) != Some("asm") {
            continue;
        }
        let name = path
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or("(unknown)")
            .to_string();
        let source = fs::read_to_string(&path)
            .unwrap_or_else(|e| panic!("read {}: {}", name, e));
        match assemble(&source) {
            Ok(words) => {
                assert!(
                    !words.is_empty(),
                    "{} assembled to zero instructions — empty fixture?",
                    name
                );
                checked += 1;
            }
            Err(e) => failures.push((name, format!("{:?}", e))),
        }
    }

    if !failures.is_empty() {
        let msg = failures
            .iter()
            .map(|(n, e)| format!("  {}: {}", n, e))
            .collect::<Vec<_>>()
            .join("\n");
        panic!("{} fixture(s) failed to assemble:\n{}", failures.len(), msg);
    }

    // As of 2026-05-09 there are 11 fixtures; assert at least 8 to allow
    // adds/removes without breaking but catch a sudden disappearance.
    assert!(
        checked >= 8,
        "expected ≥8 fixtures to assemble, only {} did — repo drift?",
        checked
    );
}

#[test]
fn fibonacci_fixture_produces_expected_instruction_shape() {
    // Sanity-anchor: fibonacci is the canonical loop test. Spot-check that
    // it assembles to a known-shape instruction stream (10 instructions per
    // the source: 3 MOVI + MOV + ADD + MOV + ADDI + JNZ + HALT = 8 in the
    // body + the loop header). We only assert "more than 6" — a harder
    // count would fight against future minor edits.
    let path = fixtures_dir().join("test-fibonacci.asm");
    let src = fs::read_to_string(&path).expect("read test-fibonacci.asm");
    let words = assemble(&src).expect("fibonacci should assemble cleanly");
    assert!(
        words.len() >= 6,
        "fibonacci assembled to only {} insns — too short?",
        words.len()
    );
}
