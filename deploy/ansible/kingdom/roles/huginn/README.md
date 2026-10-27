# huginn

The host metrics agent (ADR-084) the way `deploy/systemd/huginn.service` documents
installing it: binary in `/usr/local/bin`, the repo's unit, `/etc/huginn.env`
(root:root 0600), enabled and started. Runs as `unheaded-huginn` (kingdom-base
creates it).

Variables:

- `unheaded_repo`: path to a checkout on the controller.
- `huginn_binary`: a built `huginn` on the controller (`go build ./cmd/huginn`).
- `huginn_vm`: push target; defaults to west's own VictoriaMetrics on west and to
  west over the P2P link on east.

The env file is written only when the host has none; the Well password for
`huginn_reader` (`PGPASSWORD`) is set on the host by hand and never comes from
this repository.
