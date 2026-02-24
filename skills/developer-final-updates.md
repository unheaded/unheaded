# Developer Skill Final Updates

Based on this session's work, here are the final adjustments for `unheaded-developer.skill`:

---

## 1. Update Wotan Integration Section (SKILL.md ~line 473)

**Replace the existing Wotan integration code block with:**

```go
import wotanClient "github.com/unheaded/unheaded/pkg/wotan-client"

// Connect
client, err := wotanClient.NewClient("localhost:9090")
if err != nil {
    return fmt.Errorf("connect to wotan: %w", err)
}
defer client.Close()

// Subscribe (requires approval workflow)
sub, err := client.Subscribe(ctx, "events.system", "my-service")
if err != nil {
    return fmt.Errorf("subscribe: %w", err)
}
// Note: sub.Status may be "pending" until approved

// Publish (only works after subscription approved)
err = client.Publish(ctx, "events.system", payload)

// Stream messages
msgCh, err := client.StreamMessages(ctx, "events.system")
for msg := range msgCh {
    // handle msg.Payload
}

// Get historical messages
msgs, err := client.GetMessages(ctx, "events.system", afterSeq, limit)
```

**Why:** The actual client API uses `wotanClient` package name and has specific Subscribe/Publish/StreamMessages methods that differ from the example.

---

## 2. Add Mock Client for Testing Section (After Wotan Integration)

**Add new section:**

```markdown
### Testing with Mock Client

For unit tests, use the mock client:

```go
import "github.com/unheaded/unheaded/pkg/wotan-client/mock"

func TestMyService(t *testing.T) {
    // Create mock with auto-approve (skips approval workflow)
    mockClient := mock.NewMockClient(mock.WithAutoApprove())
    defer mockClient.Close()

    // Subscribe works immediately
    sub, err := mockClient.Subscribe(ctx, "test.topic", "test-svc")
    require.NoError(t, err)
    assert.Equal(t, "approved", sub.Status)

    // Publish
    err = mockClient.Publish(ctx, "test.topic", []byte("test"))
    require.NoError(t, err)

    // Verify calls
    assert.Equal(t, int64(1), mockClient.GetPublishCount())

    // Inject messages for testing receivers
    mockClient.InjectMessage("test.topic", `{"type":"test"}`)

    // Error injection for failure testing
    mockClient.SetError("publish", errors.New("simulated failure"))
}
```

**Mock Options:**
- `WithAutoApprove()` - Skip subscription approval
- `WithLatency(d)` - Simulate network latency
- `WithSubscribeError(err)` - Inject subscribe failures
- `WithPublishError(err)` - Inject publish failures
```

---

## 3. Update Current Project State Table (SKILL.md ~line 46)

**Replace with:**

```markdown
### Current Project State (Auto-Sync)

**Phase 1 Alpha**: 80-85% COMPLETE | ETA: Feb 9-15, 2026

| Shipped ✅ | In Progress 🚧 | Blocked ⏸️ |
|-----------|----------------|------------|
| Wotan Phase 1 (complete!) | Kanban Frontend (75%) | eBPF probes (needs Linux env) |
| Control Plane (5,784 LOC) | Dashboard (20%) | |
| Microservices (8,235 LOC) | Container Stack (audit needed) | |
| Wotan Go Client Library | | |
| Mock Client for Testing | | |
| CI/CD Pipelines | | |

**Total Shipped:** ~35,000+ LOC
```

---

## 4. Add CI/CD Section (New, after Security Checklist)

```markdown
## CI/CD Integration

All PRs run through GitHub Actions (`.github/workflows/ci.yml`):

```yaml
# Automatic on every PR:
- go-lint: golangci-lint
- go-test: go test -v -race -coverprofile
- go-build: compile all binaries
- security-scan: govulncheck + gosec
- rust-check: cargo fmt + clippy (for eBPF)
- proto-lint: buf lint (if protos exist)
- benchmark: compare against baseline
```

### Pre-Push Checklist

```bash
# Run locally before pushing
make test-race        # Race detection
make lint             # golangci-lint
make test-coverage    # Check coverage
go mod tidy           # Clean deps
```

### Coverage Gates

| Component | Target | Current |
|-----------|--------|---------|
| wotan/internal/* | 80% | Check PROGRESS.md |
| pkg/wotan-client | 80% | ✅ |
| cmd/* | 60% | Varies |
```

---

## 5. Add Proto Patterns Reference

**Add to references list at bottom:**

```markdown
- `references/proto-patterns.md` - Protocol Buffer & gRPC patterns, buf configuration
```

**Note:** The `developer-proto-patterns.md` file I created should be added as `references/proto-patterns.md` in the skill.

---

## 6. Session Start Protocol Enhancement

**Add to Session Start Protocol (after step 4):**

```markdown
5. INVOKE TIMEGURU
   If timeline feels stale or you're unsure of current state,
   invoke the unheaded-timeguru skill for canonical status.

   THE TIMEGURU KNOWS ALL. Trust it over cached mental models.
```

---

## Summary of Files to Update

1. **SKILL.md** - Apply changes above
2. **Add `references/proto-patterns.md`** - Copy from `developer-proto-patterns.md`

## Files Already Complete (No Changes Needed)

- `references/testing-patterns.md` ✅
- `references/secure-coding.md` ✅
- `references/legends.md` ✅
- `references/ebpf-dev.md` ✅
- `references/service-template.md` ✅
- `scripts/scaffold-go.sh` ✅

---

## One More Thing: The Squaresoft Test

The skill mentions it but doesn't emphasize enough:

> **The Squaresoft Test**: Before marking anything "done", ask yourself:
>
> *"Would this be worthy of appearing in a Final Fantasy title screen?"*
>
> If not, keep iterating. Polish is not optional. We ship **jewels**, not rough drafts.

This is the vibe. The code we write isn't just "working" - it's **crafted**.
