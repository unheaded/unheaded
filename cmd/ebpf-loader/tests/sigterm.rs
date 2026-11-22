// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2026 Stevie Bellis.

//! M-198.2: systemd stops a unit with SIGTERM, and the loader's cleanup (drop
//! the program handles, remove the PID file) has to run for it as it does for
//! SIGINT. No kernel is needed: a name the loader does not know is skipped, so
//! it reaches the wait with nothing loaded.

use std::path::{Path, PathBuf};
use std::process::{Child, Command, ExitStatus, Stdio};
use std::time::{Duration, Instant};

const WAIT: Duration = Duration::from_secs(20);

fn scratch(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("ebpf-loader-{tag}-{}", std::process::id()));
    std::fs::create_dir_all(&dir).expect("scratch dir");
    std::fs::write(dir.join("no-such-program"), b"object").expect("write object");
    dir
}

fn spawn_waiting(dir: &Path, pid_file: &Path) -> Child {
    let child = Command::new(env!("CARGO_BIN_EXE_ebpf-loader"))
        .args(["--only", "no-such-program", "--obj-dir"])
        .arg(dir)
        .arg("--allow-untrusted-ebpf-obj-dir")
        .arg("--pid-file")
        .arg(pid_file)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .expect("run ebpf-loader");
    let start = Instant::now();
    while !pid_file.exists() {
        assert!(
            start.elapsed() < WAIT,
            "the loader never wrote its PID file"
        );
        std::thread::sleep(Duration::from_millis(20));
    }
    // The handler is installed right after the PID file is written.
    std::thread::sleep(Duration::from_millis(200));
    child
}

fn signal_and_wait(mut child: Child, signal: &str) -> ExitStatus {
    let sent = Command::new("kill")
        .args([signal, &child.id().to_string()])
        .status()
        .expect("run kill");
    assert!(sent.success(), "kill {signal} failed");
    let start = Instant::now();
    loop {
        if let Some(status) = child.try_wait().expect("wait") {
            return status;
        }
        if start.elapsed() > WAIT {
            let _ = child.kill();
            panic!("the loader did not exit on {signal}");
        }
        std::thread::sleep(Duration::from_millis(20));
    }
}

fn cleans_up_on(signal: &str, tag: &str) {
    let dir = scratch(tag);
    let pid_file = dir.join("loader.pid");
    let status = signal_and_wait(spawn_waiting(&dir, &pid_file), signal);
    assert!(
        status.success(),
        "{signal}: the loader was killed or failed ({status}), so its cleanup did not run"
    );
    assert!(
        !pid_file.exists(),
        "{signal}: the PID file is still there, so the cleanup did not run"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

#[test]
fn sigterm_runs_the_cleanup() {
    cleans_up_on("-TERM", "sigterm");
}

#[test]
fn sigint_still_runs_the_cleanup() {
    cleans_up_on("-INT", "sigint");
}
