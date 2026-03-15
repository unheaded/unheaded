# Unheaded Alpha Demo — The Protocol Awakens

**Title**: Unheaded Alpha Demo — The Protocol Awakens  
**Duration**: ~5 minutes  
**Audience**: Technical (SRE/Platform engineers), Investors  
**Resolution**: 1080p60 minimum  
**Format**: Narrated screencast with live UI interaction  

---

## Pre-Production Setup Checklist

### Infrastructure Requirements
- [ ] **protocol-api** running in mock mode on `:16666`
- [ ] **dashboard-backend** running on `:16667`
- [ ] **wotan** message bus live and emitting demo data
- [ ] **demo-data.js** enabled (green "DEMO MODE" badge visible)
- [ ] All 8 dashboard tabs loading without errors
- [ ] Mock DOOM tab rendering screen buffer updates
- [ ] Headers tab showing real UNHEADED_METRIC_V1 breakdowns
- [ ] Network latency <10ms (for smooth WebSocket updates)

### Recording Equipment
- [ ] Screen recording software: OBS Studio, Quicktime, or ffmpeg
- [ ] Resolution: 1920×1080 at 60fps
- [ ] Audio: Clear microphone (USB or built-in with noise reduction)
- [ ] Backup audio recording (Audacity or inline)
- [ ] Test run: 30-second recording, verify quality before full take

### Preparation Steps
```bash
# 1. Clear browser cache
rm -rf ~/.cache/google-chrome ~/.config/chromium ~/.mozilla/firefox

# 2. Start Unheaded services (from repo root)
cd /path/to/unheaded
./scripts/demo-start.sh  # or: make up

# 3. Wait for services to stabilize (30 seconds)
sleep 30

# 4. Verify endpoints
curl -s http://localhost:16666/api/v1/health | jq
curl -s http://localhost:16667/api/v1/health | jq

# 5. Open browser to dashboard
open http://localhost:16667/dashboard

# 6. Verify demo mode badge
curl -s http://localhost:16667/api/v1/config | jq .demo_mode  # Should be true

# 7. Ready to record
```

---

## Scene 1: The Dashboard (0:00–1:00)

### Narration
> "This is the Unheaded dashboard. Eight tabs. Every metric, trace, and packet flow in real time. All running from a single Go binary."

### Actions
1. **[0:00]** Browser at `localhost:16667/dashboard`
   - Full dashboard visible, all 8 tabs across the top
   - Live packet counter in Packet Flow tab incrementing (~8,600/sec)
   - Services tab showing 25 green service cards

2. **[0:15]** Slow hover over each tab header, highlighting its name
   - Packet Flow → (pause 0.5s)
   - Traces → (pause 0.5s)
   - Services → (pause 0.5s)
   - Infrastructure → (pause 0.5s)
   - Logs → (pause 0.5s)
   - AI Inference → (pause 0.5s)
   - DOOM → (pause 0.5s)
   - Headers → (pause 0.5s)

3. **[0:40]** Click into **Traces** tab
   - Show waterfall view: gateway → protocol-api → wotan → dashboard-backend
   - Each span shows trace ID, span ID, start time, duration
   - Highlight: "Latency 12ms, all from packet headers"

4. **[0:55]** Return to **Packet Flow** tab
   - Packet counter now shows ~43,000 packets (grew since we started)
   - Show real-time histogram of packet sizes and latencies

### Visual Indicators
- Ensure demo mode badge is visible (top-right corner): "🟢 DEMO MODE"
- All text should be readable at 1080p (no font <10pt)
- Smooth WebSocket updates; no visible stutter

### Audio Cues
- Soft background: A-minor ambient tone (loopable, 5 min duration)
- No sound effects (maintain professionalism for investor audience)

---

## Scene 2: The Protocol (1:00–2:00)

### Narration
> "Under the hood: the Unheaded Protocol. Three primitives — Monad, Sophia, Wotan. Monad: a 20-byte register living in IPv6 Hop-by-Hop headers. The packet is the message."

### Actions
1. **[1:00]** Minimize browser, open terminal
   - Terminal background: dark (solarized dark or equivalent)
   - Font: Monospace (Monaco, Fira Code), 14pt
   - Zoom: 150% for readability in video

2. **[1:05]** Run command to fetch a live Monad register
   ```bash
   curl -s http://localhost:16666/api/v1/monad | jq '.[0]'
   ```
   Output example:
   ```json
   {
     "timestamp": 1709059200123456,
     "service_id": 5,
     "hop_count": 2,
     "status": "2xx",
     "trace_id": "ae3d4c8f9b2e1a5c7f3d9c2e1b8a9d3f",
     "span_id": "8f3d9c2e1b8a9d3f"
   }
   ```

3. **[1:20]** Explain each field on screen with voice-over
   - Narrator: "Timestamp in microseconds, Service ID for routing, Status code inline—"
   - Use terminal highlighting or annotations to point to each line

4. **[1:35]** Show another Monad example, different service
   ```bash
   curl -s http://localhost:16666/api/v1/monad | jq '.[5]'
   ```

5. **[1:50]** Transition: "The same data lives in every packet's Hop-by-Hop header."
   - Type: "hexdump -C /tmp/unheaded_sample.pcap | head -20" (or show pre-recorded hex dump)
   - Show raw bytes with annotations overlay

### Visual Indicators
- Terminal text must be legible (high contrast)
- JSON output well-formatted with syntax highlighting
- Use terminal multiplexer (tmux) to keep context

---

## Scene 3: DOOM over IPv6 (2:00–3:00)

### Narration
> "DOOM. Running inside IPv6 packets. Each packet executes 16 MBC instructions. At 8,600 packets/second, Doom runs at ~30 FPS — over the network layer."

### Actions
1. **[2:00]** Switch back to browser, click **DOOM** tab
   - DOOM game screen visible (retro ASCII or pixel art render)
   - Bottom-left: packet counter incrementing
   - Bottom-right: FPS indicator (~30 FPS) and packet rate (8.6K/sec)

2. **[2:10]** Show DOOM title screen / in-game scene
   - Screen buffer should update in real time as packets arrive
   - Smooth animation (not stuttering)

3. **[2:25]** Demonstrate DOOM movement: Press arrow keys
   - Each arrow key press → sends packet with MBC movement instruction
   - Screen buffer updates immediately (latency <50ms)
   - Narrator: "WASD to move, Space to jump. All encoded in IPv6 packets."

4. **[2:45]** Show a sequence of movement
   - Walk the player forward (3–4 arrow presses)
   - Show the DOOM sprite moving through the virtual space
   - Packet counter increments with each action

5. **[2:55]** Final view: DOOM screen, highlight the stats
   - "16 MBC instructions per packet"
   - "8,600 packets/second = 137K instructions/sec = ~30 FPS"
   - Narrator: "Computational completeness, at line rate."

### Visual Indicators
- DOOM window must be 1080p-compatible size (not full 4K)
- Packet counter in corner should be readable
- FPS must be stable (no significant variance >5%)

### Technical Notes
- DOOM is running in a JavaScript Canvas implementation
- MBC bytecode interpreter translates IPv6 packet payload to screen buffer writes
- See: `pkg/doom/` for implementation
- Mock mode generates synthetic DOOM packets; no actual game client needed for demo

---

## Scene 4: The Trace (3:00–4:00)

### Narration
> "Every service call is traced. Trace ID, Span ID, timestamps — all embedded IN the packet. No sidecar. No collector. The packet IS the telemetry."

### Actions
1. **[3:00]** Return to browser, click **Traces** tab
   - Waterfall diagram visible showing multi-service call chain
   - 5–7 spans in a single trace

2. **[3:10]** Click into a specific trace (largest latency)
   - Show full span details:
     - Root span: "gateway" (span_id: 0x8f3d9c2e...)
     - Child span 1: "protocol-api" (parent: gateway)
     - Child span 2: "wotan" (parent: protocol-api)
     - Child span 3: "dashboard-backend" (parent: wotan)

3. **[3:25]** Highlight the Flow Label field in the packet
   - Narrator: "See this 20-bit Flow Label? It's a fast-path: Service ID, Status, Latency, Flags."
   - Show breakdown diagram overlay:
     ```
     20 bits: [SVC:4][STATUS:4][LATENCY_BUCKET:8][FLAGS:4]
     ```

4. **[3:40]** Click the **Headers** tab
   - Show the UNHEADED_METRIC_V1 header breakdown
   - Hex dump of the 52-byte Hop-by-Hop option
   - Decoded fields:
     ```
     Trace ID:   0xae3d4c8f9b2e1a5c7f3d9c2e1b8a9d3f
     Span ID:    0x8f3d9c2e1b8a9d3f
     Timestamp:  2026-02-25T03:20:00.123456Z
     Latency:    2450 μs
     Service:    wotan
     Status:     2xx
     ```

5. **[3:55]** Narrator: "Trace context, in every single packet. From the application all the way to XDP."

### Visual Indicators
- Waterfall chart should be clean and readable
- Span timelines must be to scale
- Headers hex dump must be monospaced and aligned

### Color Scheme (Consistent Throughout)
- Trace IDs: Teal (#20B2AA)
- Span IDs: Gold (#FFD700)
- Timestamps: Silver (#C0C0C0)
- Status 2xx: Green (#00AA00)
- Status 3xx: Cyan (#00AAFF)
- Status 4xx: Yellow (#FFAA00)
- Status 5xx: Red (#FF0000)

---

## Scene 5: The Stack (4:00–5:00)

### Narration
> "25 services. gRPC-first. Service discovery. Log aggregation. All managed by the Unheaded daemon. Infrastructure-as-code from config to container in one command."

### Actions
1. **[4:00]** Click **Services** tab
   - Show 25 service cards in a grid layout
   - Cards should include:
     - Service name (e.g., "wotan", "timeguru", "captain")
     - Status indicator (green circle = healthy)
     - Request count (e.g., "2.4M requests")
     - Error rate (e.g., "0.02%")
     - Latency p95 (e.g., "12ms")

2. **[4:15]** Hover over a service card to expand details
   - Show: Port number, protocol (gRPC / HTTP), uptime
   - Click the service → shows service-specific traces

3. **[4:30]** Switch to terminal, show Infrastructure-as-Code
   ```bash
   cat unheaded-config.yaml | head -30
   ```
   Output:
   ```yaml
   services:
     wotan:
       port: 16667
       protocol: grpc
       replicas: 1
       healthcheck:
         path: /health
         interval: 5s
       logging:
         level: info
         aggregator: chronicle
     timeguru:
       port: 16668
       protocol: grpc
       replicas: 2
       # ... more services ...
   ```

4. **[4:45]** Show the generate command in action
   ```bash
   unheaded generate --backend=docker
   # Output: Generated docker-compose.yml (25 services)
   cat docker-compose.yml | wc -l  # Output: ~500 lines
   ```

5. **[4:55]** Return to browser, final dashboard view
   - Show all 8 tabs again
   - Packet counter now at ~2.5M packets
   - All service cards green and healthy
   - Narrator: "Infrastructure-as-code. Service mesh. Observability. Security. All from one config."

### Visual Indicators
- Service cards should be consistently sized
- Status indicators must be obvious (green/red colors)
- Metrics should update in real time (live animation)

---

## Final Frame (5:00)

### End Card (3 seconds static)
```
Unheaded Alpha Demo
github.com/stevenrbellis/unheaded

Stevie Bellis
stevie@bellis.tech

The packet IS the telemetry.
```

### Audio
- Fade ambient tone to silence over last 3 seconds
- Add subtle "complete" tone (e.g., G major chord) at very end

---

## Production Quality Checklist

### Video Quality
- [ ] 1920×1080 resolution
- [ ] 60fps frame rate
- [ ] H.264 codec (mp4 container)
- [ ] Bitrate: 8–12 Mbps (for YouTube streaming)
- [ ] No frame drops during recording
- [ ] Color grading: Neutral (no heavy filters)

### Audio Quality
- [ ] Narration: Clear, no background noise, 44.1 kHz 16-bit
- [ ] Ambient tone: Consistent volume, no clipping
- [ ] Sync: Narration in sync with on-screen events (±200ms tolerance)
- [ ] Levels: Narration at -6dB, ambient at -18dB, mixing to -3dB peak

### Browser Rendering
- [ ] No console errors (F12 → Console)
- [ ] All WebSocket connections active
- [ ] No CSS layout shifts (jank)
- [ ] Smooth scrolling in traces list
- [ ] Readable fonts at playback resolution

### Accessibility
- [ ] Captions/subtitles (SRT format) provided separately
- [ ] High contrast colors (pass WCAG AA standard)
- [ ] No flickering content (safe for photosensitive viewers)

---

## Post-Production Tasks

### Editing
1. Trim intro/outro silence (leave 0.5s buffer)
2. Add title card: "Unheaded Alpha Demo" (2 seconds at start)
3. Add scene transitions (fade black, 0.3s between scenes)
4. Speed up non-critical moments (e.g., terminal output waiting)
   - Terminal rendering: 1.5× speed is imperceptible
   - Dashboard tab switching: 1× speed (keep real-time feel)
5. Color grading pass (ensure consistent brightness across scenes)
6. Normalize audio levels (-16 LUFS for YouTube)

### Subtitles/Captions
- [ ] Transcribe narration (use Otter.ai or manual)
- [ ] Add timing information (start/end in ms)
- [ ] Add captions for on-screen text (JSON output, hex dumps)
- [ ] Format as SRT (SubRip) and VTT for YouTube

### Delivery Formats
- [ ] **YouTube**: H.264 MP4, 1080p60, 8–12 Mbps, SRT captions
- [ ] **GitHub**: Same as YouTube, upload to Releases
- [ ] **Conference**: MOV or H.264 AVI for local playback (no streaming artifacts)
- [ ] **Backup**: High-bitrate master (ProRes or DNxHD) for archival

### Distribution
- [ ] Upload to YouTube (Unlisted initially for review)
- [ ] Add to GitHub: `docs/demo/ALPHA_DEMO_VIDEO.mp4`
- [ ] Update wiki: Add link to video from Home.md
- [ ] Announce: Twitter, LinkedIn, Hacker News with GitHub link

---

## Troubleshooting During Recording

| Problem | Solution |
|---------|----------|
| Browser WebSocket not updating | Restart dashboard-backend: `systemctl restart unheaded-dashboard` |
| Packet counter not incrementing | Verify wotan service: `curl localhost:16667/health` |
| DOOM tab frozen | Refresh browser (Cmd+R), wait 5 seconds for MBC interpreter to warm up |
| Terminal text too small | Increase font to 16pt and zoom browser to 150% |
| Audio out of sync | Re-record; sync video and audio in post-production using ffmpeg |
| CPU maxed out | Close other applications, reduce browser tab count to 1 |
| Mouse lag or stuttering | Disable hardware acceleration in browser (Settings → Advanced) |

---

## Alternative Demo Scenarios (Fallback)

### If Live Demo Fails
1. **Pre-recorded segments**: Record 5 × 1-minute segments offline, edit together
2. **Screenshot gallery**: If UI broken, use high-res PNG screenshots with narration overlay
3. **Terminal-only demo**: Show all data via curl + jq, no browser UI
4. **Code walkthrough**: Instead of demo, show and explain key code files:
   - `pkg/protocol/monad.go` (Monad wire format)
   - `pkg/ebpf/xdp_metric.c` (XDP program)
   - `pkg/dashboard/app.js` (Frontend WebSocket handling)

---

## Script Callouts for Director / Editor

### Critical Timing Points
- **[1:50]** Transition from terminal to hex dump must be quick (not>1s wait)
- **[2:10]** DOOM screen must render and update smoothly; no frozen frames
- **[3:25]** Headers tab load must complete before narrator mentions Flow Label
- **[4:30]** YAML file must display and be readable; use `less` with line breaks clear
- **[5:00]** Final end card must be visible for exactly 3 seconds

### Key Visual Moments (B-Roll / Close-ups)
- Packet counter hitting 100K (satisfying milestone)
- Trace waterfall with color-coded spans
- Hex dump with trace ID highlighted
- Service grid all green (healthy state)

### Narration Emphasis Points (Bold delivery)
- *"The packet IS the telemetry."* — Slow, deliberate (0.5s pause after)
- *"No sidecar. No collector."* — Confident, slightly faster
- *"25 services. One binary."* — With amazement (rising intonation)
- *"Sub-microsecond overhead."* — Technical, matter-of-fact

---

## Narrator Style Guide

- **Pace**: 110 WPM (slightly faster than conversational, not rushed)
- **Tone**: Technical expert explaining to engineers (not sales pitch)
- **Enthusiasm**: Genuine curiosity about the innovation, not over-the-top
- **Pauses**: 0.5–1.0s between sentences for visual focus
- **Pronunciation**:
  - Unheaded: UN-hed-ed (not un-HEED-ed)
  - Monad: MON-ad
  - Sophia: so-FEE-ah
  - Wotan: VO-tahn (German pronunciation)
  - IPv6: IP-v-six (not "IP version six")
  - XDP: ex-dee-pee
  - eBPF: ee-bee-puff

---

## Final Checklist Before Publishing

- [ ] Video plays smoothly on YouTube (test view as unlisted first)
- [ ] Audio sync verified (A/V lip-sync test)
- [ ] Subtitles display correctly (test on mobile + desktop)
- [ ] GitHub link in description is clickable and correct
- [ ] License information visible (MIT)
- [ ] No sensitive info (IP addresses, passwords, keys) visible on screen
- [ ] Thumbnail generated (custom PNG with logo/text)
- [ ] Description includes:
  - GitHub repo link
  - Timestamps for each scene (0:00 Dashboard, 1:00 Protocol, etc.)
  - Links to related docs (RFC, IANA, architecture)
  - Contact info (GitHub, email)
  - Hashtags (#Unheaded #IPv6 #eBPF #USENIX)

---

## Example Video Description (YouTube)

```
Unheaded Alpha Demo — The Protocol Awakens

This is the alpha demonstration of Unheaded, a configuration 
management platform built on the Unheaded Protocol, embedded 
in IPv6 Hop-by-Hop headers with eBPF-powered observability.

Watch as 25 microservices operate with zero sidecars, all 
telemetry flowing through the network itself at <1μs overhead 
per hop.

CHAPTERS:
0:00 — Dashboard Overview
1:00 — The Protocol (Monad)
2:00 — DOOM over IPv6
3:00 — Distributed Tracing
4:00 — Infrastructure as Code

RESOURCES:
GitHub: https://github.com/stevenrbellis/unheaded
Docs: https://github.com/stevenrbellis/unheaded/wiki
RFC Alignment: https://github.com/stevenrbellis/unheaded/docs/RFC-ALIGNMENT.md

CONTACT:
Stevie Bellis (stevenrbellis@github.com)
stevie@bellis.tech

Licensed under Busl 1.1.
```

---

*End of Demo Script*

**Duration**: 5 minutes ± 10 seconds  
**Last Updated**: February 25, 2026
