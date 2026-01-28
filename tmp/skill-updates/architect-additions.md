# Architect Additions

**Add to the "Reference Docs" section:**

---

## Reference Docs (Updated)

- `references/nixos-patterns.md` - NixOS module patterns, flake structures
- `references/network-fabrics.md` - BGP/EVPN/VXLAN design patterns
- `references/ebpf-recipes.md` - eBPF programs for observability/security
- `references/hardening-checklist.md` - Headers to kernel security baseline
- `references/project-roadmap.md` - Detailed phase breakdown and progress

### Cross-Reference: Developer Skill

For eBPF development patterns (aya-rs, XDP/TC programs, BPF maps, testing):
→ See `unheaded-developer/references/ebpf-dev.md`

For service implementation patterns (Go microservices, Busboy integration):
→ See `unheaded-developer/references/service-template.md`

### Cross-Reference: Micromanager Skill

For QA checkpoints and Definition of Done:
→ See `unheaded-micromanager` for task completion criteria

For security isolation verification:
→ Micromanager owns the customer data isolation checklist

---

**Add to end of "Quick Patterns" section:**

### Busboy Client Integration

All services MUST use the Busboy client for inter-service communication:

```go
import busboyClient "github.com/unheaded/unheaded/pkg/busboy-client"

// In service initialization
client, err := busboyClient.NewClient("localhost:9090")
if err != nil {
    return fmt.Errorf("busboy connect: %w", err)
}
defer client.Close()

// Subscribe to relevant topics
sub, err := client.Subscribe(ctx, "system.events", serviceName)
if err != nil {
    return fmt.Errorf("subscribe: %w", err)
}

// Wait for approval (may be auto-approved in dev)
if sub.Status == "pending" {
    // Handle pending state
}

// Publish events
err = client.Publish(ctx, "system.events", payload)

// Stream messages
msgCh, err := client.StreamMessages(ctx, "system.events")
for msg := range msgCh {
    // Handle message
}
```

**Topic Conventions:**
- `system.*` - Infrastructure events
- `ebpf.*` - eBPF probe data
- `tasks.*` - Kanban/task updates
- `metrics.*` - Metrics pipeline
- `<service>.*` - Service-specific topics
