# Local font assets — LAN-only posture

WAVE15 Phase 5: vendor the Google Fonts CSS + woff2 files locally so the
Zhen web UI renders correctly with no internet connection (per Stevie's
operating vision: "0 internet, just LAN").

## Status

🟡 **Not yet vendored.** `raft/static/index.html` lines 7-10 still reference
`fonts.googleapis.com` directly. On a no-internet host the page will:

- Render (HTML + CSS + JS work fine; the Google Fonts request fails silently)
- Fall back to the system mono / sans-serif fonts (ugly but functional)
- Throw a console error visible in DevTools

This is an **acceptable degraded state** — not a blocker. Fixing it is
a one-time chore for an operator with internet access.

## Vendoring procedure (one-time, requires internet)

```bash
cd ~/tmp/unheaded/raft/static/fonts/

# 1. Fetch the Google Fonts CSS for the two faces the UI uses.
wget -O jetbrains.css \
    'https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@300;400;700&display=swap'
wget -O space-grotesk.css \
    'https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@300;400;500;700&display=swap'

# 2. Extract the woff2 URLs and download each font file.
grep -oE 'https://[^)]*\.woff2' jetbrains.css space-grotesk.css \
    | sort -u \
    | while read -r url; do
        out=$(basename "$url")
        wget -O "$out" "$url"
    done

# 3. Rewrite the CSS files so url(...) points at local paths.
for css in jetbrains.css space-grotesk.css; do
    sed -i -E 's|https://fonts\.gstatic\.com/[^)]*/([^/)]+\.woff2)|/static/fonts/\1|g' "$css"
done

# 4. Update raft/static/index.html (lines 7-10) to point at the local CSS:
#    Replace:
#      <link rel="preconnect" href="https://fonts.googleapis.com">
#      <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
#      <link href="https://fonts.googleapis.com/css2?...JetBrains+Mono...Space+Grotesk..." rel="stylesheet" media="print" onload="this.media='all'">
#      <noscript><link href="https://fonts.googleapis.com/css2?..." rel="stylesheet"></noscript>
#    With:
#      <link href="/static/fonts/jetbrains.css"     rel="stylesheet">
#      <link href="/static/fonts/space-grotesk.css" rel="stylesheet">
```

After vendoring, verify LAN-only posture:

```bash
# Block external HTTP/S
sudo iptables -A OUTPUT -p tcp --dport 443 ! -d 127.0.0.0/8 \
    ! -d 192.168.0.0/16 ! -d 10.0.0.0/8 -j REJECT
sudo iptables -A OUTPUT -p tcp --dport 80  ! -d 127.0.0.0/8 \
    ! -d 192.168.0.0/16 ! -d 10.0.0.0/8 -j REJECT

# Open the UI in a browser. Fonts should render correctly with no
# external network calls (verify in DevTools Network tab).

# Restore networking
sudo iptables -F OUTPUT
```

## Why not just always vendor?

The original `raft/static/index.html` was authored against fonts.googleapis.com
because it's the path of least resistance. WAVE15 documents the LAN-only
posture commitment in `docs/battle-plans/WAVE15-ZHENAI-REWIRE.md` §3
("No internet at runtime"); this directory is the place that commitment
lands as a real artifact.

We don't auto-vendor at build time because:

1. Build hosts may not have internet either.
2. Google Fonts file URLs include cache-bust hashes that change; pinning
   them needs a manual review step anyway.
3. The vendoring is a one-time operator chore, not an every-CI-run task.

## References

- `docs/battle-plans/WAVE15-ZHENAI-REWIRE.md` §Phase 5 — operational chores
- `~/.claude/plans/synthetic-stirring-pudding.md` §3 — LAN-only posture commitment
- `docs/security/application-threat-model.md` (no T-entry; this is a
  posture commitment, not a threat — the LAN-only smoke verifies no
  outbound traffic on a hardened host)
