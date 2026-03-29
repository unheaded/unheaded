//! Phase 3 Exit Gate: Two nodes see each other.
//!
//! Fragment ingested on node A appears on node B within 30 seconds (localhost test).

use std::sync::Arc;
use std::time::Duration;

use tokio::sync::mpsc;

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

#[tokio::test]
async fn test_two_nodes_gossip_fragment() {
    // Use port 0 so OS assigns free ports. But then we need to know the port
    // before starting the engine. Instead, we'll pick high ephemeral ports and hope
    // they're free. For robustness, use a retry mechanism.
    //
    // Actually, since the engine binds on startup, we need to know the address
    // at config time. We'll use port 0 trick: bind a temp socket, get the port,
    // close it, then use that port for the engine.
    let port_a = get_free_port().await;
    let port_b = get_free_port().await;

    let tmp_a = tempfile::TempDir::new().unwrap();
    let tmp_b = tempfile::TempDir::new().unwrap();

    let config_a = test_config(port_a, Some(format!("127.0.0.1:{}", port_b)), tmp_a.path());
    let config_b = test_config(port_b, Some(format!("127.0.0.1:{}", port_a)), tmp_b.path());

    let store_a = Arc::new(TieredStore::open(tmp_a.path()).unwrap());
    let store_b = Arc::new(TieredStore::open(tmp_b.path()).unwrap());

    // Ingest a fragment on node A.
    let payload = b"the dao that can be named is not the eternal dao".to_vec();
    let fragment = Fragment::new(payload.clone(), vec![], None);
    let fragment_id = fragment.id.clone();
    assert!(store_a.ingest(fragment).unwrap(), "should be novel");

    // Verify node B does NOT have it yet.
    assert!(!store_b.contains(&fragment_id).unwrap());

    // Start gossip engines.
    let (shutdown_tx_a, shutdown_rx_a) = mpsc::channel(1);
    let (shutdown_tx_b, shutdown_rx_b) = mpsc::channel(1);

    let store_a_clone = store_a.clone();
    let engine_a = tokio::spawn(async move {
        let mut engine = GossipEngine::new(store_a_clone, config_a, shutdown_rx_a, None);
        engine.run().await
    });

    let store_b_clone = store_b.clone();
    let engine_b = tokio::spawn(async move {
        let mut engine = GossipEngine::new(store_b_clone, config_b, shutdown_rx_b, None);
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

    // Shutdown engines.
    let _ = shutdown_tx_a.send(()).await;
    let _ = shutdown_tx_b.send(()).await;

    // Give engines time to shut down.
    tokio::time::sleep(Duration::from_millis(200)).await;

    // Drop handles (don't unwrap — shutdown may cause recv errors which is fine).
    drop(engine_a);
    drop(engine_b);

    assert!(found, "fragment should have been gossiped from node A to node B within 30 seconds");

    // Verify the fragment content on node B.
    let frag_b = store_b.get(&fragment_id).unwrap().expect("fragment should exist on node B");
    assert_eq!(frag_b.payload, payload);
    assert!(frag_b.verify(), "fragment integrity must hold after gossip");
    assert!(frag_b.hop_count >= 1, "hop count should be incremented");
}

#[tokio::test]
async fn test_multiple_fragments_gossip() {
    let port_a = get_free_port().await;
    let port_b = get_free_port().await;

    let tmp_a = tempfile::TempDir::new().unwrap();
    let tmp_b = tempfile::TempDir::new().unwrap();

    let config_a = test_config(port_a, Some(format!("127.0.0.1:{}", port_b)), tmp_a.path());
    let config_b = test_config(port_b, Some(format!("127.0.0.1:{}", port_a)), tmp_b.path());

    let store_a = Arc::new(TieredStore::open(tmp_a.path()).unwrap());
    let store_b = Arc::new(TieredStore::open(tmp_b.path()).unwrap());

    // Ingest multiple fragments on node A.
    let mut fragment_ids = Vec::new();
    for i in 0..5 {
        let payload = format!("fragment number {}", i).into_bytes();
        let fragment = Fragment::new(payload, vec![], None);
        fragment_ids.push(fragment.id.clone());
        store_a.ingest(fragment).unwrap();
    }

    let (shutdown_tx_a, shutdown_rx_a) = mpsc::channel(1);
    let (shutdown_tx_b, shutdown_rx_b) = mpsc::channel(1);

    let store_a_clone = store_a.clone();
    let engine_a = tokio::spawn(async move {
        let mut engine = GossipEngine::new(store_a_clone, config_a, shutdown_rx_a, None);
        engine.run().await
    });

    let store_b_clone = store_b.clone();
    let engine_b = tokio::spawn(async move {
        let mut engine = GossipEngine::new(store_b_clone, config_b, shutdown_rx_b, None);
        engine.run().await
    });

    // Wait for all fragments to propagate.
    let deadline = tokio::time::Instant::now() + Duration::from_secs(30);
    let mut all_found = false;

    while tokio::time::Instant::now() < deadline {
        tokio::time::sleep(Duration::from_millis(200)).await;

        let found_count = fragment_ids.iter()
            .filter(|id| store_b.contains(id).unwrap_or(false))
            .count();

        if found_count == fragment_ids.len() {
            all_found = true;
            break;
        }
    }

    let _ = shutdown_tx_a.send(()).await;
    let _ = shutdown_tx_b.send(()).await;
    tokio::time::sleep(Duration::from_millis(200)).await;
    drop(engine_a);
    drop(engine_b);

    assert!(all_found, "all 5 fragments should propagate from A to B");
}

#[tokio::test]
async fn test_bidirectional_gossip() {
    let port_a = get_free_port().await;
    let port_b = get_free_port().await;

    let tmp_a = tempfile::TempDir::new().unwrap();
    let tmp_b = tempfile::TempDir::new().unwrap();

    let config_a = test_config(port_a, Some(format!("127.0.0.1:{}", port_b)), tmp_a.path());
    let config_b = test_config(port_b, Some(format!("127.0.0.1:{}", port_a)), tmp_b.path());

    let store_a = Arc::new(TieredStore::open(tmp_a.path()).unwrap());
    let store_b = Arc::new(TieredStore::open(tmp_b.path()).unwrap());

    // Ingest a fragment on node A.
    let frag_a = Fragment::new(b"from node A".to_vec(), vec![], None);
    let id_a = frag_a.id.clone();
    store_a.ingest(frag_a).unwrap();

    // Ingest a different fragment on node B.
    let frag_b = Fragment::new(b"from node B".to_vec(), vec![], None);
    let id_b = frag_b.id.clone();
    store_b.ingest(frag_b).unwrap();

    let (shutdown_tx_a, shutdown_rx_a) = mpsc::channel(1);
    let (shutdown_tx_b, shutdown_rx_b) = mpsc::channel(1);

    let store_a_clone = store_a.clone();
    let engine_a = tokio::spawn(async move {
        let mut engine = GossipEngine::new(store_a_clone, config_a, shutdown_rx_a, None);
        engine.run().await
    });

    let store_b_clone = store_b.clone();
    let engine_b = tokio::spawn(async move {
        let mut engine = GossipEngine::new(store_b_clone, config_b, shutdown_rx_b, None);
        engine.run().await
    });

    let deadline = tokio::time::Instant::now() + Duration::from_secs(30);
    let mut both_found = false;

    while tokio::time::Instant::now() < deadline {
        tokio::time::sleep(Duration::from_millis(200)).await;

        let a_has_b = store_a.contains(&id_b).unwrap_or(false);
        let b_has_a = store_b.contains(&id_a).unwrap_or(false);

        if a_has_b && b_has_a {
            both_found = true;
            break;
        }
    }

    let _ = shutdown_tx_a.send(()).await;
    let _ = shutdown_tx_b.send(()).await;
    tokio::time::sleep(Duration::from_millis(200)).await;
    drop(engine_a);
    drop(engine_b);

    assert!(both_found, "fragments should propagate bidirectionally between A and B");
}

/// Get a free port by binding to port 0, reading the assigned port, then dropping the socket.
async fn get_free_port() -> u16 {
    let socket = tokio::net::UdpSocket::bind("127.0.0.1:0").await.unwrap();
    socket.local_addr().unwrap().port()
}
