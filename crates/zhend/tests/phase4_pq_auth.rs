//! Phase 4 Exit Gate: PQ Peer Authentication.
//!
//! ML-DSA-65 identity exchange + admission control.
//! Authenticated peers gossip normally; unauthenticated peers get zero fragment data.

use std::sync::Arc;
use std::time::Duration;

use tokio::sync::mpsc;

use zhend::crypto::sign::PqSigningKeypair;
use zhend::pu::{Fragment, TieredStore};
use zhend::qi::GossipEngine;
use zhend::ZhenConfig;

/// Helper to create a test config bound to a specific port.
fn test_config(gossip_port: u16, seed_peer: Option<String>, data_dir: &std::path::Path) -> ZhenConfig {
    ZhenConfig {
        data_dir: data_dir.to_path_buf(),
        grpc_addr: "[::1]:0".into(),
        quic_addr: "[::1]:0".into(),
        gossip_addr: format!("127.0.0.1:{}", gossip_port),
        seed_peers: seed_peer.into_iter().collect(),
        gossip_fanout: 10,
        gossip_interval_ms: 100, // fast cycle for testing
        ..ZhenConfig::default()
    }
}

/// Get a free port by binding to port 0, reading the assigned port, then dropping the socket.
async fn get_free_port() -> u16 {
    let socket = tokio::net::UdpSocket::bind("127.0.0.1:0").await.unwrap();
    socket.local_addr().unwrap().port()
}

#[tokio::test]
async fn test_authenticated_gossip_works() {
    // Two nodes, both with PQ identity. They should authenticate each other
    // and gossip fragments normally.
    let port_a = get_free_port().await;
    let port_b = get_free_port().await;

    let tmp_a = tempfile::TempDir::new().unwrap();
    let tmp_b = tempfile::TempDir::new().unwrap();

    let config_a = test_config(port_a, Some(format!("127.0.0.1:{}", port_b)), tmp_a.path());
    let config_b = test_config(port_b, Some(format!("127.0.0.1:{}", port_a)), tmp_b.path());

    let store_a = Arc::new(TieredStore::open(tmp_a.path()).unwrap());
    let store_b = Arc::new(TieredStore::open(tmp_b.path()).unwrap());

    // Ingest a fragment on node A.
    let payload = b"authenticated knowledge flows through the breath".to_vec();
    let fragment = Fragment::new(payload.clone(), vec![], None);
    let fragment_id = fragment.id.clone();
    assert!(store_a.ingest(fragment).unwrap(), "should be novel");

    // Verify node B does NOT have it yet.
    assert!(!store_b.contains(&fragment_id).unwrap());

    // Both nodes have PQ signing keypairs.
    let kp_a = PqSigningKeypair::generate();
    let kp_b = PqSigningKeypair::generate();

    let (shutdown_tx_a, shutdown_rx_a) = mpsc::channel(1);
    let (shutdown_tx_b, shutdown_rx_b) = mpsc::channel(1);

    let store_a_clone = store_a.clone();
    let engine_a = tokio::spawn(async move {
        let mut engine = GossipEngine::new(store_a_clone, config_a, shutdown_rx_a, Some(&kp_a));
        engine.run().await
    });

    let store_b_clone = store_b.clone();
    let engine_b = tokio::spawn(async move {
        let mut engine = GossipEngine::new(store_b_clone, config_b, shutdown_rx_b, Some(&kp_b));
        engine.run().await
    });

    // Poll until node B has the fragment, or timeout at 30 seconds.
    let deadline = tokio::time::Instant::now() + Duration::from_secs(30);
    let mut found = false;

    while tokio::time::Instant::now() < deadline {
        tokio::time::sleep(Duration::from_millis(200)).await;

        if store_b.contains(&fragment_id).unwrap() {
            found = true;
            break;
        }
    }

    let _ = shutdown_tx_a.send(()).await;
    let _ = shutdown_tx_b.send(()).await;
    tokio::time::sleep(Duration::from_millis(200)).await;
    drop(engine_a);
    drop(engine_b);

    assert!(found, "fragment should gossip between authenticated peers within 30 seconds");

    // Verify content integrity.
    let frag_b = store_b.get(&fragment_id).unwrap().expect("fragment should exist on node B");
    assert_eq!(frag_b.payload, payload);
    assert!(frag_b.verify(), "fragment integrity must hold after authenticated gossip");
}

#[tokio::test]
async fn test_unauthenticated_peer_gets_no_fragments() {
    // Node A has PQ identity, node B has NONE (unauthenticated).
    // Node A should NOT send fragment data to node B.
    let port_a = get_free_port().await;
    let port_b = get_free_port().await;

    let tmp_a = tempfile::TempDir::new().unwrap();
    let tmp_b = tempfile::TempDir::new().unwrap();

    let config_a = test_config(port_a, Some(format!("127.0.0.1:{}", port_b)), tmp_a.path());
    let config_b = test_config(port_b, Some(format!("127.0.0.1:{}", port_a)), tmp_b.path());

    let store_a = Arc::new(TieredStore::open(tmp_a.path()).unwrap());
    let store_b = Arc::new(TieredStore::open(tmp_b.path()).unwrap());

    // Ingest a fragment on node A.
    let payload = b"this should NOT reach the unauthenticated node".to_vec();
    let fragment = Fragment::new(payload, vec![], None);
    let fragment_id = fragment.id.clone();
    store_a.ingest(fragment).unwrap();

    // Node A has identity, node B does NOT.
    let kp_a = PqSigningKeypair::generate();

    let (shutdown_tx_a, shutdown_rx_a) = mpsc::channel(1);
    let (shutdown_tx_b, shutdown_rx_b) = mpsc::channel(1);

    let store_a_clone = store_a.clone();
    let engine_a = tokio::spawn(async move {
        let mut engine = GossipEngine::new(store_a_clone, config_a, shutdown_rx_a, Some(&kp_a));
        engine.run().await
    });

    let store_b_clone = store_b.clone();
    let engine_b = tokio::spawn(async move {
        // Node B has NO identity — runs in open mode but node A won't send it data.
        let mut engine = GossipEngine::new(store_b_clone, config_b, shutdown_rx_b, None);
        engine.run().await
    });

    // Wait 5 seconds — fragment should NOT propagate.
    tokio::time::sleep(Duration::from_secs(5)).await;

    let found = store_b.contains(&fragment_id).unwrap();

    let _ = shutdown_tx_a.send(()).await;
    let _ = shutdown_tx_b.send(()).await;
    tokio::time::sleep(Duration::from_millis(200)).await;
    drop(engine_a);
    drop(engine_b);

    assert!(
        !found,
        "unauthenticated peer must NOT receive fragment data from authenticated node"
    );
}
