//! Phase 6 integration test — gRPC service via tonic client.
//!
//! Tests Ingest, Surface, and Status RPCs end-to-end.
//! GPL-3.0-only

use std::sync::Arc;
use tempfile::TempDir;
use tokio::net::TcpListener;
use tonic::transport::{Channel, Server};

use zhend::api::grpc::ZhenService;
use zhend::de::embedder::Embedder;
use zhend::proto::zhen::v1::zhen_client::ZhenClient;
use zhend::proto::zhen::v1::{IngestRequest, StatusRequest, SurfaceRequest};
use zhend::pu::TieredStore;
use zhend::ZhenConfig;

/// Spin up a real gRPC server on a random port and return the client channel.
async fn setup() -> (ZhenClient<Channel>, TempDir) {
    let tmp = TempDir::new().unwrap();
    let store = Arc::new(TieredStore::open(tmp.path()).unwrap());
    let embedder = Arc::new(Embedder::new(384));

    // Bind to port 0 to get a random free port.
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    let config = ZhenConfig {
        data_dir: tmp.path().to_path_buf(),
        grpc_addr: addr.to_string(),
        ..ZhenConfig::default()
    };

    let service = ZhenService::new(store, embedder, config);
    let incoming = tokio_stream::wrappers::TcpListenerStream::new(listener);

    // Spawn server.
    tokio::spawn(async move {
        Server::builder()
            .add_service(service.into_server())
            .serve_with_incoming(incoming)
            .await
            .unwrap();
    });

    // Give the server a moment to start accepting.
    tokio::time::sleep(tokio::time::Duration::from_millis(50)).await;

    let channel = Channel::from_shared(format!("http://{addr}"))
        .unwrap()
        .connect()
        .await
        .unwrap();

    let client = ZhenClient::new(channel);
    (client, tmp)
}

#[tokio::test]
async fn test_ingest_returns_fragment_id_and_novel() {
    let (mut client, _tmp) = setup().await;

    let resp = client
        .ingest(IngestRequest {
            payload: b"the dao that can be named is not the eternal dao".to_vec(),
            embedding: vec![],
            content_type: "text/plain".into(),
        })
        .await
        .unwrap()
        .into_inner();

    assert_eq!(resp.fragment_id.len(), 32, "fragment_id must be 32 bytes (BLAKE3)");
    assert!(resp.novel, "first ingest must be novel");

    // Duplicate ingest.
    let resp2 = client
        .ingest(IngestRequest {
            payload: b"the dao that can be named is not the eternal dao".to_vec(),
            embedding: vec![],
            content_type: "text/plain".into(),
        })
        .await
        .unwrap()
        .into_inner();

    assert!(!resp2.novel, "duplicate ingest must not be novel");
    assert_eq!(resp.fragment_id, resp2.fragment_id, "same payload = same id");
}

#[tokio::test]
async fn test_ingest_empty_payload_rejected() {
    let (mut client, _tmp) = setup().await;

    let result = client
        .ingest(IngestRequest {
            payload: vec![],
            embedding: vec![],
            content_type: String::new(),
        })
        .await;

    assert!(result.is_err(), "empty payload should be rejected");
    let status = result.unwrap_err();
    assert_eq!(status.code(), tonic::Code::InvalidArgument);
}

#[tokio::test]
async fn test_surface_returns_ingested_fragment() {
    let (mut client, _tmp) = setup().await;

    // Ingest a fragment with a known embedding.
    let embedding = vec![100u8, 200, 50, 150];
    let payload = b"knowledge of the ancients".to_vec();

    let ingest_resp = client
        .ingest(IngestRequest {
            payload: payload.clone(),
            embedding: embedding.clone(),
            content_type: String::new(),
        })
        .await
        .unwrap()
        .into_inner();

    assert!(ingest_resp.novel);

    // Surface with the same embedding — should find our fragment.
    let surface_resp = client
        .surface(SurfaceRequest {
            context_embedding: embedding,
            top_k: 10,
            min_de: 0.0,
        })
        .await
        .unwrap()
        .into_inner();

    assert_eq!(surface_resp.fragments.len(), 1);
    assert_eq!(surface_resp.fragments[0].payload, payload);
    assert!(surface_resp.fragments[0].de_score > 0.99, "identical embedding should score ~1.0");
}

#[tokio::test]
async fn test_status_returns_strata_snapshot() {
    let (mut client, _tmp) = setup().await;

    // Status on empty store.
    let resp = client
        .status(StatusRequest {})
        .await
        .unwrap()
        .into_inner();

    assert!(resp.strata.is_some());
    let strata = resp.strata.unwrap();
    assert_eq!(strata.l1_count, 0);
    assert_eq!(strata.l2_count, 0);
    assert_eq!(strata.l3_count, 0);
    assert!(resp.embedder_ready, "hash-based embedder should be ready");
    assert!(resp.uptime_secs < 10, "uptime should be low in test");

    // Ingest a fragment and check status again.
    client
        .ingest(IngestRequest {
            payload: b"fragment one".to_vec(),
            embedding: vec![],
            content_type: String::new(),
        })
        .await
        .unwrap();

    let resp2 = client
        .status(StatusRequest {})
        .await
        .unwrap()
        .into_inner();

    let strata2 = resp2.strata.unwrap();
    assert_eq!(strata2.l1_count, 1, "should have 1 fragment in L1 after ingest");
}

#[tokio::test]
async fn test_full_ingest_surface_status_cycle() {
    let (mut client, _tmp) = setup().await;

    // Ingest 3 fragments with embeddings.
    for i in 0..3u8 {
        let payload = format!("fragment number {i}").into_bytes();
        let embedding = vec![i * 50 + 10, i * 30 + 20, i * 10 + 100, 200 - i * 40];
        client
            .ingest(IngestRequest {
                payload,
                embedding,
                content_type: "text/plain".into(),
            })
            .await
            .unwrap();
    }

    // Status should show 3 in L1.
    let status = client
        .status(StatusRequest {})
        .await
        .unwrap()
        .into_inner();
    assert_eq!(status.strata.unwrap().l1_count, 3);

    // Surface with top_k=2 to test limiting.
    let surface = client
        .surface(SurfaceRequest {
            context_embedding: vec![10, 20, 100, 200], // close to fragment 0
            top_k: 2,
            min_de: 0.0,
        })
        .await
        .unwrap()
        .into_inner();

    assert!(
        surface.fragments.len() <= 2,
        "top_k=2 should return at most 2 fragments"
    );
}
