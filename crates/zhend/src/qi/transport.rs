//! Transport layer for Qi gossip.
//!
//! Two modes:
//! 1. Piggybacked — fragments ride existing Monad HbH options (zero extra traffic)
//! 2. Standalone — UDP datagrams when Monad transport unavailable
//!
//! Piggybacked mode is preferred. Wu Wei — knowledge flows without forcing.

use crate::{ZhenError, ZhenResult};
use std::net::SocketAddr;
use tokio::net::UdpSocket;

/// Maximum UDP message size. Practical limit for gossip datagrams.
/// 64KB minus IP/UDP headers, rounded down for safety.
pub const MAX_MSG_SIZE: usize = 65_000;

/// UDP transport for standalone gossip when Monad transport is unavailable.
pub struct UdpTransport {
    socket: UdpSocket,
    local_addr: SocketAddr,
}

impl UdpTransport {
    /// Bind a UDP socket for gossip transport.
    ///
    /// `addr` should be a `host:port` string, e.g. `"[::]:7302"` or `"0.0.0.0:7302"`.
    pub async fn bind(addr: &str) -> ZhenResult<Self> {
        let socket = UdpSocket::bind(addr).await.map_err(|e| {
            ZhenError::Transport(format!("failed to bind UDP socket on {}: {}", addr, e))
        })?;
        let local_addr = socket
            .local_addr()
            .map_err(|e| ZhenError::Transport(format!("failed to get local addr: {}", e)))?;

        tracing::info!(%local_addr, "qi UDP transport bound");

        Ok(Self { socket, local_addr })
    }

    /// Send a datagram to the target address.
    ///
    /// Returns number of bytes sent. Enforces MAX_MSG_SIZE.
    pub async fn send_to(&self, data: &[u8], target: SocketAddr) -> ZhenResult<usize> {
        if data.len() > MAX_MSG_SIZE {
            return Err(ZhenError::Transport(format!(
                "message too large: {} bytes > {} max",
                data.len(),
                MAX_MSG_SIZE
            )));
        }

        self.socket
            .send_to(data, target)
            .await
            .map_err(|e| ZhenError::Transport(format!("send_to {} failed: {}", target, e)))
    }

    /// Receive a datagram. Blocks until data arrives.
    ///
    /// Returns (bytes_read, sender_addr). Buffer must be at least MAX_MSG_SIZE.
    pub async fn recv_from(&self, buf: &mut [u8]) -> ZhenResult<(usize, SocketAddr)> {
        self.socket
            .recv_from(buf)
            .await
            .map_err(|e| ZhenError::Transport(format!("recv_from failed: {}", e)))
    }

    /// Local address this transport is bound to.
    pub fn local_addr(&self) -> SocketAddr {
        self.local_addr
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_bind_and_local_addr() {
        let transport = UdpTransport::bind("127.0.0.1:0").await.unwrap();
        let addr = transport.local_addr();
        assert!(addr.port() > 0);
        assert_eq!(addr.ip(), std::net::Ipv4Addr::LOCALHOST);
    }

    #[tokio::test]
    async fn test_send_recv_loopback() {
        let t1 = UdpTransport::bind("127.0.0.1:0").await.unwrap();
        let t2 = UdpTransport::bind("127.0.0.1:0").await.unwrap();

        let msg = b"hello qi";
        t1.send_to(msg, t2.local_addr()).await.unwrap();

        let mut buf = vec![0u8; MAX_MSG_SIZE];
        let (n, from) = t2.recv_from(&mut buf).await.unwrap();
        assert_eq!(&buf[..n], msg);
        assert_eq!(from, t1.local_addr());
    }

    #[tokio::test]
    async fn test_oversized_message_rejected() {
        let t = UdpTransport::bind("127.0.0.1:0").await.unwrap();
        let big = vec![0u8; MAX_MSG_SIZE + 1];
        let target: SocketAddr = "127.0.0.1:9999".parse().unwrap();
        let result = t.send_to(&big, target).await;
        assert!(result.is_err());
    }
}
