// SPDX-License-Identifier: GPL-3.0-only
//! Screen bridge: reads SCREEN_MAP from BPF and streams pixel data to a browser.
//!
//! Replaces the Go `doom-bridge` binary with a Rust implementation integrated
//! into the doom-runner process.
//!
//! # Protocol
//!
//! The bridge serves two endpoints:
//!
//! - `GET /` — returns a minimal HTML page with a `<canvas>` element and
//!   JavaScript that connects to the WebSocket endpoint.
//!
//! - `WS /ws` — WebSocket endpoint that streams framebuffer data at ~60 Hz.
//!   Each message is a binary frame containing 64,000 bytes (320x200 pixels,
//!   8-bit palette indices). The browser JavaScript applies the Doom palette
//!   and renders to canvas.
//!
//! # Keyboard Input
//!
//! The WebSocket also accepts incoming binary messages with key events:
//! `[scancode: u16 LE][pressed: u8]` — written to KBD_MAP.
//!
//! # Implementation Status
//!
//! This is a scaffold. The actual WebSocket server and BPF map reading will be
//! implemented once the core loader/tail_calls pipeline is working.

use anyhow::Result;
use tracing::info;

use crate::memory;

/// Bridge server configuration.
#[derive(Debug, Clone)]
pub struct BridgeConfig {
    /// Listen address (default: 0.0.0.0:16666).
    pub listen_addr: String,
    /// BPF map pin path for SCREEN_MAP.
    pub screen_map_pin: String,
    /// BPF map pin path for KBD_MAP.
    pub kbd_map_pin: String,
    /// BPF map pin path for STATS (for frame_ready counter).
    pub stats_map_pin: String,
    /// Target frame rate for screen polling (Hz).
    pub target_fps: u32,
}

impl Default for BridgeConfig {
    fn default() -> Self {
        Self {
            listen_addr: "0.0.0.0:16666".to_string(),
            screen_map_pin: "/sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP".to_string(),
            kbd_map_pin: "/sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP".to_string(),
            stats_map_pin: "/sys/fs/bpf/unheaded/doom-ring/maps/STATS".to_string(),
            target_fps: 60,
        }
    }
}

/// The Doom color palette (256 RGB entries).
/// Each entry is [R, G, B]. This is the standard Doom PLAYPAL lump, entry 0.
/// Stored here so the bridge can do palette conversion server-side if needed,
/// but typically the browser applies it client-side for lower bandwidth.
pub const DOOM_PALETTE_SIZE: usize = 256;

/// Start the screen bridge server.
///
/// This is a placeholder — the actual implementation will:
/// 1. Open pinned SCREEN_MAP via Aya
/// 2. Spawn a tokio task that polls SCREEN_MAP at target_fps Hz
/// 3. Serve HTTP + WebSocket on listen_addr
/// 4. Stream framebuffer data to connected WebSocket clients
/// 5. Accept keyboard events from WebSocket and write to KBD_MAP
pub async fn serve(config: &BridgeConfig) -> Result<()> {
    info!(
        "screen bridge: {}x{} ({} bytes/frame) at {} fps",
        memory::SCREEN_WIDTH,
        memory::SCREEN_HEIGHT,
        memory::SCREEN_SIZE,
        config.target_fps,
    );
    info!("listen: {}", config.listen_addr);
    info!("screen_map: {}", config.screen_map_pin);
    info!("kbd_map: {}", config.kbd_map_pin);

    // TODO: Implement WebSocket server.
    //
    // Dependencies needed (add to Cargo.toml when implementing):
    //   tokio-tungstenite = "0.21"
    //   axum = { version = "0.7", features = ["ws"] }
    //
    // Rough architecture:
    //
    // 1. Open SCREEN_MAP as Array<u8> via aya::maps::Array::from_pin()
    // 2. Open KBD_MAP as Array<u32> via aya::maps::Array::from_pin()
    // 3. Open STATS as HashMap<u32, u64> for STAT_FRAME_READY polling
    // 4. Spawn frame poller task:
    //    loop {
    //        let frame_ready = stats.get(&11)?;
    //        if frame_ready != last_frame {
    //            let pixels: Vec<u8> = (0..64000).map(|i| screen_map.get(&i, 0)).collect();
    //            broadcast_to_clients(pixels);
    //            last_frame = frame_ready;
    //        }
    //        tokio::time::sleep(Duration::from_millis(1000 / target_fps)).await;
    //    }
    // 5. Serve / with HTML canvas page
    // 6. Serve /ws with WebSocket upgrade

    info!("bridge not yet implemented — scaffold only");
    Ok(())
}

/// Minimal HTML page for the Doom canvas viewer.
/// Served at `GET /`.
pub const VIEWER_HTML: &str = r#"<!DOCTYPE html>
<html>
<head>
  <title>Doom over IPv6</title>
  <style>
    body { background: #000; margin: 0; display: flex; justify-content: center; align-items: center; height: 100vh; }
    canvas { image-rendering: pixelated; width: 960px; height: 600px; }
  </style>
</head>
<body>
  <canvas id="doom" width="320" height="200"></canvas>
  <script>
    const canvas = document.getElementById('doom');
    const ctx = canvas.getContext('2d');
    const img = ctx.createImageData(320, 200);
    // Doom palette will be loaded from /palette.json or embedded
    const ws = new WebSocket(`ws://${location.host}/ws`);
    ws.binaryType = 'arraybuffer';
    ws.onmessage = (e) => {
      const pixels = new Uint8Array(e.data);
      // Apply palette and render (palette lookup in real implementation)
      for (let i = 0; i < 64000; i++) {
        const p = pixels[i];
        const off = i * 4;
        img.data[off] = p;     // R (placeholder — need palette)
        img.data[off+1] = p;   // G
        img.data[off+2] = p;   // B
        img.data[off+3] = 255; // A
      }
      ctx.putImageData(img, 0, 0);
    };
    document.addEventListener('keydown', (e) => {
      const buf = new ArrayBuffer(3);
      const view = new DataView(buf);
      view.setUint16(0, e.keyCode, true);
      view.setUint8(2, 1);
      ws.send(buf);
    });
    document.addEventListener('keyup', (e) => {
      const buf = new ArrayBuffer(3);
      const view = new DataView(buf);
      view.setUint16(0, e.keyCode, true);
      view.setUint8(2, 0);
      ws.send(buf);
    });
  </script>
</body>
</html>
"#;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_config_port() {
        let cfg = BridgeConfig::default();
        assert!(cfg.listen_addr.contains("16666"));
    }

    #[test]
    fn viewer_html_has_canvas() {
        assert!(VIEWER_HTML.contains("<canvas"));
        assert!(VIEWER_HTML.contains("320"));
        assert!(VIEWER_HTML.contains("200"));
    }
}
