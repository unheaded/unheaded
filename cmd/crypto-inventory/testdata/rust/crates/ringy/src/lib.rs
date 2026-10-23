// SPDX-License-Identifier: GPL-3.0-or-later
use std::sync::Arc;

pub fn server(versions: &[&'static rustls::SupportedProtocolVersion]) {
    let _b = rustls::ServerConfig::builder_with_protocol_versions(versions);
}

pub fn client() {
    let provider = rustls::crypto::ring::default_provider();
    let _b = rustls::ClientConfig::builder_with_provider(Arc::new(provider));
}
