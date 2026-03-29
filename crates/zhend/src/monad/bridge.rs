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

use crate::de::{self, ContextVector, Embedder};
use crate::de::similarity::rank_by_de;
use crate::monad::hbh::{self, ZhenHbhOption};
use crate::pu::{Fragment, TieredStore};
use crate::ZhenResult;

/// Result of processing an incoming Monad packet through the bridge.
pub type OnPacketResult = ZhenResult<(Option<MonadContext>, Vec<(Fragment, f32)>)>;

/// Context extracted from a Monad register (20-byte wire format).
///
/// The Monad register carries flow metadata that Zhen re-interprets
/// as context signals for De ranking. Monad doesn't know this happens.
///
/// Wire mapping:
/// - `service_id` → domain context (which service generated this flow)
/// - `action_id`  → operation context (what operation is being performed)
/// - `flow_label` → session continuity (IPv6 flow label, 20 bits)
/// - `flags`      → behavioral hints (C|Y|T|E|S|M|CUST|R bitfield)
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MonadContext {
    /// Service identifier from Monad register.
    pub service_id: u16,
    /// Action identifier from Monad register.
    pub action_id: u16,
    /// IPv6 flow label (20 bits, stored in u32).
    pub flow_label: u32,
    /// Monad flags byte (C|Y|T|E|S|M|CUST|R).
    pub flags: u8,
}

impl MonadContext {
    /// Extract MonadContext from raw Monad register bytes (20 bytes).
    ///
    /// Wire layout (from Foundation spec v0x01):
    /// ```text
    /// Bytes 0-1:   Version + Flags
    /// Bytes 2-3:   Service ID
    /// Bytes 4-5:   Action ID
    /// Bytes 6-8:   Flow Label (20 bits packed in 3 bytes, big-endian)
    /// Bytes 9-19:  Remaining register fields (ignored by Zhen)
    /// ```
    ///
    /// ALL INPUTS HOSTILE.
    pub fn from_register(register: &[u8]) -> Option<Self> {
        if register.len() < 20 {
            return None;
        }

        let flags = register[1];
        let service_id = u16::from_be_bytes([register[2], register[3]]);
        let action_id = u16::from_be_bytes([register[4], register[5]]);
        // Flow label: 20 bits from bytes 6-8 (mask top 4 bits of byte 6).
        let flow_label = ((register[6] as u32 & 0x0F) << 16)
            | ((register[7] as u32) << 8)
            | (register[8] as u32);

        Some(Self {
            service_id,
            action_id,
            flow_label,
            flags,
        })
    }

    /// Convert Monad context into a text representation suitable for
    /// embedding via De. This is how protocol flow metadata becomes
    /// a context vector for fragment surfacing.
    ///
    /// The string encodes service domain + operation type so that the
    /// hash-based embedder produces deterministic, distinguishable vectors
    /// for different service/action combinations.
    pub fn to_context_string(&self) -> String {
        format!(
            "service:{} action:{} flow:{}",
            self.service_id, self.action_id, self.flow_label
        )
    }

    /// Build a ContextVector from this Monad context using the given embedder.
    ///
    /// This is the main entry point: Monad register fields → De context vector.
    pub fn to_context_vector(&self, embedder: &Embedder) -> ContextVector {
        ContextVector::from_text(embedder, &self.to_context_string())
    }
}

/// Events emitted by the bridge as it processes Monad traffic.
///
/// Consumers (Li topology observer, metrics, logging) subscribe to
/// these events without coupling to the bridge internals.
#[derive(Debug, Clone)]
pub enum BridgeEvent {
    /// A Monad packet was observed and context extracted.
    PacketSeen {
        context: MonadContext,
    },
    /// A Zhen fragment digest was found piggybacked on a packet.
    FragmentPiggybacked {
        option: ZhenHbhOption,
        context: MonadContext,
    },
    /// The bridge's accumulated context was updated from a new flow.
    ContextUpdated {
        context: MonadContext,
        surfaced_count: usize,
    },
}

/// The Monad Bridge — where Layer 0 meets Layer 1+.
///
/// Holds references to the TieredStore (Pu) and Embedder (De) so it
/// can process incoming Monad packets and select outgoing piggyback
/// fragments.
pub struct MonadBridge {
    /// Fragment store for ingestion and retrieval.
    store: TieredStore,
    /// Embedding engine for context vector construction.
    embedder: Embedder,
    /// Event log for bridge activity (bounded ring buffer in production;
    /// Vec in tests for assertion).
    events: std::sync::Mutex<Vec<BridgeEvent>>,
}

impl MonadBridge {
    /// Create a new MonadBridge with the given store and embedder.
    pub fn new(store: TieredStore, embedder: Embedder) -> Self {
        Self {
            store,
            embedder,
            events: std::sync::Mutex::new(Vec::new()),
        }
    }

    /// Process an incoming Monad packet.
    ///
    /// 1. Extract MonadContext from the register bytes.
    /// 2. Check for piggybacked Zhen HbH option in extension header bytes.
    /// 3. Build context vector → surface relevant fragments via De.
    /// 4. Emit BridgeEvents for observers.
    ///
    /// Returns the surfaced fragments (if any) and the extracted context.
    ///
    /// `register`: raw 20-byte Monad register.
    /// `hbh_options`: raw bytes from IPv6 Hop-by-Hop extension header
    ///                (may be empty if no HbH header present).
    pub fn on_packet(
        &self,
        register: &[u8],
        hbh_options: &[u8],
        top_k: usize,
    ) -> OnPacketResult {
        let context = match MonadContext::from_register(register) {
            Some(ctx) => ctx,
            None => return Ok((None, vec![])),
        };

        // Emit PacketSeen event.
        self.emit(BridgeEvent::PacketSeen {
            context: context.clone(),
        });

        // Check for piggybacked Zhen option.
        if let Some(option) = hbh::parse_zhen_option(hbh_options) {
            self.emit(BridgeEvent::FragmentPiggybacked {
                option,
                context: context.clone(),
            });
        }

        // Build context vector from Monad flow metadata.
        let ctx_vec = context.to_context_vector(&self.embedder);

        // Surface relevant fragments from the store.
        let surfaced = de::surface(&self.store, &ctx_vec, top_k)?;

        self.emit(BridgeEvent::ContextUpdated {
            context: context.clone(),
            surfaced_count: surfaced.len(),
        });

        Ok((Some(context), surfaced))
    }

    /// Select the best fragment digest to piggyback on an outgoing packet.
    ///
    /// Given a Monad context for the outgoing flow, find the most relevant
    /// fragment from the store and return its BLAKE3 hash + metadata for
    /// encoding into a Zhen HbH option.
    ///
    /// Returns None if:
    /// - No fragments in store
    /// - No fragments have embeddings
    /// - Context produces no matches
    pub fn select_piggyback(
        &self,
        context: &MonadContext,
    ) -> ZhenResult<Option<ZhenHbhOption>> {
        let ctx_vec = context.to_context_vector(&self.embedder);
        if ctx_vec.is_empty() {
            return Ok(None);
        }

        let fragments = self.store.l1_fragments()?;
        if fragments.is_empty() {
            return Ok(None);
        }

        let ranked = rank_by_de(&ctx_vec.embedding, &fragments, 1);
        if ranked.is_empty() {
            return Ok(None);
        }

        let best = ranked[0].0;
        Ok(Some(ZhenHbhOption {
            fragment_hash: best.id.0,
            embedding_dim: Some(best.embedding.len() as u8),
            payload_len: Some(best.payload_len() as u16),
        }))
    }

    /// Drain all emitted events (for testing / observer consumption).
    pub fn drain_events(&self) -> Vec<BridgeEvent> {
        let mut events = self.events.lock().expect("event lock poisoned");
        events.drain(..).collect()
    }

    /// Access the underlying store (for test setup / inspection).
    pub fn store(&self) -> &TieredStore {
        &self.store
    }

    /// Access the embedder.
    pub fn embedder(&self) -> &Embedder {
        &self.embedder
    }

    fn emit(&self, event: BridgeEvent) {
        let mut events = self.events.lock().expect("event lock poisoned");
        events.push(event);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::de::ingest_with_embedding;
    use crate::monad::hbh::encode_zhen_option;
    use tempfile::TempDir;

    /// Build a mock 20-byte Monad register.
    fn mock_register(service_id: u16, action_id: u16, flow_label: u32, flags: u8) -> [u8; 20] {
        let mut reg = [0u8; 20];
        reg[0] = 0x01; // version
        reg[1] = flags;
        reg[2..4].copy_from_slice(&service_id.to_be_bytes());
        reg[4..6].copy_from_slice(&action_id.to_be_bytes());
        // Flow label: 20 bits into bytes 6-8.
        reg[6] = ((flow_label >> 16) & 0x0F) as u8;
        reg[7] = ((flow_label >> 8) & 0xFF) as u8;
        reg[8] = (flow_label & 0xFF) as u8;
        reg
    }

    fn test_bridge() -> (MonadBridge, TempDir) {
        let tmp = TempDir::new().unwrap();
        let store = TieredStore::open(tmp.path()).unwrap();
        let embedder = Embedder::default_hash();
        let bridge = MonadBridge::new(store, embedder);
        (bridge, tmp)
    }

    // ---- MonadContext tests ----

    #[test]
    fn test_monad_context_from_register() {
        let reg = mock_register(0x1234, 0x5678, 0xABCDE, 0b1010_0000);
        let ctx = MonadContext::from_register(&reg).expect("valid register");

        assert_eq!(ctx.service_id, 0x1234);
        assert_eq!(ctx.action_id, 0x5678);
        assert_eq!(ctx.flow_label, 0xABCDE);
        assert_eq!(ctx.flags, 0b1010_0000);
    }

    #[test]
    fn test_monad_context_rejects_short_register() {
        assert!(MonadContext::from_register(&[]).is_none());
        assert!(MonadContext::from_register(&[0u8; 19]).is_none());
    }

    #[test]
    fn test_monad_context_to_context_vector() {
        let reg = mock_register(100, 200, 300, 0);
        let ctx = MonadContext::from_register(&reg).unwrap();
        let embedder = Embedder::default_hash();
        let cv = ctx.to_context_vector(&embedder);

        assert!(!cv.is_empty());
        assert_eq!(cv.embedding.len(), 384);
    }

    #[test]
    fn test_monad_context_different_services_different_vectors() {
        let embedder = Embedder::default_hash();

        let reg_a = mock_register(1, 1, 0, 0);
        let ctx_a = MonadContext::from_register(&reg_a).unwrap();
        let cv_a = ctx_a.to_context_vector(&embedder);

        let reg_b = mock_register(2, 1, 0, 0);
        let ctx_b = MonadContext::from_register(&reg_b).unwrap();
        let cv_b = ctx_b.to_context_vector(&embedder);

        assert_ne!(
            cv_a.embedding, cv_b.embedding,
            "different service_ids must produce different context vectors"
        );
    }

    #[test]
    fn test_monad_context_flow_label_20bit_mask() {
        // Flow label with all 20 bits set.
        let reg = mock_register(0, 0, 0xFFFFF, 0);
        let ctx = MonadContext::from_register(&reg).unwrap();
        assert_eq!(ctx.flow_label, 0xFFFFF);

        // Flow label zero.
        let reg = mock_register(0, 0, 0, 0);
        let ctx = MonadContext::from_register(&reg).unwrap();
        assert_eq!(ctx.flow_label, 0);
    }

    // ---- Bridge on_packet tests ----

    #[test]
    fn test_on_packet_extracts_context() {
        let (bridge, _tmp) = test_bridge();
        let reg = mock_register(42, 7, 12345, 0xC0);

        let (ctx, surfaced) = bridge.on_packet(&reg, &[], 10).unwrap();

        let ctx = ctx.expect("should extract context");
        assert_eq!(ctx.service_id, 42);
        assert_eq!(ctx.action_id, 7);
        assert_eq!(ctx.flow_label, 12345);
        assert!(surfaced.is_empty(), "empty store should produce no results");
    }

    #[test]
    fn test_on_packet_rejects_invalid_register() {
        let (bridge, _tmp) = test_bridge();

        let (ctx, surfaced) = bridge.on_packet(&[0u8; 5], &[], 10).unwrap();
        assert!(ctx.is_none());
        assert!(surfaced.is_empty());
    }

    #[test]
    fn test_on_packet_detects_piggybacked_option() {
        let (bridge, _tmp) = test_bridge();
        let reg = mock_register(1, 1, 1, 0);
        let hash = *blake3::hash(b"piggybacked fragment").as_bytes();
        let hbh_bytes = encode_zhen_option(&hash, Some(128), Some(1024));

        bridge.on_packet(&reg, &hbh_bytes, 10).unwrap();

        let events = bridge.drain_events();
        assert!(events.len() >= 2, "should have PacketSeen + FragmentPiggybacked");

        let has_piggyback = events.iter().any(|e| matches!(e, BridgeEvent::FragmentPiggybacked { .. }));
        assert!(has_piggyback, "should emit FragmentPiggybacked event");
    }

    #[test]
    fn test_on_packet_emits_events() {
        let (bridge, _tmp) = test_bridge();
        let reg = mock_register(1, 1, 1, 0);

        bridge.on_packet(&reg, &[], 10).unwrap();

        let events = bridge.drain_events();
        assert_eq!(events.len(), 2, "should emit PacketSeen + ContextUpdated");

        assert!(matches!(&events[0], BridgeEvent::PacketSeen { .. }));
        assert!(matches!(&events[1], BridgeEvent::ContextUpdated { surfaced_count: 0, .. }));
    }

    #[test]
    fn test_on_packet_surfaces_relevant_fragments() {
        let (bridge, _tmp) = test_bridge();

        // Ingest fragments with embeddings matching different service contexts.
        let ctx_str_42 = format!("service:{} action:{} flow:{}", 42, 1, 0);
        ingest_with_embedding(
            bridge.store(),
            bridge.embedder(),
            ctx_str_42.as_bytes().to_vec(),
            Some("text/plain".into()),
        )
        .unwrap();

        ingest_with_embedding(
            bridge.store(),
            bridge.embedder(),
            b"completely unrelated cooking recipe".to_vec(),
            Some("text/plain".into()),
        )
        .unwrap();

        // Query with service_id=42 — should surface the matching fragment first.
        let reg = mock_register(42, 1, 0, 0);
        let (ctx, surfaced) = bridge.on_packet(&reg, &[], 10).unwrap();

        assert!(ctx.is_some());
        assert!(!surfaced.is_empty(), "should surface fragments");

        // The service-42 fragment should rank highest (exact content match
        // via hash embedder: identical string → identical vector → cosine 1.0).
        assert_eq!(
            surfaced[0].0.payload,
            ctx_str_42.as_bytes(),
            "most relevant fragment should match service context"
        );
    }

    // ---- Piggyback selection tests ----

    #[test]
    fn test_select_piggyback_returns_most_relevant() {
        let (bridge, _tmp) = test_bridge();

        // Ingest fragments.
        let ctx_str = format!("service:{} action:{} flow:{}", 10, 20, 0);
        ingest_with_embedding(
            bridge.store(),
            bridge.embedder(),
            ctx_str.as_bytes().to_vec(),
            None,
        )
        .unwrap();

        ingest_with_embedding(
            bridge.store(),
            bridge.embedder(),
            b"irrelevant data about bananas".to_vec(),
            None,
        )
        .unwrap();

        let context = MonadContext {
            service_id: 10,
            action_id: 20,
            flow_label: 0,
            flags: 0,
        };

        let option = bridge.select_piggyback(&context).unwrap();
        let option = option.expect("should select a fragment for piggyback");

        // The hash should match the service-context fragment (exact match).
        let expected_hash = blake3::hash(ctx_str.as_bytes());
        assert_eq!(
            option.fragment_hash,
            *expected_hash.as_bytes(),
            "piggyback should select the most relevant fragment"
        );
        assert!(option.embedding_dim.is_some());
        assert!(option.payload_len.is_some());
    }

    #[test]
    fn test_select_piggyback_empty_store() {
        let (bridge, _tmp) = test_bridge();

        let context = MonadContext {
            service_id: 1,
            action_id: 1,
            flow_label: 1,
            flags: 0,
        };

        let option = bridge.select_piggyback(&context).unwrap();
        assert!(option.is_none(), "empty store should produce no piggyback");
    }

    #[test]
    fn test_select_piggyback_disabled_embedder() {
        let tmp = TempDir::new().unwrap();
        let store = TieredStore::open(tmp.path()).unwrap();
        let embedder = Embedder::disabled();
        let bridge = MonadBridge::new(store, embedder);

        let context = MonadContext {
            service_id: 1,
            action_id: 1,
            flow_label: 1,
            flags: 0,
        };

        let option = bridge.select_piggyback(&context).unwrap();
        assert!(
            option.is_none(),
            "disabled embedder should produce no piggyback"
        );
    }

    // ---- HbH roundtrip through bridge ----

    #[test]
    fn test_hbh_roundtrip_through_bridge() {
        let (bridge, _tmp) = test_bridge();

        // Simulate: a remote node piggybacked a fragment digest.
        let hash = *blake3::hash(b"remote knowledge").as_bytes();
        let hbh_bytes = encode_zhen_option(&hash, Some(64), Some(2048));

        let reg = mock_register(99, 1, 500, 0);
        bridge.on_packet(&reg, &hbh_bytes, 5).unwrap();

        let events = bridge.drain_events();
        let piggyback_event = events.iter().find(|e| {
            matches!(e, BridgeEvent::FragmentPiggybacked { .. })
        });

        let piggyback_event = piggyback_event.expect("should detect piggybacked option");
        if let BridgeEvent::FragmentPiggybacked { option, context } = piggyback_event {
            assert_eq!(option.fragment_hash, hash);
            assert_eq!(option.embedding_dim, Some(64));
            assert_eq!(option.payload_len, Some(2048));
            assert_eq!(context.service_id, 99);
        } else {
            panic!("wrong event type");
        }
    }

    // ---- Event drain ----

    #[test]
    fn test_drain_events_clears_buffer() {
        let (bridge, _tmp) = test_bridge();
        let reg = mock_register(1, 1, 1, 0);

        bridge.on_packet(&reg, &[], 10).unwrap();
        assert!(!bridge.drain_events().is_empty());

        // Second drain should be empty.
        assert!(bridge.drain_events().is_empty());
    }
}
