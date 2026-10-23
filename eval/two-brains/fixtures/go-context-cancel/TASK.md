<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# go-context-cancel

`Retry` keeps retrying after its context is cancelled. It must stop and
return `ctx.Err()` as soon as the context is done (check it before each
attempt), and still return nil on the first success.
