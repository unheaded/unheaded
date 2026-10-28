// SPDX-License-Identifier: GPL-3.0-or-later
//! Print what an eBPF object declares about itself (ADR-080 §"I/O-time
//! gates"): `cargo run --example object_facts -- <object> [<object> ...]`.
//! Reads the file only; loads nothing.

use upc_api::object::hex32;
use upc_api::ObjectFacts;

fn main() {
    let mut failed = false;
    for path in std::env::args().skip(1) {
        match ObjectFacts::from_path(std::path::Path::new(&path)) {
            Ok(f) => {
                println!("{path}");
                println!("  UPC_SURFACE_ID   {}", hex32(&f.surface_id));
                match f.mmu_geometry {
                    Some(g) => println!(
                        "  UPC_MMU_GEOMETRY base 0x{:X} stride 0x{:X} slice_len 0x{:X} ceiling 0x{:X} max_pids {}",
                        g.base, g.stride, g.slice_len, g.ceiling, g.max_pids
                    ),
                    None => println!("  UPC_MMU_GEOMETRY (absent)"),
                }
            }
            Err(e) => {
                eprintln!("{path}: {e}");
                failed = true;
            }
        }
    }
    if failed {
        std::process::exit(1);
    }
}
