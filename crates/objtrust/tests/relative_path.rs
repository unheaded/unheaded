// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//! A relative path is checked from the working directory, as open(2) takes it.
//! One test in its own binary, as it changes the process's working directory.

use objtrust::check;
use std::path::Path;

#[test]
fn a_relative_path_is_taken_from_the_working_directory() {
    std::env::set_current_dir("/etc").unwrap();
    check(Path::new("passwd")).unwrap();
    check(Path::new("../etc/passwd")).unwrap();

    let dir = std::env::temp_dir().join(format!("objtrust-relative-{}", std::process::id()));
    std::fs::create_dir_all(&dir).unwrap();
    std::fs::write(dir.join("prog.o"), b"x").unwrap();
    std::env::set_current_dir(&dir).unwrap();
    assert!(check(Path::new("prog.o")).is_err());
    std::env::set_current_dir("/").unwrap();
    std::fs::remove_dir_all(dir).ok();
}
