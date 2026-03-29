//! gRPC service implementation for zhend.
//!
//! Uses tonic for gRPC over HTTP/2. Internal Kingdom transport.
//! Service definition will come from proto file — for now, manual impl.

use std::sync::Arc;
use crate::pu::TieredStore;
use crate::de::embedder::Embedder;
use crate::ZhenConfig;

/// Core Zhen gRPC service.
pub struct ZhenService {
    #[allow(dead_code)]
    store: Arc<TieredStore>,
    #[allow(dead_code)]
    embedder: Arc<Embedder>,
    #[allow(dead_code)]
    config: ZhenConfig,
}

impl ZhenService {
    pub fn new(store: Arc<TieredStore>, embedder: Arc<Embedder>, config: ZhenConfig) -> Self {
        Self { store, embedder, config }
    }
}

// TODO: generate from proto/zhen.proto via tonic-build
//
// service Zhen {
//   // Submit a new fragment (Pu).
//   rpc Ingest(IngestRequest) returns (IngestResponse);
//
//   // Surface relevant fragments for a context vector (De).
//   rpc Surface(SurfaceRequest) returns (SurfaceResponse);
//
//   // Node health and strata snapshot.
//   rpc Status(StatusRequest) returns (StatusResponse);
//
//   // Initiate deep retrieval from Jing.
//   rpc Pilgrimage(PilgrimageRequest) returns (stream PilgrimageUpdate);
//
//   // Stream fragment digests for gossip sync.
//   rpc SyncDigests(stream DigestBatch) returns (stream WantBatch);
// }

/// Start the gRPC server.
pub async fn serve(
    _service: ZhenService,
    addr: &str,
) -> Result<(), Box<dyn std::error::Error>> {
    let addr: std::net::SocketAddr = addr.parse()?;

    tracing::info!(%addr, "zhend gRPC server starting");

    // TODO: register generated service with tonic Server
    // For now, placeholder.
    //
    // tonic::transport::Server::builder()
    //     .add_service(ZhenServer::new(service))
    //     .serve(addr)
    //     .await?;

    Ok(())
}
