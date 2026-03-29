//! QUIC/HTTP3 transport for external Zhen access.
//!
//! Uses quinn for QUIC. Benefits over TCP:
//! - 0-RTT connection establishment (returning peers reconnect instantly)
//! - Connection migration (mobile/roaming nodes keep sessions alive)
//! - Multiplexed streams (multiple pilgrimages in parallel, no head-of-line blocking)
//! - Built-in encryption (TLS 1.3 mandatory)
//!
//! This is the transport for nodes outside the Kingdom's gRPC mesh —
//! edge nodes, mobile agents, external knowledge contributors.

// TODO: quinn server setup with self-signed or ACME certs
// TODO: HTTP/3 request handler mapping to same operations as gRPC
// TODO: 0-RTT session resumption for returning peers
// TODO: connection migration handling for gossip peers
