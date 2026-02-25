# Kingdom Mode Math Verification

Mathematical verification of the Extended Register Space formulas:

```
reclaimed = 2 * (128 - host_bits)
```

| Mode | Host Bits | Reclaimed Bits | Reclaimed Bytes | Max Hosts |
|------|-----------|---------------|-----------------|-----------|
| /8 | 8 | 208 | 26 | 16.7M |
| /12 | 12 | 216 | 27 | 1M |
| /16 | 16 | 224 | 28 | 65K |

---

> **Source:** [docs/KINGDOM_MODE_MATH_VERIFICATION.md](../docs/KINGDOM_MODE_MATH_VERIFICATION.md)
