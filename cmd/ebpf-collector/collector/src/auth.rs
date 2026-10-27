// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.
//! ebpf-collector's Wotan service identity (ADR-103 §6, S-IRON-GATE
//! Phase 7). Every request to Wotan, gRPC and HTTP, carries
//! `Bearer <EdDSA token, sub=ebpf-collector, aud=wotan>`, minted by
//! crates/svc-token exactly as Go's pkg/auth mints it.
//!
//! The key is `AUTH_KEY_FILE`, else `/etc/unheaded/keys/ebpf-collector.key`
//! (`unheaded-keygen ebpf-collector`), loaded once at start. A missing key
//! logs one warning and requests go without a token (a Wotan with
//! AUTH_ENABLED=true refuses them); a key that is there but unusable logs
//! one error and does the same, like the Go clients'
//! `wotanClient.InitServiceIdentity`. Wotan allows the identity in
//! AUTH_ALLOW_CALLERS and approves it in configs/wotan.yaml auto_approve.

use std::sync::Arc;

use log::{error, warn};
use svc_token::{Identity, Minter, WOTAN_AUDIENCE};
use tonic::metadata::MetadataValue;
use tonic::service::Interceptor;
use tonic::{Request, Status};

/// ebpf-collector's auth identity: its key's trust.d name and the "sub" of
/// its tokens; also the display_name it publishes under.
pub const IDENTITY: &str = "ebpf-collector";

/// Authorizes requests to Wotan; cloning shares the minter and its cache.
#[derive(Clone, Default, Debug)]
pub struct WotanAuth {
    minter: Option<Arc<Minter>>,
}

impl WotanAuth {
    /// Loads the identity, logging a missing or unusable key once.
    pub fn load() -> Self {
        match svc_token::load_identity(IDENTITY) {
            Ok(Identity::Loaded(m)) => Self::with_minter(m),
            Ok(Identity::Missing(path)) => {
                warn!("{}", svc_token::missing_key_warning(IDENTITY, &path));
                Self::default()
            }
            Err(e) => {
                error!("{e}; Wotan calls carry no token as {IDENTITY:?} (ADR-103)");
                Self::default()
            }
        }
    }

    /// An authorizer presenting m's tokens.
    pub fn with_minter(m: Minter) -> Self {
        Self {
            minter: Some(Arc::new(m)),
        }
    }

    /// The Authorization value for Wotan, None without a key.
    pub fn header(&self) -> Result<Option<String>, svc_token::Error> {
        match &self.minter {
            None => Ok(None),
            Some(m) => m.authorization(WOTAN_AUDIENCE).map(Some),
        }
    }
}

impl Interceptor for WotanAuth {
    fn call(&mut self, mut req: Request<()>) -> Result<Request<()>, Status> {
        let header = self
            .header()
            .map_err(|e| Status::unauthenticated(format!("wotan service token: {e}")))?;
        if let Some(h) = header {
            let v: MetadataValue<_> = h
                .parse()
                .map_err(|_| Status::internal("service token is not valid metadata"))?;
            req.metadata_mut().insert("authorization", v);
        }
        Ok(req)
    }
}
