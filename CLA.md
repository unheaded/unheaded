# Contributor License Agreement (DCO)

SPDX-License-Identifier: GPL-3.0-or-later | Copyright (c) 2024-2026 Stevie Bellis | **Last updated:** 2026-03-25

## Overview

Unheaded uses a **Developer Certificate of Origin (DCO)** process, the same
lightweight approach used by the Linux kernel, Git, and hundreds of other open
source projects. There is no separate CLA form to sign and no copyright
assignment required.

By adding a `Signed-off-by` line to your commit message, you certify that you
wrote the contribution (or have the right to submit it) and that you agree to
release it under the project's license.

## Developer Certificate of Origin v1.1

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

## How to Sign Off

Add a `Signed-off-by` line at the end of every commit message:

```
feat(wotan): add gRPC reflection support

Enables dynamic service discovery via gRPC reflection API.

Signed-off-by: Your Name <your.email@example.com>
```

Git can do this automatically with the `-s` flag:

```bash
git commit -s -m "feat(wotan): add gRPC reflection support"
```

## Requirements

1. **Every commit** in a pull request must carry a valid `Signed-off-by` line.
2. The name and email must match your Git identity (`user.name` / `user.email`).
3. Pseudonyms are acceptable if they are consistently used and legally traceable.

## Project License

All contributions are accepted under the project's **GNU General Public
License v3.0 or later** (GPL-3.0-or-later). See [LICENSE](LICENSE) for the
full text.

Protocol specification contributions under `docs/protocol/` are dual-licensed
**GPL-3.0-or-later / Apache-2.0**. See [LICENSE-PROTOCOLS](LICENSE-PROTOCOLS)
for details.

## Patent Grant

By signing off on a contribution, you also grant a perpetual, worldwide,
non-exclusive, royalty-free patent license under any patent claims you own or
control that are necessarily infringed by your contribution, consistent with
GPL-3.0 Section 11 (Patents).

## Enforcement

Pull requests without valid DCO sign-off will not be merged. A CI check
validates the `Signed-off-by` trailer on every commit. If you forget, you can
fix it:

```bash
# Amend the most recent commit
git commit --amend -s

# Rebase and sign off all commits in a branch
git rebase HEAD~N --signoff
```

## Why DCO Instead of a Heavyweight CLA?

- **Lower barrier to entry.** No forms to sign, no accounts to create.
- **Proven at scale.** The Linux kernel has used DCO since 2004 with thousands
  of contributors.
- **Legally sufficient.** The DCO provides an adequate chain of provenance for
  GPL-3.0 contributions.
- **Contributor-friendly.** You retain copyright on your contributions. No
  assignment to a corporate entity.

## Questions

If you have questions about the DCO or contribution process, open a GitHub
Discussion or email stevie@bellis.tech.

---

*This document is part of the Unheaded legal framework. See also:*
- *[LICENSE](LICENSE) -- GPL-3.0-or-later*
- *[LICENSE-PROTOCOLS](LICENSE-PROTOCOLS) -- Dual GPL-3.0/Apache-2.0 for specs*
- *[CONTRIBUTING.md](CONTRIBUTING.md) -- Full contribution guide*
- *[docs/legal/](docs/legal/) -- Additional legal documentation*
