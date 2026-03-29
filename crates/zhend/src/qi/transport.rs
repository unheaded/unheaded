//! Transport layer for Qi gossip.
//!
//! Two modes:
//! 1. Piggybacked — fragments ride existing Monad HbH options (zero extra traffic)
//! 2. Standalone — UDP datagrams when Monad transport unavailable
//!
//! Piggybacked mode is preferred. Wu Wei — knowledge flows without forcing.

// TODO: implement UDP transport for standalone gossip
// TODO: implement Monad HbH piggybacking via monad::bridge
