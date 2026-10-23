// SPDX-License-Identifier: GPL-3.0-or-later
use std::sync::Arc;

pub fn server() {
    let provider = rustls::crypto::aws_lc_rs::default_provider();
    let _b = rustls::ServerConfig::builder_with_provider(Arc::new(provider));
}
