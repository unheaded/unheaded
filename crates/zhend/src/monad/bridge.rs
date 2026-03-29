//! Bridge between Monad protocol flows and Zhen fragment operations.
//!
//! Listens for Monad packet events (via Anamnesis ring buffer or eBPF)
//! and:
//! 1. Extracts context metadata → feeds De (relevance engine)
//! 2. Detects piggybacked Zhen HbH options → feeds Pu (fragment store)
//! 3. Selects fragments for outgoing piggyback → feeds Qi (gossip)
//!
//! This is where Layer 0 (Zhen) meets Layer 1+ (Monad/Sophia/Wotan).
//! The bridge is one-way upward: Zhen observes Monad. Monad doesn't
//! know Zhen exists. The substrate is invisible to the protocol.

// TODO: Anamnesis event subscription (ring buffer consumer)
// TODO: eBPF map integration for raw HbH option access
// TODO: context vector construction from Monad register fields:
//       - service_id → domain context
//       - action_id → operation context
//       - flow_label → session continuity
//       - kingdom_mode bits → sovereignty context
// TODO: outbound piggyback selector (which fragments ride which packets)
