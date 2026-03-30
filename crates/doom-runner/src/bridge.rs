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

/// Poll RAM_MAP at the target frame rate and broadcast frames.
///
/// Simple timer-based polling (no frame-sync). Reads every tick regardless of
/// whether Doom produced a new frame. This ensures wipe animations, menus, and
/// all visual transitions render correctly without partial-frame artifacts.
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
                tokio::time::sleep(Duration::from_secs(1)).await;
            }
        }
    }
}

/// Palette address in RAM_MAP (I_SetPalette writes 768 bytes here).
const PALETTE_ADDR: u32 = 0x0006_0000;
const PALETTE_SIZE: u32 = 768; // 256 * 3 (RGB)

/// Read palette + pixels from RAM_MAP (16K word reads, fast).
///
/// Reads from RAM_MAP (not SCREEN_MAP) because Doom's HUD, memcpy, and memset
/// use word stores (SW) which only go to RAM_MAP (Bug 24 fix blocks SCREEN_MAP
/// word stores to prevent garbage from corrupted pointers). Reading RAM_MAP sees
/// ALL writes — byte stores AND word stores — so the status bar renders correctly.
///
/// 16,000 u32 reads is 4x fewer BPF syscalls than 64,000 u8 reads from SCREEN_MAP,
/// keeping the ebpf lock held briefly so keyboard writes aren't blocked.
fn read_screen(ebpf: &mut Ebpf) -> Option<Vec<u8>> {
    let ram: Array<_, u32> = match ebpf.map_mut("RAM_MAP") {
        Some(map) => match Array::try_from(map) {
            Ok(a) => a,
            Err(e) => {
                error!("bridge: RAM_MAP: {e}");
                return None;
            }
        },
        None => return None,
    };

    // Screen is now 32-bit XRGB (doomgeneric-style palette conversion in C).
    // Each pixel is one u32 word: 0x00RRGGBB. Read 64000 words = 256KB.
    // Send as 192000 bytes (64000 × RGB, no palette prefix needed).
    let screen_word_base = memory::SCREEN_BASE / 4;
    let num_pixels = memory::SCREEN_SIZE;  // 64000 pixels, each is one word
    let mut data = Vec::with_capacity((num_pixels * 3) as usize);

    for px in 0..num_pixels {
        let xrgb = ram.get(&(screen_word_base + px), 0).unwrap_or(0);
        // XRGB8888: R=bits 16-23, G=bits 8-15, B=bits 0-7
        data.push(((xrgb >> 16) & 0xFF) as u8); // R
        data.push(((xrgb >> 8) & 0xFF) as u8);  // G
        data.push((xrgb & 0xFF) as u8);          // B
    }
    Some(data)
}

/// Write keyboard events to KBD_MAP using circular queue (8 slots).
///
/// The executor's SYS_GET_KEY scans all 8 slots and clears consumed events.
/// Using a circular write head prevents rapid events (especially keyup after
/// held keydown) from overwriting each other, which caused keys to "stick."
async fn kbd_writer(ebpf: Arc<Mutex<Ebpf>>, mut rx: tokio::sync::mpsc::Receiver<KeyEvent>) {
    info!("bridge: keyboard writer started (8-slot circular queue)");

    let mut write_head: u32 = 0;

    while let Some(evt) = rx.recv().await {
        let mut ebpf_guard = match ebpf.lock() {
            Ok(g) => g,
            Err(e) => {
                error!("bridge: ebpf mutex poisoned: {e}");
                return;
            }
        };

        let kbd_map_ref = ebpf_guard.map_mut("KBD_MAP");
        if let Some(map) = kbd_map_ref {
            if let Ok(mut kbd) = Array::<_, u32>::try_from(map) {
                let val = (evt.scancode as u32) << 1 | (evt.pressed as u32);
                // Find an empty slot starting from write_head (circular scan).
                // If all slots are full, overwrite write_head (oldest unread event).
                let mut wrote = false;
                for i in 0..8u32 {
                    let slot = (write_head + i) % 8;
                    let current = kbd.get(&slot, 0).unwrap_or(0);
                    if current == 0 {
                        // Empty slot — write here
                        if kbd.set(slot, val, 0).is_ok() {
                            write_head = (slot + 1) % 8;
                            wrote = true;
                            break;
                        }
                    }
                }
                if !wrote {
                    // All slots full — overwrite at write_head (drop oldest)
                    let _ = kbd.set(write_head, val, 0);
                    write_head = (write_head + 1) % 8;
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
    // Retail DOOM.WAD PLAYPAL palette 0 — 256 RGB triplets (768 bytes).
    // Generated from the actual WAD file to match the dynamic palette path.
    0,0,0, 31,23,11, 23,15,7, 75,75,75, 255,255,255, 27,27,27, 19,19,19, 11,11,11,
    7,7,7, 47,55,31, 35,43,15, 23,31,7, 15,23,0, 79,59,43, 71,51,35, 63,43,27,
    255,183,183, 247,171,171, 243,163,163, 235,151,151, 231,143,143, 223,135,135, 219,123,123, 211,115,115,
    203,107,107, 199,99,99, 191,91,91, 187,87,87, 179,79,79, 175,71,71, 167,63,63, 163,59,59,
    155,51,51, 151,47,47, 143,43,43, 139,35,35, 131,31,31, 127,27,27, 119,23,23, 115,19,19,
    107,15,15, 103,11,11, 95,7,7, 91,7,7, 83,7,7, 79,0,0, 71,0,0, 67,0,0,
    255,235,223, 255,227,211, 255,219,199, 255,211,187, 255,207,179, 255,199,167, 255,191,155, 255,187,147,
    255,179,131, 247,171,123, 239,163,115, 231,155,107, 223,147,99, 215,139,91, 207,131,83, 203,127,79,
    191,123,75, 179,115,71, 171,111,67, 163,107,63, 155,99,59, 143,95,55, 135,87,51, 127,83,47,
    119,79,43, 107,71,39, 95,67,35, 83,63,31, 75,55,27, 63,47,23, 51,43,19, 43,35,15,
    239,239,239, 231,231,231, 223,223,223, 219,219,219, 211,211,211, 203,203,203, 199,199,199, 191,191,191,
    183,183,183, 179,179,179, 171,171,171, 167,167,167, 159,159,159, 151,151,151, 147,147,147, 139,139,139,
    131,131,131, 127,127,127, 119,119,119, 111,111,111, 107,107,107, 99,99,99, 91,91,91, 87,87,87,
    79,79,79, 71,71,71, 67,67,67, 59,59,59, 55,55,55, 47,47,47, 39,39,39, 35,35,35,
    119,255,111, 111,239,103, 103,223,95, 95,207,87, 91,191,79, 83,175,71, 75,159,63, 67,147,55,
    63,131,47, 55,115,43, 47,99,35, 39,83,27, 31,67,23, 23,51,15, 19,35,11, 11,23,7,
    191,167,143, 183,159,135, 175,151,127, 167,143,119, 159,135,111, 155,127,107, 147,123,99, 139,115,91,
    131,107,87, 123,99,79, 119,95,75, 111,87,67, 103,83,63, 95,75,55, 87,67,51, 83,63,47,
    159,131,99, 143,119,83, 131,107,75, 119,95,63, 103,83,51, 91,71,43, 79,59,35, 67,51,27,
    123,127,99, 111,115,87, 103,107,79, 91,99,71, 83,87,59, 71,79,51, 63,71,43, 55,63,39,
    255,255,115, 235,219,87, 215,187,67, 195,155,47, 175,123,31, 155,91,19, 135,67,7, 115,43,0,
    255,255,255, 255,219,219, 255,187,187, 255,155,155, 255,123,123, 255,95,95, 255,63,63, 255,31,31,
    255,0,0, 239,0,0, 227,0,0, 215,0,0, 203,0,0, 191,0,0, 179,0,0, 167,0,0,
    155,0,0, 139,0,0, 127,0,0, 115,0,0, 103,0,0, 91,0,0, 79,0,0, 67,0,0,
    231,231,255, 199,199,255, 171,171,255, 143,143,255, 115,115,255, 83,83,255, 55,55,255, 27,27,255,
    0,0,255, 0,0,227, 0,0,203, 0,0,179, 0,0,155, 0,0,131, 0,0,107, 0,0,83,
    255,255,255, 255,235,219, 255,215,187, 255,199,155, 255,179,123, 255,163,91, 255,143,59, 255,127,27,
    243,115,23, 235,111,15, 223,103,15, 215,95,11, 203,87,7, 195,79,0, 183,71,0, 175,67,0,
    255,255,255, 255,255,215, 255,255,179, 255,255,143, 255,255,107, 255,255,71, 255,255,35, 255,255,0,
    167,63,0, 159,55,0, 147,47,0, 135,35,0, 79,59,39, 67,47,27, 55,35,19, 47,27,11,
    0,0,83, 0,0,71, 0,0,59, 0,0,47, 0,0,35, 0,0,23, 0,0,11, 0,0,0,
    255,159,67, 255,231,75, 255,123,255, 255,0,255, 207,0,207, 159,0,155, 111,0,107, 167,107,107,
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
      /* Bilinear upscale: smooths nearest-neighbor texture banding.
         Remove these comments and add pixelated/crisp-edges to restore sharp pixels. */
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

    // Doom PLAYPAL palette 0 — 256 RGB entries (768 values)
    // From retail DOOM.WAD — must match the dynamic palette for consistent colors.
    const PALETTE = [
      0,0,0, 31,23,11, 23,15,7, 75,75,75, 255,255,255, 27,27,27, 19,19,19, 11,11,11,
      7,7,7, 47,55,31, 35,43,15, 23,31,7, 15,23,0, 79,59,43, 71,51,35, 63,43,27,
      255,183,183, 247,171,171, 243,163,163, 235,151,151, 231,143,143, 223,135,135, 219,123,123, 211,115,115,
      203,107,107, 199,99,99, 191,91,91, 187,87,87, 179,79,79, 175,71,71, 167,63,63, 163,59,59,
      155,51,51, 151,47,47, 143,43,43, 139,35,35, 131,31,31, 127,27,27, 119,23,23, 115,19,19,
      107,15,15, 103,11,11, 95,7,7, 91,7,7, 83,7,7, 79,0,0, 71,0,0, 67,0,0,
      255,235,223, 255,227,211, 255,219,199, 255,211,187, 255,207,179, 255,199,167, 255,191,155, 255,187,147,
      255,179,131, 247,171,123, 239,163,115, 231,155,107, 223,147,99, 215,139,91, 207,131,83, 203,127,79,
      191,123,75, 179,115,71, 171,111,67, 163,107,63, 155,99,59, 143,95,55, 135,87,51, 127,83,47,
      119,79,43, 107,71,39, 95,67,35, 83,63,31, 75,55,27, 63,47,23, 51,43,19, 43,35,15,
      239,239,239, 231,231,231, 223,223,223, 219,219,219, 211,211,211, 203,203,203, 199,199,199, 191,191,191,
      183,183,183, 179,179,179, 171,171,171, 167,167,167, 159,159,159, 151,151,151, 147,147,147, 139,139,139,
      131,131,131, 127,127,127, 119,119,119, 111,111,111, 107,107,107, 99,99,99, 91,91,91, 87,87,87,
      79,79,79, 71,71,71, 67,67,67, 59,59,59, 55,55,55, 47,47,47, 39,39,39, 35,35,35,
      119,255,111, 111,239,103, 103,223,95, 95,207,87, 91,191,79, 83,175,71, 75,159,63, 67,147,55,
      63,131,47, 55,115,43, 47,99,35, 39,83,27, 31,67,23, 23,51,15, 19,35,11, 11,23,7,
      191,167,143, 183,159,135, 175,151,127, 167,143,119, 159,135,111, 155,127,107, 147,123,99, 139,115,91,
      131,107,87, 123,99,79, 119,95,75, 111,87,67, 103,83,63, 95,75,55, 87,67,51, 83,63,47,
      159,131,99, 143,119,83, 131,107,75, 119,95,63, 103,83,51, 91,71,43, 79,59,35, 67,51,27,
      123,127,99, 111,115,87, 103,107,79, 91,99,71, 83,87,59, 71,79,51, 63,71,43, 55,63,39,
      255,255,115, 235,219,87, 215,187,67, 195,155,47, 175,123,31, 155,91,19, 135,67,7, 115,43,0,
      255,255,255, 255,219,219, 255,187,187, 255,155,155, 255,123,123, 255,95,95, 255,63,63, 255,31,31,
      255,0,0, 239,0,0, 227,0,0, 215,0,0, 203,0,0, 191,0,0, 179,0,0, 167,0,0,
      155,0,0, 139,0,0, 127,0,0, 115,0,0, 103,0,0, 91,0,0, 79,0,0, 67,0,0,
      231,231,255, 199,199,255, 171,171,255, 143,143,255, 115,115,255, 83,83,255, 55,55,255, 27,27,255,
      0,0,255, 0,0,227, 0,0,203, 0,0,179, 0,0,155, 0,0,131, 0,0,107, 0,0,83,
      255,255,255, 255,235,219, 255,215,187, 255,199,155, 255,179,123, 255,163,91, 255,143,59, 255,127,27,
      243,115,23, 235,111,15, 223,103,15, 215,95,11, 203,87,7, 195,79,0, 183,71,0, 175,67,0,
      255,255,255, 255,255,215, 255,255,179, 255,255,143, 255,255,107, 255,255,71, 255,255,35, 255,255,0,
      167,63,0, 159,55,0, 147,47,0, 135,35,0, 79,59,39, 67,47,27, 55,35,19, 47,27,11,
      0,0,83, 0,0,71, 0,0,59, 0,0,47, 0,0,35, 0,0,23, 0,0,11, 0,0,0,
      255,159,67, 255,231,75, 255,123,255, 255,0,255, 207,0,207, 159,0,155, 111,0,107, 167,107,107
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
        // XRGB mode: 192000 bytes (64000 pixels × 3 bytes RGB)
        // Palette conversion done in C (doomgeneric-style cmap_to_fb)
        if (data.length >= 192000) {
          for (let i = 0; i < 64000; i++) {
            const si = i * 3;
            const di = i * 4;
            img.data[di]     = data[si];     // R
            img.data[di + 1] = data[si + 1]; // G
            img.data[di + 2] = data[si + 2]; // B
            img.data[di + 3] = 255;          // A
          }
        } else if (data.length >= 64768) {
          // Legacy: palette + indices
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
        if (e.repeat) return; // suppress browser auto-repeat, only initial press matters
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
