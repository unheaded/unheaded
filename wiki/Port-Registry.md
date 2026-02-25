# Port Registry — The Doom Range

All Unheaded services use high ports (16666-26666) to avoid conflicts with common dev tools.

## Port Allocation Table

| Tier | Range | Services |
|------|-------|----------|
| Infrastructure | 16666-16999 | doom-bridge (16666), doom-injector (16667), trace-collector (16670/16671) |
| Control Plane | 17000-17999 | unheaded-daemon HTTP (17000), gRPC (17001) |
| Wotan | 18000-18099 | wotan HTTP (18000), gRPC (18001) |
| Core Services | 19000-19999 | timeguru (19000), architect (19001), captain (19002), micromanager (19003), monad (19004), sophia (19005) |
| Applications | 20000-20999 | dashboard (20000), kanban (20001), wiki (20002) |
| Gateway | 21000-21443 | HTTP (21000), HTTPS (21443) |
| User Apps | 26000-26666 | Reserved for user applications |

## Implementation

Port constants are defined in `pkg/ports/ports.go`. Use `ports.DefaultAddr()` for lookups.

Static fallback configuration: `configs/services.yaml`

## Rationale

High ports avoid conflicts with databases (5432, 3306, 6379), web servers (80, 443, 8080), and common dev tools. The "Doom Range" name comes from the project's Doom-over-eBPF demo that occupies port 16666.
