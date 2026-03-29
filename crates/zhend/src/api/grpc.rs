//! gRPC service implementation for zhend.
//!
//! Implements Ingest, Surface, and Status RPCs backed by real TieredStore
//! and Embedder. Generated service trait from proto/zhen.proto via tonic-build.
//!
//! GPL-3.0-only

use std::sync::Arc;
use std::time::Instant;
use tonic::{Request, Response, Status};

use crate::de::embedder::Embedder;
use crate::de::similarity::rank_by_de;
use crate::proto::zhen::v1 as pb;
use crate::proto::zhen::v1::zhen_server::{Zhen, ZhenServer};
use crate::pu::{Fragment, TieredStore};
use crate::ZhenConfig;

/// Core Zhen gRPC service.
pub struct ZhenService {
    store: Arc<TieredStore>,
    embedder: Arc<Embedder>,
    config: ZhenConfig,
    started_at: Instant,
}

impl ZhenService {
    pub fn new(store: Arc<TieredStore>, embedder: Arc<Embedder>, config: ZhenConfig) -> Self {
        Self {
            store,
            embedder,
            config,
            started_at: Instant::now(),
        }
    }

    /// Build a tonic `ZhenServer` ready for registration with a tonic transport Server.
    pub fn into_server(self) -> ZhenServer<Self> {
        ZhenServer::new(self)
    }
}

#[tonic::async_trait]
impl Zhen for ZhenService {
    /// Ingest a new fragment (Pu). Accepts payload + optional embedding,
    /// creates Fragment, ingests into TieredStore. Returns fragment_id + novel flag.
    async fn ingest(
        &self,
        request: Request<pb::IngestRequest>,
    ) -> Result<Response<pb::IngestResponse>, Status> {
        let req = request.into_inner();

        if req.payload.is_empty() {
            return Err(Status::invalid_argument("payload must not be empty"));
        }

        // Use provided embedding or generate one.
        let embedding = if req.embedding.is_empty() {
            self.embedder.embed_bytes(&req.payload, None)
        } else {
            req.embedding
        };

        let content_type = if req.content_type.is_empty() {
            None
        } else {
            Some(req.content_type)
        };

        let fragment = Fragment::new(req.payload, embedding, content_type);
        let fragment_id = fragment.id.0.to_vec();

        match self.store.ingest(fragment) {
            Ok(novel) => Ok(Response::new(pb::IngestResponse {
                fragment_id,
                novel,
            })),
            Err(e) => Err(Status::internal(format!("ingest failed: {e}"))),
        }
    }

    /// Surface De-ranked fragments for a given context embedding.
    async fn surface(
        &self,
        request: Request<pb::SurfaceRequest>,
    ) -> Result<Response<pb::SurfaceResponse>, Status> {
        let req = request.into_inner();

        let top_k = if req.top_k == 0 { 10 } else { req.top_k as usize };
        let min_de = req.min_de;

        // Get all L1 fragments for ranking.
        let fragments = self.store.l1_fragments().map_err(|e| {
            Status::internal(format!("failed to read fragments: {e}"))
        })?;

        let ranked = rank_by_de(&req.context_embedding, &fragments, top_k);

        let scored: Vec<pb::ScoredFragment> = ranked
            .into_iter()
            .filter(|(_, score)| *score >= min_de)
            .map(|(frag, score)| pb::ScoredFragment {
                fragment_id: frag.id.0.to_vec(),
                payload: frag.payload.clone(),
                de_score: score,
                tier: match frag.tier {
                    crate::pu::Tier::Hot => pb::Tier::Hot as i32,
                    crate::pu::Tier::Warm => pb::Tier::Warm as i32,
                    crate::pu::Tier::Cold => pb::Tier::Cold as i32,
                },
                access_count: frag.access_count,
            })
            .collect();

        Ok(Response::new(pb::SurfaceResponse { fragments: scored }))
    }

    /// Node health, strata snapshot, gossip stats, uptime, embedder status.
    async fn status(
        &self,
        _request: Request<pb::StatusRequest>,
    ) -> Result<Response<pb::StatusResponse>, Status> {
        let l1 = self.store.l1_count().map_err(|e| {
            Status::internal(format!("failed to read L1 count: {e}"))
        })? as u64;

        // L2 count from sled.
        let l2 = self.store.l2_count() as u64;

        let l3 = self.store.l3_count().map_err(|e| {
            Status::internal(format!("failed to read L3 count: {e}"))
        })? as u64;

        let uptime = self.started_at.elapsed().as_secs();

        Ok(Response::new(pb::StatusResponse {
            node_id: self.config.grpc_addr.clone(),
            strata: Some(pb::StrataSnapshot {
                l1_count: l1,
                l2_count: l2,
                l3_count: l3,
                total_bytes: 0, // TODO: compute actual byte sizes
            }),
            gossip: Some(pb::GossipStats {
                peer_count: 0,
                alive_peers: 0,
                digests_sent: 0,
                fragments_exchanged: 0,
            }),
            uptime_secs: uptime,
            embedder_ready: self.embedder.is_ready(),
        }))
    }

    /// Pilgrimage — deep Jing retrieval. Streams progress.
    type PilgrimageStream = tokio_stream::wrappers::ReceiverStream<Result<pb::PilgrimageUpdate, Status>>;

    async fn pilgrimage(
        &self,
        request: Request<pb::PilgrimageRequest>,
    ) -> Result<Response<Self::PilgrimageStream>, Status> {
        let req = request.into_inner();
        let (tx, rx) = tokio::sync::mpsc::channel(4);

        let store = Arc::clone(&self.store);

        tokio::spawn(async move {
            // Try to find the fragment.
            let _ = tx.send(Ok(pb::PilgrimageUpdate {
                state: pb::PilgrimageState::Scanning as i32,
                entries_scanned: 0,
                payload: vec![],
            })).await;

            if req.fragment_id.len() != 32 {
                let _ = tx.send(Ok(pb::PilgrimageUpdate {
                    state: pb::PilgrimageState::Failed as i32,
                    entries_scanned: 0,
                    payload: vec![],
                })).await;
                return;
            }

            let mut id_bytes = [0u8; 32];
            id_bytes.copy_from_slice(&req.fragment_id);
            let id = crate::pu::FragmentId(id_bytes);

            match store.get(&id) {
                Ok(Some(frag)) => {
                    let _ = tx.send(Ok(pb::PilgrimageUpdate {
                        state: pb::PilgrimageState::Found as i32,
                        entries_scanned: 1,
                        payload: frag.payload,
                    })).await;
                }
                Ok(None) => {
                    let _ = tx.send(Ok(pb::PilgrimageUpdate {
                        state: pb::PilgrimageState::NotFoundLocally as i32,
                        entries_scanned: 1,
                        payload: vec![],
                    })).await;
                }
                Err(_) => {
                    let _ = tx.send(Ok(pb::PilgrimageUpdate {
                        state: pb::PilgrimageState::Failed as i32,
                        entries_scanned: 0,
                        payload: vec![],
                    })).await;
                }
            }
        });

        Ok(Response::new(tokio_stream::wrappers::ReceiverStream::new(rx)))
    }

    /// SyncDigests — gossip sync. Bidirectional streaming.
    type SyncDigestsStream = tokio_stream::wrappers::ReceiverStream<Result<pb::WantBatch, Status>>;

    async fn sync_digests(
        &self,
        request: Request<tonic::Streaming<pb::DigestBatch>>,
    ) -> Result<Response<Self::SyncDigestsStream>, Status> {
        let mut stream = request.into_inner();
        let (tx, rx) = tokio::sync::mpsc::channel(4);
        let store = Arc::clone(&self.store);

        tokio::spawn(async move {
            while let Ok(Some(batch)) = stream.message().await {
                let mut wanted = Vec::new();
                for digest in &batch.fragment_ids {
                    if digest.len() == 32 {
                        let mut id_bytes = [0u8; 32];
                        id_bytes.copy_from_slice(digest);
                        let id = crate::pu::FragmentId(id_bytes);
                        if let Ok(false) = store.contains(&id) {
                            wanted.push(digest.clone());
                        }
                    }
                }
                if tx.send(Ok(pb::WantBatch { wanted_ids: wanted })).await.is_err() {
                    break;
                }
            }
        });

        Ok(Response::new(tokio_stream::wrappers::ReceiverStream::new(rx)))
    }
}

/// Start the gRPC server on the given address.
pub async fn serve(
    service: ZhenService,
    addr: &str,
) -> Result<(), Box<dyn std::error::Error>> {
    let addr: std::net::SocketAddr = addr.parse()?;

    tracing::info!(%addr, "zhend gRPC server starting");

    tonic::transport::Server::builder()
        .add_service(service.into_server())
        .serve(addr)
        .await?;

    Ok(())
}
