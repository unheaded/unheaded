// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.
//! trace-collector's Wotan service identity (ADR-103 §6, S-IRON-GATE
//! Phase 7): every gRPC request to Wotan carries
//! `authorization: Bearer <EdDSA token, sub=trace-collector, aud=wotan>`,
//! minted by crates/svc-token exactly as Go's pkg/auth mints it.
//!
//! The key is `AUTH_KEY_FILE`, else `/etc/unheaded/keys/trace-collector.key`
//! (`unheaded-keygen trace-collector`). It is loaded once per process. A
//! missing key logs one warning and requests go without a token (a Wotan
//! with AUTH_ENABLED=true refuses them); a key that is there but unusable
//! (group/other-readable, not a seed PEM) logs one error and does the same,
//! like the Go clients' `wotanClient.InitServiceIdentity`.

use std::sync::{Arc, OnceLock};

use svc_token::{Identity, Minter, WOTAN_AUDIENCE};
use tonic::metadata::MetadataValue;
use tonic::service::Interceptor;
use tonic::{Request, Status};
use tracing::{error, warn};

/// trace-collector's auth identity: its key's trust.d name, the "sub" of
/// its tokens, and the name on Wotan's AUTH_ALLOW_CALLERS and auto_approve.
pub const IDENTITY: &str = "trace-collector";

/// Attaches the service token to every request (a tonic interceptor).
/// Cloning shares the minter and its token cache.
#[derive(Clone, Default)]
pub struct WotanAuth {
    minter: Option<Arc<Minter>>,
}

impl std::fmt::Debug for WotanAuth {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("WotanAuth")
            .field("minter", &self.minter)
            .finish()
    }
}

static PROCESS: OnceLock<WotanAuth> = OnceLock::new();

impl WotanAuth {
    /// The process identity, loaded (and its warning logged) on first use.
    pub fn process() -> Self {
        PROCESS
            .get_or_init(|| match svc_token::load_identity(IDENTITY) {
                Ok(Identity::Loaded(m)) => Self::with_minter(m),
                Ok(Identity::Missing(path)) => {
                    warn!("{}", svc_token::missing_key_warning(IDENTITY, &path));
                    Self::default()
                }
                Err(e) => {
                    error!(error = %e, "Wotan calls carry no token as {IDENTITY:?} (ADR-103)");
                    Self::default()
                }
            })
            .clone()
    }

    /// An authorizer presenting m's tokens.
    pub fn with_minter(m: Minter) -> Self {
        Self {
            minter: Some(Arc::new(m)),
        }
    }

    /// Whether requests carry a token.
    pub fn has_identity(&self) -> bool {
        self.minter.is_some()
    }

    /// The authorization metadata value for Wotan, None without a key.
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

#[cfg(test)]
mod tests {
    use super::*;

    fn minter() -> Minter {
        let mut seed = [0u8; 32];
        for (i, b) in seed.iter_mut().enumerate() {
            *b = (i as u8).wrapping_mul(13).wrapping_add(7);
        }
        Minter::from_seed(IDENTITY, &seed).unwrap()
    }

    #[test]
    fn interceptor_attaches_bearer_token_for_wotan() {
        let m = minter();
        let kid = m.kid().to_string();
        let mut auth = WotanAuth::with_minter(m);
        assert!(auth.has_identity());
        let req = auth.call(Request::new(())).unwrap();
        let h = req
            .metadata()
            .get("authorization")
            .expect("authorization metadata")
            .to_str()
            .unwrap()
            .to_string();
        let tok = h.strip_prefix("Bearer ").expect("Bearer scheme");
        let parts: Vec<&str> = tok.split('.').collect();
        assert_eq!(parts.len(), 3);
        use base64::Engine as _;
        let dec = |s: &str| {
            String::from_utf8(
                base64::engine::general_purpose::URL_SAFE_NO_PAD
                    .decode(s)
                    .unwrap(),
            )
            .unwrap()
        };
        assert!(dec(parts[0]).contains(&format!(r#""kid":"{kid}""#)));
        let claims = dec(parts[1]);
        assert!(claims.starts_with(r#"{"sub":"trace-collector","iss":"unheaded","aud":"wotan","#));
        // Cached: the next request carries the same token.
        let again = auth.call(Request::new(())).unwrap();
        assert_eq!(
            again
                .metadata()
                .get("authorization")
                .unwrap()
                .to_str()
                .unwrap(),
            h
        );
    }

    #[test]
    fn no_identity_leaves_request_alone() {
        let mut auth = WotanAuth::default();
        assert!(!auth.has_identity());
        let req = auth.call(Request::new(())).unwrap();
        assert!(req.metadata().get("authorization").is_none());
    }
}
