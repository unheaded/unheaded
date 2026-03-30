// SPDX-License-Identifier: GPL-3.0-only
//! Screen bridge: reads SCREEN_MAP from BPF and streams pixel data to a browser.
//!
//! Integrated into doom-runner — no external Go bridge needed.
//!
//! # Protocol
//!
//! - `GET /` — HTML page with `<canvas>` and JavaScript WebSocket client.
//! - `WS /ws` — Binary frames: 64,000 bytes (320x200 palette indices) at ~60 Hz.
//!   Client sends `[scancode: u16 LE][pressed: u8]` for keyboard input.

use std::sync::{Arc, Mutex};
use std::time::Duration;

use anyhow::Result;
use aya::maps::Array;
use aya::Ebpf;
use axum::extract::ws::{Message, WebSocket};
use axum::extract::{State, WebSocketUpgrade};
use axum::response::{Html, IntoResponse};
use axum::routing::get;
use axum::Router;
use futures_util::{SinkExt, StreamExt};
use tokio::sync::broadcast;
use tracing::{error, info, warn};

use crate::memory;

/// Shared application state passed to axum handlers.
#[derive(Clone)]
struct AppState {
    /// Broadcast channel for screen frames (64K bytes each).
    frame_tx: broadcast::Sender<Arc<Vec<u8>>>,
    /// Channel for keyboard events (written to KBD_MAP).
    key_tx: tokio::sync::mpsc::Sender<KeyEvent>,
}

/// A keyboard event from the browser.
struct KeyEvent {
    scancode: u16,
    pressed: bool,
}

/// Bridge server configuration.
#[derive(Debug, Clone)]
pub struct BridgeConfig {
    /// Listen address (default: 0.0.0.0:16666).
    pub listen_addr: String,
    /// Target frame rate for screen polling (Hz).
    pub target_fps: u32,
}

impl Default for BridgeConfig {
    fn default() -> Self {
        Self {
            listen_addr: "0.0.0.0:16666".to_string(),
            target_fps: 60,
        }
    }
}

/// Start the integrated WebSocket bridge.
///
/// This takes ownership of the Ebpf via Arc<Mutex<>> so maps stay alive.
/// Serves HTML viewer at `/` and WebSocket at `/ws`.
/// Blocks until Ctrl-C.
pub async fn serve(ebpf: Arc<Mutex<Ebpf>>, config: &BridgeConfig) -> Result<()> {
    info!(
        "bridge: {}x{} ({} bytes/frame) at {} fps",
        memory::SCREEN_WIDTH,
        memory::SCREEN_HEIGHT,
        memory::SCREEN_SIZE,
        config.target_fps,
    );
    info!("bridge: listening on http://{}", config.listen_addr);

    // Broadcast channel for frames — capacity 2 (drop old frames if slow client)
    let (frame_tx, _) = broadcast::channel::<Arc<Vec<u8>>>(2);
    // MPSC channel for keyboard events
    let (key_tx, key_rx) = tokio::sync::mpsc::channel::<KeyEvent>(64);

    let state = AppState {
        frame_tx: frame_tx.clone(),
        key_tx,
    };

    // Spawn frame poller task
    let poller_ebpf = ebpf.clone();
    let poller_tx = frame_tx.clone();
    let frame_interval = Duration::from_millis(1000 / config.target_fps as u64);
    tokio::spawn(async move {
        frame_poller(poller_ebpf, poller_tx, frame_interval).await;
    });

    // Spawn keyboard writer task
    let kbd_ebpf = ebpf.clone();
    tokio::spawn(async move {
        kbd_writer(kbd_ebpf, key_rx).await;
    });

    // Build axum router
    let app = Router::new()
        .route("/", get(index_handler))
        .route("/ws", get(ws_handler))
        .with_state(state);

    let listener = tokio::net::TcpListener::bind(&config.listen_addr).await?;
    info!("bridge: server ready — open http://{} in your browser", config.listen_addr);

    axum::serve(listener, app)
        .with_graceful_shutdown(async {
            tokio::signal::ctrl_c().await.ok();
            info!("bridge: shutting down");
        })
        .await?;

    Ok(())
}

/// Serve the HTML viewer page.
async fn index_handler() -> impl IntoResponse {
    Html(VIEWER_HTML)
}

/// WebSocket upgrade handler.
async fn ws_handler(ws: WebSocketUpgrade, State(state): State<AppState>) -> impl IntoResponse {
    ws.on_upgrade(move |socket| handle_ws(socket, state))
}

/// Handle a single WebSocket connection.
async fn handle_ws(socket: WebSocket, state: AppState) {
    let (mut sender, mut receiver) = socket.split();

    // Subscribe to frame broadcasts
    let mut frame_rx = state.frame_tx.subscribe();
    let key_tx = state.key_tx.clone();

    info!("bridge: WebSocket client connected");

    // Spawn a task to forward frames to this client
    let send_task = tokio::spawn(async move {
        loop {
            match frame_rx.recv().await {
                Ok(frame) => {
                    if sender
                        .send(Message::Binary(frame.as_ref().clone().into()))
                        .await
                        .is_err()
                    {
                        break; // Client disconnected
                    }
                }
                Err(broadcast::error::RecvError::Lagged(n)) => {
                    warn!("bridge: client lagged {n} frames");
                }
                Err(broadcast::error::RecvError::Closed) => break,
            }
        }
    });

    // Receive key events from client
    let recv_task = tokio::spawn(async move {
        while let Some(Ok(msg)) = receiver.next().await {
            if let Message::Binary(data) = msg {
                if data.len() == 3 {
                    let scancode = u16::from_le_bytes([data[0], data[1]]);
                    let pressed = data[2] != 0;
                    let _ = key_tx.send(KeyEvent { scancode, pressed }).await;
                }
            }
        }
    });

    // Wait for either task to finish (client disconnect)
    tokio::select! {
        _ = send_task => {},
        _ = recv_task => {},
    }

    info!("bridge: WebSocket client disconnected");
}

/// Poll SCREEN_MAP at the target frame rate and broadcast frames.
async fn frame_poller(
    ebpf: Arc<Mutex<Ebpf>>,
    tx: broadcast::Sender<Arc<Vec<u8>>>,
    interval: Duration,
) {
    let mut ticker = tokio::time::interval(interval);
    ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);

    info!("bridge: frame poller started (interval {:?})", interval);

    loop {
        ticker.tick().await;

        // If no subscribers, skip the work
        if tx.receiver_count() == 0 {
            continue;
        }

        let frame = {
            let mut ebpf_guard = match ebpf.lock() {
                Ok(g) => g,
                Err(e) => {
                    error!("bridge: ebpf mutex poisoned: {e}");
                    return;
                }
            };
            read_screen(&mut ebpf_guard)
        };

        match frame {
            Some(pixels) => {
                let _ = tx.send(Arc::new(pixels));
            }
            None => {
                // Map not available yet — wait and retry
                tokio::time::sleep(Duration::from_secs(1)).await;
            }
        }
    }
}

/// Palette address in RAM_MAP (I_SetPalette writes 768 bytes here).
const PALETTE_ADDR: u32 = 0x0006_0000;
const PALETTE_SIZE: u32 = 768; // 256 * 3 (RGB)

/// Read 768-byte palette + 64,000 pixels from RAM_MAP.
/// Returns palette (768 bytes) followed by pixels (64000 bytes) = 64768 total.
/// The bridge JS splits the message: first 768 = palette, rest = pixels.
fn read_screen(ebpf: &mut Ebpf) -> Option<Vec<u8>> {
    let ram_map_ref = ebpf.map_mut("RAM_MAP");
    let ram: aya::maps::Array<_, u32> = match ram_map_ref {
        Some(map) => match aya::maps::Array::try_from(map) {
            Ok(a) => a,
            Err(e) => {
                error!("bridge: RAM_MAP not an Array<u32>: {e}");
                return None;
            }
        },
        None => {
            return None;
        }
    };

    let mut data = Vec::with_capacity((PALETTE_SIZE + memory::SCREEN_SIZE) as usize);

    // Read palette (768 bytes = 192 words)
    let pal_word_base = PALETTE_ADDR / 4;
    let pal_words = (PALETTE_SIZE + 3) / 4;
    for w in 0..pal_words {
        let word = ram.get(&(pal_word_base + w), 0).unwrap_or(0);
        data.push((word & 0xFF) as u8);
        data.push(((word >> 8) & 0xFF) as u8);
        data.push(((word >> 16) & 0xFF) as u8);
        data.push(((word >> 24) & 0xFF) as u8);
    }
    data.truncate(PALETTE_SIZE as usize);

    // Read pixels (64000 bytes = 16000 words)
    let screen_word_base = memory::SCREEN_BASE / 4;
    let num_words = (memory::SCREEN_SIZE + 3) / 4;
    for w in 0..num_words {
        let word = ram.get(&(screen_word_base + w), 0).unwrap_or(0);
        data.push((word & 0xFF) as u8);
        data.push(((word >> 8) & 0xFF) as u8);
        data.push(((word >> 16) & 0xFF) as u8);
        data.push(((word >> 24) & 0xFF) as u8);
    }
    data.truncate((PALETTE_SIZE + memory::SCREEN_SIZE) as usize);
    Some(data)
}

/// Write keyboard events to KBD_MAP.
async fn kbd_writer(ebpf: Arc<Mutex<Ebpf>>, mut rx: tokio::sync::mpsc::Receiver<KeyEvent>) {
    info!("bridge: keyboard writer started");

    while let Some(evt) = rx.recv().await {
        let mut ebpf_guard = match ebpf.lock() {
            Ok(g) => g,
            Err(e) => {
                error!("bridge: ebpf mutex poisoned: {e}");
                return;
            }
        };

        // KBD_MAP[0] = scancode << 1 | pressed
        let kbd_map_ref = ebpf_guard.map_mut("KBD_MAP");
        if let Some(map) = kbd_map_ref {
            if let Ok(mut kbd) = Array::<_, u32>::try_from(map) {
                let val = (evt.scancode as u32) << 1 | (evt.pressed as u32);
                if let Err(e) = kbd.set(0, val, 0) {
                    warn!("bridge: KBD_MAP write failed: {e}");
                }
            }
        }
    }
}

/// The standard Doom PLAYPAL palette (entry 0) — 256 RGB triplets (768 values).
/// Extracted from the DOOM source code / WAD PLAYPAL lump.
/// Available for server-side palette conversion if needed in the future.
#[allow(dead_code)]
const DOOM_PALETTE: [u8; 768] = [
    0,0,0, 31,23,11, 23,15,7, 75,75,75, 255,255,255, 27,27,27, 19,19,19, 11,11,11,
    7,7,7, 47,55,31, 35,43,15, 23,31,7, 15,23,0, 79,59,43, 71,51,35, 63,43,27,
    255,183,183, 247,171,171, 243,163,163, 235,151,151, 231,143,143, 223,135,135, 219,123,123, 211,115,115,
    203,107,107, 199,99,99, 191,91,91, 187,87,87, 179,79,79, 175,71,71, 167,63,63, 163,59,59,
    155,51,51, 151,47,47, 143,43,43, 139,35,35, 131,31,31, 127,27,27, 119,23,23, 115,19,19,
    107,15,15, 103,11,11, 95,7,7, 91,7,7, 83,7,7, 79,0,0, 71,0,0, 67,0,0,
    255,235,223, 255,227,211, 255,219,199, 255,211,187, 255,199,175, 255,191,163, 255,183,155, 255,175,143,
    255,167,131, 255,159,115, 255,147,103, 255,139,91, 255,131,83, 255,123,75, 255,111,63, 255,103,55,
    255,95,47, 255,87,35, 255,79,27, 247,73,23, 239,67,19, 231,63,15, 223,55,11, 215,51,7,
    207,47,7, 199,43,7, 191,39,7, 183,35,7, 175,31,7, 167,27,7, 159,23,7, 151,19,7,
    143,15,7, 135,11,7, 127,7,7, 119,7,7, 111,7,7, 103,7,7, 95,7,7, 87,7,7,
    79,7,7, 71,7,7, 63,7,7, 55,7,7, 47,7,7, 39,7,7, 35,7,7, 27,7,7,
    187,115,159, 175,107,143, 163,95,131, 151,87,119, 139,79,107, 127,75,95, 115,67,83, 107,59,75,
    95,51,63, 83,43,55, 71,35,43, 59,31,35, 47,23,27, 35,19,19, 23,11,11, 15,7,7,
    187,115,159, 219,195,187, 215,187,175, 207,179,167, 203,167,155, 195,159,147, 191,151,135, 183,143,127,
    175,135,115, 171,127,107, 163,119,99, 155,111,91, 151,103,83, 143,95,79, 135,91,71, 131,83,63,
    123,79,59, 115,71,51, 111,67,47, 103,59,43, 99,55,39, 91,51,35, 87,43,31, 83,39,27,
    75,35,23, 71,31,19, 63,27,15, 59,23,15, 51,19,11, 47,15,7, 43,15,7, 35,11,7,
    27,7,0, 23,7,0, 19,7,0, 15,7,0, 79,47,43, 59,35,31, 43,23,15, 255,183,0,
    247,179,0, 239,175,0, 231,167,0, 223,163,0, 215,155,0, 207,151,0, 199,143,0, 191,139,0,
    183,131,0, 175,127,0, 167,123,0, 159,115,0, 151,111,0, 143,107,0, 135,99,0, 127,95,0,
    119,91,0, 111,83,0, 103,79,0, 95,71,0, 87,67,0, 79,59,0, 71,55,0, 63,47,0,
    55,43,0, 47,39,0, 39,31,0, 31,27,0, 23,19,0, 15,15,0, 7,7,0, 0,0,0,
    15,15,15, 23,23,23, 31,31,31, 47,47,47, 59,59,59, 71,71,71, 83,83,83, 99,99,99,
    111,111,111, 127,127,127, 139,139,139, 155,155,155, 167,167,167, 183,183,183, 195,195,195, 211,211,211,
    227,227,227, 239,239,239, 251,251,251, 167,59,43, 159,47,35, 151,43,27, 147,35,23, 139,27,19,
    131,23,15, 127,15,7, 119,11,7, 111,7,0, 107,7,0, 99,7,0, 91,7,0, 87,7,0,
    79,7,0, 75,7,0, 67,7,0, 63,7,0, 59,7,0, 55,7,0, 47,7,0, 43,7,0,
    39,7,0, 35,7,0, 27,7,0, 23,7,0, 19,7,0, 15,7,0, 11,7,0, 7,7,0,
    0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0,
    0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0,
    0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0,
];

/// Minimal HTML page for the Doom canvas viewer with embedded palette.
pub const VIEWER_HTML: &str = r##"<!DOCTYPE html>
<html>
<head>
  <title>Doom over IPv6 - Unheaded</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      background: #0a0a0a;
      display: flex;
      flex-direction: column;
      justify-content: center;
      align-items: center;
      height: 100vh;
      font-family: 'JetBrains Mono', 'Fira Code', monospace;
      color: #c0c0c0;
    }
    h1 {
      font-size: 14px;
      margin-bottom: 8px;
      color: #ff5c00;
      letter-spacing: 2px;
      text-transform: uppercase;
    }
    canvas {
      image-rendering: pixelated;
      image-rendering: crisp-edges;
      width: 960px;
      height: 600px;
      border: 1px solid #333;
    }
    #status {
      margin-top: 8px;
      font-size: 11px;
      color: #666;
    }
    #fps {
      color: #0f0;
    }
  </style>
</head>
<body>
  <h1>Doom over IPv6</h1>
  <canvas id="doom" width="320" height="200"></canvas>
  <div id="status">
    <span id="conn">connecting...</span> |
    frames: <span id="fcount">0</span> |
    fps: <span id="fps">0</span>
  </div>
  <script>
    const canvas = document.getElementById('doom');
    const ctx = canvas.getContext('2d');
    const img = ctx.createImageData(320, 200);
    const connEl = document.getElementById('conn');
    const fcountEl = document.getElementById('fcount');
    const fpsEl = document.getElementById('fps');

    // Doom PLAYPAL palette — 256 RGB entries (768 values)
    const PALETTE = [
      0,0,0, 31,23,11, 23,15,7, 75,75,75, 255,255,255, 27,27,27, 19,19,19, 11,11,11,
      7,7,7, 47,55,31, 35,43,15, 23,31,7, 15,23,0, 79,59,43, 71,51,35, 63,43,27,
      255,183,183, 247,171,171, 243,163,163, 235,151,151, 231,143,143, 223,135,135, 219,123,123, 211,115,115,
      203,107,107, 199,99,99, 191,91,91, 187,87,87, 179,79,79, 175,71,71, 167,63,63, 163,59,59,
      155,51,51, 151,47,47, 143,43,43, 139,35,35, 131,31,31, 127,27,27, 119,23,23, 115,19,19,
      107,15,15, 103,11,11, 95,7,7, 91,7,7, 83,7,7, 79,0,0, 71,0,0, 67,0,0,
      255,235,223, 255,227,211, 255,219,199, 255,211,187, 255,199,175, 255,191,163, 255,183,155, 255,175,143,
      255,167,131, 255,159,115, 255,147,103, 255,139,91, 255,131,83, 255,123,75, 255,111,63, 255,103,55,
      255,95,47, 255,87,35, 255,79,27, 247,73,23, 239,67,19, 231,63,15, 223,55,11, 215,51,7,
      207,47,7, 199,43,7, 191,39,7, 183,35,7, 175,31,7, 167,27,7, 159,23,7, 151,19,7,
      143,15,7, 135,11,7, 127,7,7, 119,7,7, 111,7,7, 103,7,7, 95,7,7, 87,7,7,
      79,7,7, 71,7,7, 63,7,7, 55,7,7, 47,7,7, 39,7,7, 35,7,7, 27,7,7,
      187,115,159, 175,107,143, 163,95,131, 151,87,119, 139,79,107, 127,75,95, 115,67,83, 107,59,75,
      95,51,63, 83,43,55, 71,35,43, 59,31,35, 47,23,27, 35,19,19, 23,11,11, 15,7,7,
      187,115,159, 219,195,187, 215,187,175, 207,179,167, 203,167,155, 195,159,147, 191,151,135, 183,143,127,
      175,135,115, 171,127,107, 163,119,99, 155,111,91, 151,103,83, 143,95,79, 135,91,71, 131,83,63,
      123,79,59, 115,71,51, 111,67,47, 103,59,43, 99,55,39, 91,51,35, 87,43,31, 83,39,27,
      75,35,23, 71,31,19, 63,27,15, 59,23,15, 51,19,11, 47,15,7, 43,15,7, 35,11,7,
      27,7,0, 23,7,0, 19,7,0, 15,7,0, 79,47,43, 59,35,31, 43,23,15, 255,183,0,
      247,179,0, 239,175,0, 231,167,0, 223,163,0, 215,155,0, 207,151,0, 199,143,0, 191,139,0,
      183,131,0, 175,127,0, 167,123,0, 159,115,0, 151,111,0, 143,107,0, 135,99,0, 127,95,0,
      119,91,0, 111,83,0, 103,79,0, 95,71,0, 87,67,0, 79,59,0, 71,55,0, 63,47,0,
      55,43,0, 47,39,0, 39,31,0, 31,27,0, 23,19,0, 15,15,0, 7,7,0, 0,0,0,
      15,15,15, 23,23,23, 31,31,31, 47,47,47, 59,59,59, 71,71,71, 83,83,83, 99,99,99,
      111,111,111, 127,127,127, 139,139,139, 155,155,155, 167,167,167, 183,183,183, 195,195,195, 211,211,211,
      227,227,227, 239,239,239, 251,251,251, 167,59,43, 159,47,35, 151,43,27, 147,35,23, 139,27,19,
      131,23,15, 127,15,7, 119,11,7, 111,7,0, 107,7,0, 99,7,0, 91,7,0, 87,7,0,
      79,7,0, 75,7,0, 67,7,0, 63,7,0, 59,7,0, 55,7,0, 47,7,0, 43,7,0,
      39,7,0, 35,7,0, 27,7,0, 23,7,0, 19,7,0, 15,7,0, 11,7,0, 7,7,0,
      0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0,
      0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0,
      0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0, 0,0,0
    ];

    let frameCount = 0;
    let lastFpsTime = performance.now();
    let lastFpsCount = 0;

    function connect() {
      const ws = new WebSocket('ws://' + location.host + '/ws');
      ws.binaryType = 'arraybuffer';

      ws.onopen = () => {
        connEl.textContent = 'connected';
        connEl.style.color = '#0f0';
      };

      ws.onclose = () => {
        connEl.textContent = 'disconnected';
        connEl.style.color = '#f00';
        // Reconnect after 1 second
        setTimeout(connect, 1000);
      };

      ws.onerror = () => {
        ws.close();
      };

      ws.onmessage = (e) => {
        const data = new Uint8Array(e.data);
        // Message format: [768 bytes palette][64000 bytes pixels]
        if (data.length >= 64768) {
          // Dynamic palette from WAD PLAYPAL (first 768 bytes)
          const pal = data.subarray(0, 768);
          const pixels = data.subarray(768);
          for (let i = 0; i < 64000; i++) {
            const ci = pixels[i] * 3;
            const off = i * 4;
            img.data[off]     = pal[ci];
            img.data[off + 1] = pal[ci + 1];
            img.data[off + 2] = pal[ci + 2];
            img.data[off + 3] = 255;
          }
        } else if (data.length === 64000) {
          // Legacy: no palette prefix, use hardcoded
          for (let i = 0; i < 64000; i++) {
            const ci = data[i] * 3;
            const off = i * 4;
            img.data[off]     = PALETTE[ci];
            img.data[off + 1] = PALETTE[ci + 1];
            img.data[off + 2] = PALETTE[ci + 2];
            img.data[off + 3] = 255;
          }
        } else return;
        ctx.putImageData(img, 0, 0);

        frameCount++;
        fcountEl.textContent = frameCount;

        const now = performance.now();
        if (now - lastFpsTime >= 1000) {
          const fps = ((frameCount - lastFpsCount) * 1000 / (now - lastFpsTime)).toFixed(1);
          fpsEl.textContent = fps;
          lastFpsTime = now;
          lastFpsCount = frameCount;
        }
      };

      // Keyboard input
      function sendKey(code, pressed) {
        if (ws.readyState !== WebSocket.OPEN) return;
        const buf = new ArrayBuffer(3);
        const view = new DataView(buf);
        view.setUint16(0, code, true); // little-endian scancode
        view.setUint8(2, pressed ? 1 : 0);
        ws.send(buf);
      }

      document.addEventListener('keydown', (e) => {
        e.preventDefault();
        sendKey(e.keyCode, true);
      });

      document.addEventListener('keyup', (e) => {
        e.preventDefault();
        sendKey(e.keyCode, false);
      });
    }

    connect();
  </script>
</body>
</html>
"##;

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

    #[test]
    fn viewer_html_has_palette() {
        assert!(VIEWER_HTML.contains("PALETTE"));
        assert!(VIEWER_HTML.contains("ws://"));
    }

    #[test]
    fn doom_palette_size() {
        assert_eq!(DOOM_PALETTE.len(), 768);
        // Color 4 is white (255,255,255)
        assert_eq!(DOOM_PALETTE[4 * 3], 255);
        assert_eq!(DOOM_PALETTE[4 * 3 + 1], 255);
        assert_eq!(DOOM_PALETTE[4 * 3 + 2], 255);
    }
}
