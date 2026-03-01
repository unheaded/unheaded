// SPDX-License-Identifier: GPL-2.0-only
// Unheaded Kingdom — AF_XDP userspace socket library
// Zero-copy packet I/O via kernel AF_XDP interface
//
// Sacred Law: NO external deps. Rust std only. All kernel interaction
// via raw syscalls through std::arch::asm!

pub mod engine;
pub mod ffi;
pub mod ring;
pub mod syscall;
pub mod umem;
pub mod xsk;

pub use af_xdp_common as common;
pub use engine::{EngineStats, EventLoop, PacketBuf, SignalHandler, XdpEngine};
pub use ffi::{AfxdpHandle, AfxdpStats};
pub use ring::Ring;
pub use umem::Umem;
pub use xsk::XskSocket;
