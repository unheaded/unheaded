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
//
// Transport (2026-10-30): plain HTTP/1.0 over std sockets, no HTTP crate. A
// boot inside netns upc0 cannot reach the host's 127.0.0.1, so
// UPC_TTY_BRIDGE_SOCKET names the bridge's unix socket (upc-tty-bridge
// --listen-unix): a filesystem path reaches across network namespaces.
// Otherwise UPC_TTY_BRIDGE_HOST / UPC_TTY_BRIDGE_PORT, default
// 127.0.0.1:26100. HTTP/1.0 makes the server end the body by closing the
// connection, so no chunked decoding is needed.

use serde::Serialize;
use std::io::{Read, Write};
use std::net::{TcpStream, ToSocketAddrs};
use std::os::unix::net::UnixStream;
use std::time::Duration;

const DEFAULT_HOST: &str = "127.0.0.1";
pub const DEFAULT_PORT: u16 = 26100;
const POST_TIMEOUT: Duration = Duration::from_millis(500);
/// Largest response read from the bridge (a kbd drain is at most a few
/// hundred bytes; ingest answers 204 with no body).
const MAX_RESPONSE: u64 = 64 * 1024;

/// Where the bridge listens.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Endpoint {
    Tcp(String, u16),
    Unix(String),
}

impl Endpoint {
    /// UPC_TTY_BRIDGE_SOCKET wins; else UPC_TTY_BRIDGE_HOST/PORT.
    pub fn from_env() -> Self {
        Self::from_vars(
            std::env::var("UPC_TTY_BRIDGE_SOCKET").ok(),
            std::env::var("UPC_TTY_BRIDGE_HOST").ok(),
            std::env::var("UPC_TTY_BRIDGE_PORT").ok(),
        )
    }

    fn from_vars(socket: Option<String>, host: Option<String>, port: Option<String>) -> Self {
        if let Some(s) = socket.filter(|s| !s.is_empty()) {
            return Endpoint::Unix(s);
        }
        let host = host
            .filter(|h| !h.is_empty())
            .unwrap_or_else(|| DEFAULT_HOST.to_string());
        let port = port
            .and_then(|p| p.parse::<u16>().ok())
            .unwrap_or(DEFAULT_PORT);
        Endpoint::Tcp(host, port)
    }

    fn describe(&self, path: &str) -> String {
        match self {
            Endpoint::Tcp(h, p) => format!("http://{h}:{p}{path}"),
            Endpoint::Unix(s) => format!("unix:{s}{path}"),
        }
    }

    /// One request, one connection. Returns (status, body).
    fn request(
        &self,
        method: &str,
        path: &str,
        json: Option<&[u8]>,
    ) -> anyhow::Result<(u16, Vec<u8>)> {
        let host_hdr = match self {
            Endpoint::Tcp(h, p) => format!("{h}:{p}"),
            Endpoint::Unix(_) => "localhost".to_string(),
        };
        let mut req = format!("{method} {path} HTTP/1.0\r\nHost: {host_hdr}\r\n");
        if let Some(b) = json {
            req.push_str(&format!(
                "Content-Type: application/json\r\nContent-Length: {}\r\n",
                b.len()
            ));
        }
        req.push_str("\r\n");
        let mut buf = req.into_bytes();
        if let Some(b) = json {
            buf.extend_from_slice(b);
        }
        let raw = match self {
            Endpoint::Tcp(h, p) => {
                let addr = (h.as_str(), *p)
                    .to_socket_addrs()?
                    .next()
                    .ok_or_else(|| anyhow::anyhow!("{h}: no address"))?;
                let mut s = TcpStream::connect_timeout(&addr, POST_TIMEOUT)?;
                exchange(&mut s, &buf, |s, t| {
                    s.set_read_timeout(t)?;
                    s.set_write_timeout(t)
                })?
            }
            Endpoint::Unix(p) => {
                let mut s = UnixStream::connect(p)?;
                exchange(&mut s, &buf, |s, t| {
                    s.set_read_timeout(t)?;
                    s.set_write_timeout(t)
                })?
            }
        };
        parse_response(&raw)
    }
}

fn exchange<S: Read + Write>(
    s: &mut S,
    req: &[u8],
    timeouts: impl Fn(&S, Option<Duration>) -> std::io::Result<()>,
) -> anyhow::Result<Vec<u8>> {
    timeouts(s, Some(POST_TIMEOUT))?;
    s.write_all(req)?;
    let mut raw = Vec::new();
    s.take(MAX_RESPONSE + 1).read_to_end(&mut raw)?;
    if raw.len() as u64 > MAX_RESPONSE {
        anyhow::bail!("response larger than {MAX_RESPONSE} bytes");
    }
    Ok(raw)
}

/// Split an HTTP/1.x response into status and body. A Content-Length, when
/// present, must not exceed what arrived; the body is cut to it.
pub fn parse_response(raw: &[u8]) -> anyhow::Result<(u16, Vec<u8>)> {
    let end = raw
        .windows(4)
        .position(|w| w == b"\r\n\r\n")
        .ok_or_else(|| anyhow::anyhow!("no end of headers"))?;
    let head =
        std::str::from_utf8(&raw[..end]).map_err(|_| anyhow::anyhow!("headers not UTF-8"))?;
    let mut lines = head.split("\r\n");
    let status_line = lines.next().unwrap_or_default();
    let mut parts = status_line.splitn(3, ' ');
    let (proto, code) = (
        parts.next().unwrap_or_default(),
        parts.next().unwrap_or_default(),
    );
    if !proto.starts_with("HTTP/1.") || code.len() != 3 {
        anyhow::bail!("bad status line {status_line:?}");
    }
    let status: u16 = code
        .parse()
        .map_err(|_| anyhow::anyhow!("bad status code {code:?}"))?;
    let mut body = raw[end + 4..].to_vec();
    for l in lines {
        if let Some((k, v)) = l.split_once(':') {
            if k.trim().eq_ignore_ascii_case("content-length") {
                let n: usize = v
                    .trim()
                    .parse()
                    .map_err(|_| anyhow::anyhow!("bad Content-Length {v:?}"))?;
                if n > body.len() {
                    anyhow::bail!("body short: {} of {n} bytes", body.len());
                }
                body.truncate(n);
            } else if k.trim().eq_ignore_ascii_case("transfer-encoding") {
                anyhow::bail!("chunked response to an HTTP/1.0 request");
            }
        }
    }
    Ok((status, body))
}

#[derive(Serialize)]
struct IngestRequest<'a> {
    instance: u8,
    bytes: &'a [i32],
}

/// POST `bytes` to the tty-bridge ingest endpoint for `instance`.
/// Honors `UPC_TTY_BRIDGE_SOCKET` (unix socket, for boots inside a netns) or
/// `UPC_TTY_BRIDGE_HOST` / `UPC_TTY_BRIDGE_PORT` (e.g. the bridge running on
/// EAST while bootctl runs on WEST). Best-effort: logs and returns Ok(()) on transport
/// failures — boot path must not block on bridge availability.
pub fn post_ingest(instance: u8, bytes: &[u8]) {
    if bytes.is_empty() {
        return;
    }
    let ep = Endpoint::from_env();
    let url = ep.describe("/api/v1/tty/ingest");

    // Bridge expects JSON-array-of-numbers (NOT base64) so the payload is
    // human-readable in curl/logs during dev, per main.go comment.
    let bytes_i32: Vec<i32> = bytes.iter().map(|&b| b as i32).collect();
    let req = IngestRequest {
        instance,
        bytes: &bytes_i32,
    };
    let body = match serde_json::to_vec(&req) {
        Ok(b) => b,
        Err(e) => {
            eprintln!("[upc-bootctl] tty-bridge: encode failed: {e}");
            return;
        }
    };

    match ep.request("POST", "/api/v1/tty/ingest", Some(&body)) {
        Ok((204, _)) => {
            eprintln!(
                "[upc-bootctl] tty-bridge: posted {} bytes to instance 0x{:02X}",
                bytes.len(),
                instance
            );
        }
        Ok((status, _)) => {
            eprintln!("[upc-bootctl] tty-bridge: unexpected status {status} from {url}");
        }
        Err(e) => {
            // Bridge not running is expected in many local-dev flows.
            // Log at info level only; do NOT escalate.
            eprintln!("[upc-bootctl] tty-bridge unreachable at {url} ({e}); skipping (dev OK)");
        }
    }
}

/// Parse the bridge's `/api/v1/kbd/drain` body (a JSON array of byte values).
/// Rejects anything that is not an array of integers in 0..=255, and more than
/// `max` entries (the bridge must never hand back more than was asked for).
pub fn parse_drain_body(body: &str, max: usize) -> anyhow::Result<Vec<u8>> {
    let vals: Vec<i64> =
        serde_json::from_str(body).map_err(|e| anyhow::anyhow!("kbd drain: bad JSON: {e}"))?;
    if vals.len() > max {
        anyhow::bail!("kbd drain: {} bytes returned, asked for {max}", vals.len());
    }
    vals.iter()
        .enumerate()
        .map(|(i, &v)| {
            u8::try_from(v).map_err(|_| anyhow::anyhow!("kbd drain: byte {i} out of range: {v}"))
        })
        .collect()
}

/// Quiet client for the interactive console loop: ingest TTY bytes and drain
/// queued keystrokes. Errors are returned (not printed) so the loop can back
/// off instead of logging every batch.
pub struct BridgeClient {
    ep: Endpoint,
}

impl BridgeClient {
    pub fn from_env() -> anyhow::Result<Self> {
        Ok(Self {
            ep: Endpoint::from_env(),
        })
    }

    pub fn ingest(&self, instance: u8, bytes: &[u8]) -> anyhow::Result<()> {
        if bytes.is_empty() {
            return Ok(());
        }
        let bytes_i32: Vec<i32> = bytes.iter().map(|&b| b as i32).collect();
        let body = serde_json::to_vec(&IngestRequest {
            instance,
            bytes: &bytes_i32,
        })?;
        let (status, _) = self.ep.request("POST", "/api/v1/tty/ingest", Some(&body))?;
        if status != 204 {
            anyhow::bail!("tty ingest: status {status}");
        }
        Ok(())
    }

    pub fn drain(&self, instance: u8, max: usize) -> anyhow::Result<Vec<u8>> {
        if max == 0 {
            return Ok(Vec::new());
        }
        let (status, body) = self.ep.request(
            "GET",
            &format!("/api/v1/kbd/drain?instance={instance}&max={max}"),
            None,
        )?;
        if !(200..300).contains(&status) {
            anyhow::bail!("kbd drain: status {status}");
        }
        let text =
            String::from_utf8(body).map_err(|_| anyhow::anyhow!("kbd drain: body not UTF-8"))?;
        parse_drain_body(&text, max)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn drain_body_parses_bytes() {
        assert_eq!(parse_drain_body("[108,115,10]", 64).unwrap(), b"ls\n");
        assert!(parse_drain_body("[]", 64).unwrap().is_empty());
    }

    #[test]
    fn drain_body_rejects_out_of_range_and_garbage() {
        assert!(parse_drain_body("[256]", 64).is_err());
        assert!(parse_drain_body("[-1]", 64).is_err());
        assert!(parse_drain_body("{\"a\":1}", 64).is_err());
        assert!(parse_drain_body("[1.5]", 64).is_err());
        assert!(parse_drain_body("", 64).is_err());
    }

    #[test]
    fn drain_body_rejects_more_than_asked() {
        assert!(parse_drain_body("[1,2,3]", 2).is_err());
    }

    #[test]
    fn empty_bytes_is_noop() {
        // No HTTP call should fire; if a bridge isn't running this would
        // print an error otherwise. Just verify it doesn't panic.
        post_ingest(0xDE, &[]);
    }

    #[test]
    fn endpoint_from_vars() {
        assert_eq!(
            Endpoint::from_vars(None, None, None),
            Endpoint::Tcp("127.0.0.1".into(), 26100)
        );
        assert_eq!(
            Endpoint::from_vars(
                Some("/run/x.sock".into()),
                Some("east".into()),
                Some("1".into())
            ),
            Endpoint::Unix("/run/x.sock".into())
        );
        assert_eq!(
            Endpoint::from_vars(Some(String::new()), Some("east".into()), Some("bad".into())),
            Endpoint::Tcp("east".into(), 26100)
        );
    }

    #[test]
    fn parse_response_status_body_and_length() {
        let (st, b) =
            parse_response(b"HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\n[1]extra").unwrap();
        assert_eq!((st, b.as_slice()), (200, b"[1]".as_slice()));
        let (st, b) = parse_response(b"HTTP/1.0 204 No Content\r\n\r\n").unwrap();
        assert_eq!((st, b.len()), (204, 0));
        assert!(parse_response(b"HTTP/1.1 200 OK\r\nContent-Length: 9\r\n\r\n[1]").is_err());
        assert!(
            parse_response(b"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3\r\n[1]")
                .is_err()
        );
        assert!(parse_response(b"garbage").is_err());
        assert!(parse_response(b"SPDY 200 OK\r\n\r\n").is_err());
    }

    /// A one-shot server: records the request, sends `reply`, closes.
    fn serve_once<L, S>(listener: L, reply: &'static [u8]) -> std::thread::JoinHandle<Vec<u8>>
    where
        L: FnOnce() -> S + Send + 'static,
        S: Read + Write,
    {
        std::thread::spawn(move || {
            let mut s = listener();
            let mut req = Vec::new();
            let mut buf = [0u8; 4096];
            // Read headers, then Content-Length more bytes.
            loop {
                let n = s.read(&mut buf).unwrap();
                req.extend_from_slice(&buf[..n]);
                if let Some(end) = req.windows(4).position(|w| w == b"\r\n\r\n") {
                    let head = String::from_utf8_lossy(&req[..end]).to_string();
                    let want = head
                        .lines()
                        .find_map(|l| l.strip_prefix("Content-Length: "))
                        .map(|v| v.trim().parse::<usize>().unwrap())
                        .unwrap_or(0);
                    while req.len() < end + 4 + want {
                        let n = s.read(&mut buf).unwrap();
                        req.extend_from_slice(&buf[..n]);
                    }
                    break;
                }
                if n == 0 {
                    break;
                }
            }
            s.write_all(reply).unwrap();
            req
        })
    }

    #[test]
    fn ingest_over_unix_socket() {
        let dir = std::env::temp_dir().join(format!("upc-bridge-test-{}", std::process::id()));
        let _ = std::fs::create_dir_all(&dir);
        let sock = dir.join("b.sock");
        let _ = std::fs::remove_file(&sock);
        let l = std::os::unix::net::UnixListener::bind(&sock).unwrap();
        let h = serve_once(
            move || l.accept().unwrap().0,
            b"HTTP/1.0 204 No Content\r\n\r\n",
        );
        let c = BridgeClient {
            ep: Endpoint::Unix(sock.to_string_lossy().into()),
        };
        c.ingest(0xDE, b"hi").unwrap();
        let req = String::from_utf8(h.join().unwrap()).unwrap();
        assert!(
            req.starts_with("POST /api/v1/tty/ingest HTTP/1.0\r\n"),
            "{req}"
        );
        assert!(
            req.ends_with(r#"{"instance":222,"bytes":[104,105]}"#),
            "{req}"
        );
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn drain_over_tcp() {
        let l = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let port = l.local_addr().unwrap().port();
        let h = serve_once(
            move || l.accept().unwrap().0,
            b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 11\r\n\r\n[108,115,10]",
        );
        let c = BridgeClient {
            ep: Endpoint::Tcp("127.0.0.1".into(), port),
        };
        // Content-Length 11 cuts "[108,115,10]" (12) to "[108,115,10" -> bad JSON.
        assert!(c.drain(1, 64).is_err());
        let req = String::from_utf8(h.join().unwrap()).unwrap();
        assert!(
            req.starts_with("GET /api/v1/kbd/drain?instance=1&max=64 HTTP/1.0\r\n"),
            "{req}"
        );

        let l = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let port = l.local_addr().unwrap().port();
        let h = serve_once(
            move || l.accept().unwrap().0,
            b"HTTP/1.1 200 OK\r\n\r\n[108,115,10]",
        );
        let c = BridgeClient {
            ep: Endpoint::Tcp("127.0.0.1".into(), port),
        };
        assert_eq!(c.drain(1, 64).unwrap(), b"ls\n");
        h.join().unwrap();
    }

    #[test]
    fn unreachable_is_an_error_not_a_hang() {
        let c = BridgeClient {
            ep: Endpoint::Unix("/nonexistent/upc-bridge.sock".into()),
        };
        assert!(c.ingest(1, b"x").is_err());
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
