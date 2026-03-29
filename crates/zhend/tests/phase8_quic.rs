//! Phase 8 integration test — QUIC/HTTP3 edge transport via quinn.
//!
//! Tests Ingest, Surface, and Status operations over QUIC streams
//! using the binary wire protocol.
//! GPL-3.0-only

use std::net::SocketAddr;
use std::sync::Arc;

use tempfile::TempDir;

use zhend::api::quic::{
    build_client_config_insecure, build_server_config, send_request, QuicServer,
    OP_INGEST, OP_STATUS, OP_SURFACE, STATUS_BAD_REQUEST, STATUS_OK,
};
use zhend::de::embedder::Embedder;
use zhend::pu::TieredStore;
use zhend::ZhenConfig;

/// Spin up a QUIC server on a random port and return the client endpoint + server address.
async fn setup() -> (quinn::Endpoint, SocketAddr, TempDir) {
    let tmp = TempDir::new().unwrap();
    let store = Arc::new(TieredStore::open(tmp.path()).unwrap());
    let embedder = Arc::new(Embedder::new(384));

    // Bind server to port 0 for random free port.
    let server_config = build_server_config();
    let server_endpoint =
        quinn::Endpoint::server(server_config, "127.0.0.1:0".parse().unwrap()).unwrap();
    let server_addr = server_endpoint.local_addr().unwrap();

    let config = ZhenConfig {
        data_dir: tmp.path().to_path_buf(),
        quic_addr: server_addr.to_string(),
        ..ZhenConfig::default()
    };

    let server = Arc::new(QuicServer::new(store, embedder, config));

    // Spawn server accept loop.
    tokio::spawn(async move {
        while let Some(incoming) = server_endpoint.accept().await {
            let server = Arc::clone(&server);
            tokio::spawn(async move {
                if let Ok(conn) = incoming.await {
                    while let Ok((send, recv)) = conn.accept_bi().await {
                        let s = Arc::clone(&server);
                        tokio::spawn(async move {
                            s.handle_stream(send, recv).await;
                        });
                    }
                }
            });
        }
    });

    // Build client endpoint (no server config needed).
    let mut client_endpoint =
        quinn::Endpoint::client("127.0.0.1:0".parse().unwrap()).unwrap();
    client_endpoint.set_default_client_config(build_client_config_insecure());

    // Give server a moment.
    tokio::time::sleep(tokio::time::Duration::from_millis(50)).await;

    (client_endpoint, server_addr, tmp)
}

/// Connect to the QUIC server and return the connection.
async fn connect(endpoint: &quinn::Endpoint, addr: SocketAddr) -> quinn::Connection {
    endpoint.connect(addr, "localhost").unwrap().await.unwrap()
}

#[tokio::test]
async fn test_quic_server_accepts_connection() {
    let (client_ep, addr, _tmp) = setup().await;

    // Just connecting and getting a connection object proves the QUIC handshake works.
    let conn = connect(&client_ep, addr).await;
    assert_eq!(conn.remote_address(), addr);

    conn.close(0u32.into(), b"done");
}

#[tokio::test]
async fn test_quic_ingest() {
    let (client_ep, addr, _tmp) = setup().await;
    let conn = connect(&client_ep, addr).await;

    let (mut send, mut recv) = conn.open_bi().await.unwrap();
    let payload = b"the dao that can be named is not the eternal dao";

    let (status, resp) = send_request(&mut send, &mut recv, OP_INGEST, payload)
        .await
        .unwrap();

    assert_eq!(status, STATUS_OK, "ingest should succeed");
    assert_eq!(resp.len(), 33, "response = 32-byte ID + 1-byte novel flag");
    assert_eq!(resp[32], 1, "first ingest should be novel");

    // Duplicate ingest on a new stream.
    let (mut send2, mut recv2) = conn.open_bi().await.unwrap();
    let (status2, resp2) = send_request(&mut send2, &mut recv2, OP_INGEST, payload)
        .await
        .unwrap();

    assert_eq!(status2, STATUS_OK);
    assert_eq!(resp2[32], 0, "duplicate ingest should not be novel");
    assert_eq!(&resp[..32], &resp2[..32], "same payload = same fragment ID");

    conn.close(0u32.into(), b"done");
}

#[tokio::test]
async fn test_quic_ingest_empty_rejected() {
    let (client_ep, addr, _tmp) = setup().await;
    let conn = connect(&client_ep, addr).await;

    let (mut send, mut recv) = conn.open_bi().await.unwrap();
    let (status, _) = send_request(&mut send, &mut recv, OP_INGEST, b"")
        .await
        .unwrap();

    assert_eq!(status, STATUS_BAD_REQUEST, "empty payload should be rejected");

    conn.close(0u32.into(), b"done");
}

#[tokio::test]
async fn test_quic_status() {
    let (client_ep, addr, _tmp) = setup().await;
    let conn = connect(&client_ep, addr).await;

    // Status on empty store.
    let (mut send, mut recv) = conn.open_bi().await.unwrap();
    let (status, resp) = send_request(&mut send, &mut recv, OP_STATUS, b"")
        .await
        .unwrap();

    assert_eq!(status, STATUS_OK);
    let json: serde_json::Value = serde_json::from_slice(&resp).unwrap();
    assert_eq!(json["l1_count"], 0);
    assert_eq!(json["l2_count"], 0);
    assert_eq!(json["l3_count"], 0);
    assert!(json["embedder_ready"].as_bool().unwrap());

    // Ingest a fragment, then check status again.
    let (mut send2, mut recv2) = conn.open_bi().await.unwrap();
    let (s, _) = send_request(&mut send2, &mut recv2, OP_INGEST, b"fragment one")
        .await
        .unwrap();
    assert_eq!(s, STATUS_OK);

    let (mut send3, mut recv3) = conn.open_bi().await.unwrap();
    let (status3, resp3) = send_request(&mut send3, &mut recv3, OP_STATUS, b"")
        .await
        .unwrap();

    assert_eq!(status3, STATUS_OK);
    let json3: serde_json::Value = serde_json::from_slice(&resp3).unwrap();
    assert_eq!(json3["l1_count"], 1, "should have 1 fragment after ingest");

    conn.close(0u32.into(), b"done");
}

#[tokio::test]
async fn test_quic_surface() {
    let (client_ep, addr, _tmp) = setup().await;
    let conn = connect(&client_ep, addr).await;

    // Ingest a fragment with a known embedding.
    let (mut send1, mut recv1) = conn.open_bi().await.unwrap();
    let payload = b"knowledge of the ancients";
    let (s, _) = send_request(&mut send1, &mut recv1, OP_INGEST, payload)
        .await
        .unwrap();
    assert_eq!(s, STATUS_OK);

    // Surface with the hash-derived embedding (same as what embedder produces).
    // The embedder produces a deterministic embedding from the payload hash.
    // For surface, we pass context_embedding bytes — use the same payload to get high De.
    let (mut send2, mut recv2) = conn.open_bi().await.unwrap();
    let embedder = zhend::de::embedder::Embedder::new(384);
    let context_embedding = embedder.embed_bytes(payload, None);

    let (status, resp) = send_request(&mut send2, &mut recv2, OP_SURFACE, &context_embedding)
        .await
        .unwrap();

    assert_eq!(status, STATUS_OK);
    let fragments: Vec<serde_json::Value> = serde_json::from_slice(&resp).unwrap();
    assert_eq!(fragments.len(), 1, "should find the ingested fragment");
    assert!(
        fragments[0]["de_score"].as_f64().unwrap() > 0.99,
        "identical embedding should score ~1.0"
    );

    conn.close(0u32.into(), b"done");
}

#[tokio::test]
async fn test_quic_unknown_op_rejected() {
    let (client_ep, addr, _tmp) = setup().await;
    let conn = connect(&client_ep, addr).await;

    let (mut send, mut recv) = conn.open_bi().await.unwrap();
    let (status, resp) = send_request(&mut send, &mut recv, 255, b"")
        .await
        .unwrap();

    assert_eq!(status, STATUS_BAD_REQUEST);
    assert_eq!(&resp, b"unknown op code");

    conn.close(0u32.into(), b"done");
}

#[tokio::test]
async fn test_quic_multiple_streams_one_connection() {
    let (client_ep, addr, _tmp) = setup().await;
    let conn = connect(&client_ep, addr).await;

    // Open 3 streams in parallel for different ops.
    let mut handles = Vec::new();

    for i in 0..3u8 {
        let conn = conn.clone();
        handles.push(tokio::spawn(async move {
            let (mut send, mut recv) = conn.open_bi().await.unwrap();
            let payload = format!("parallel fragment {i}").into_bytes();
            let (status, resp) =
                send_request(&mut send, &mut recv, OP_INGEST, &payload)
                    .await
                    .unwrap();
            assert_eq!(status, STATUS_OK);
            assert_eq!(resp.len(), 33);
            assert_eq!(resp[32], 1, "each unique fragment should be novel");
        }));
    }

    for h in handles {
        h.await.unwrap();
    }

    // Verify status shows all 3.
    let (mut send, mut recv) = conn.open_bi().await.unwrap();
    let (status, resp) = send_request(&mut send, &mut recv, OP_STATUS, b"")
        .await
        .unwrap();
    assert_eq!(status, STATUS_OK);
    let json: serde_json::Value = serde_json::from_slice(&resp).unwrap();
    assert_eq!(json["l1_count"], 3, "should have 3 fragments after parallel ingest");

    conn.close(0u32.into(), b"done");
}
