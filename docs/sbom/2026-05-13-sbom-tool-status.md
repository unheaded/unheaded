# SBOM Tool Inventory + syft Regen — 2026-05-13

**References:** marshal-drain items 57-59 (SBOM tooling).

## Tool Status (verified today)

| Tool                | Installed?               | Path / Notes                         |
|---------------------|--------------------------|--------------------------------------|
| `syft`              | YES (v1.44.0)            | `/home/govan/.local/bin/syft`        |
| `scancode-toolkit`  | NO                       | `which scancode` empty               |
| `cyclonedx`         | NO                       | `which cyclonedx` empty              |
| `cyclonedx-npm`     | NO                       | `which cyclonedx-npm` empty          |

## syft Regen

Ran `syft dir:. -o text > docs/sbom/2026-05-13-syft.txt`. Exit 0, output
~12,900 lines covering github-actions, python, npm, and rust artifacts.
Committed alongside this status doc.

## Install Recipes (for next sudo-able shift)

```bash
# scancode-toolkit (deeper license detection than syft)
pip install --user scancode-toolkit  # or: apt install scancode-toolkit
# Heavyweight (~1 GB deps); plan disk + time accordingly.

# cyclonedx-npm (npm-tree CycloneDX SBOM)
npm install -g @cyclonedx/cyclonedx-npm
# Needs npm global write perms.

# cyclonedx-cli (general CycloneDX manipulation, optional)
# Download release from https://github.com/CycloneDX/cyclonedx-cli/releases
```

## Recommendation

- **Today:** syft regen committed — drain item 57 advanced.
- **Defer:** scancode-toolkit + cyclonedx installs to a sudo-able shift
  (drain items 58, 59). No blocker on current SBOM work; syft covers the
  primary dependency surface.

---

*Free to use, free to share.*
