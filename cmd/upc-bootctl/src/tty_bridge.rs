// SPDX-License-Identifier: GPL-3.0-or-later
//
// tty_bridge — best-effort POST of TTY bytes captured from TTY_MAP into the
// upc-tty-bridge service's /api/v1/tty/ingest endpoint (default port 26100,
// per pkg/ports/ports.go user-app band).
//
// ASCEND-LINUX Phase 1 SHIP §"Phase 1.1 SHIP decisions 2026-05-10":
//   "TTY transport is HTTP loopback from upc-bootctl runner →
//    upc-tty-bridge on port 26100. New endpoint /api/v1/tty/ingest.
//    Wotan-topic path deferred to Phase 3+."
//
// Endpoint shape (per cmd/upc-tty-bridge/main.go ttyIngestRequest):
//   POST http://<host>:<port>/api/v1/tty/ingest
//   Content-Type: application/json
//   Body: {"instance": <u8>, "bytes": [<int>, <int>, ...]}
//   Response: 204 No Content on success.
//
// All failures are logged but do NOT abort the boot path — the kernel can
// still run on the host without the browser xterm being attached.

use serde::Serialize;
use std::time::Duration;

const DEFAULT_HOST: &str = "127.0.0.1";
pub const DEFAULT_PORT: u16 = 26100;
const POST_TIMEOUT: Duration = Duration::from_millis(500);

#[derive(Serialize)]
struct IngestRequest<'a> {
    instance: u8,
    bytes: &'a [i32],
}

/// POST `bytes` to the tty-bridge ingest endpoint for `instance`.
/// Honors `UPC_TTY_BRIDGE_HOST` / `UPC_TTY_BRIDGE_PORT` env overrides for
/// non-default deployments (e.g. the bridge running on EAST while bootctl
/// runs on WEST). Best-effort: logs and returns Ok(()) on transport
/// failures — boot path must not block on bridge availability.
pub fn post_ingest(instance: u8, bytes: &[u8]) {
    if bytes.is_empty() {
        return;
    }
    let host = std::env::var("UPC_TTY_BRIDGE_HOST").unwrap_or_else(|_| DEFAULT_HOST.to_string());
    let port = std::env::var("UPC_TTY_BRIDGE_PORT")
        .ok()
        .and_then(|s| s.parse::<u16>().ok())
        .unwrap_or(DEFAULT_PORT);
    let url = format!("http://{}:{}/api/v1/tty/ingest", host, port);

    // Bridge expects JSON-array-of-numbers (NOT base64) so the payload is
    // human-readable in curl/logs during dev, per main.go comment.
    let bytes_i32: Vec<i32> = bytes.iter().map(|&b| b as i32).collect();
    let req = IngestRequest {
        instance,
        bytes: &bytes_i32,
    };

    let client = match reqwest::blocking::Client::builder()
        .timeout(POST_TIMEOUT)
        .build()
    {
        Ok(c) => c,
        Err(e) => {
            eprintln!("[upc-bootctl] tty-bridge: client build failed: {e}");
            return;
        }
    };

    match client.post(&url).json(&req).send() {
        Ok(resp) if resp.status().as_u16() == 204 => {
            eprintln!(
                "[upc-bootctl] tty-bridge: posted {} bytes to instance 0x{:02X}",
                bytes.len(),
                instance
            );
        }
        Ok(resp) => {
            eprintln!(
                "[upc-bootctl] tty-bridge: unexpected status {} from {}",
                resp.status(),
                url
            );
        }
        Err(e) => {
            // Bridge not running is expected in many local-dev flows.
            // Log at info level only; do NOT escalate.
            eprintln!(
                "[upc-bootctl] tty-bridge unreachable at {} ({}); skipping (dev OK)",
                url, e
            );
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_bytes_is_noop() {
        // No HTTP call should fire; if a bridge isn't running this would
        // print an error otherwise. Just verify it doesn't panic.
        post_ingest(0xDE, &[]);
    }

    #[test]
    fn ingest_request_serializes_to_expected_shape() {
        let bytes_i32: Vec<i32> = vec![0x78, 0x76, 0x36];
        let req = IngestRequest {
            instance: 0xDE,
            bytes: &bytes_i32,
        };
        let json = serde_json::to_string(&req).unwrap();
        assert_eq!(json, r#"{"instance":222,"bytes":[120,118,54]}"#);
    }
}
