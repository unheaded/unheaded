# Norse Mythology — Protocol & Messaging Names

Wotan (the Germanic form of Odin from Wagner's Ring Cycle) is the message bus at the center of the Unheaded service mesh. The naming draws from both Norse mythology and Wagnerian opera.

## Wotan → Message Bus

| Wotan Role | Mythological Aspect | Technical Function |
|-----------|---------------------|-------------------|
| Ring Buffer | Odin's ravens (Thought & Memory) | Lock-free eBPF perf_event ring. Receives kernel events. |
| Event Bus | Odin in Hlidskjalf (sees all) | Pub/sub gRPC/HTTP with topic routing. |
| Protocol RAM | Odin's runes (encoded knowledge) | BPF map memory substrate for Monad compute. |

## Reserved Names

| Name | Mythological Source | Reserved For |
|------|-------------------|-------------|
| **Mysteltainn** | Mistletoe that killed Baldr | Penetration testing / vulnerability discovery |
| **Tyrfing** | Cursed sword — must kill when drawn | Irreversible operations (migrations, destructive deploys) |
| **Nagan** | Jörmungandr / World Serpent (ouroboros) | Global state sync / ring topology |
| **Halcyon** | Alcyone / kingfisher — calm seas | Graceful shutdown / steady-state mode |
| **Nibelung** | Das Rheingold | Resource hoarding detection (memory/CPU) |
| **Brünnhilde** | Die Walküre | Firewall / perimeter defense |
| **Siegfried** | Siegfried | Fearless mode — no retries, no circuit breakers (testing) |
| **Götterdämmerung** | Götterdämmerung | Full system teardown / disaster recovery test |

---

> **Source:** [docs/lore/NORSE_MYTHOLOGY.md](../docs/lore/NORSE_MYTHOLOGY.md)
