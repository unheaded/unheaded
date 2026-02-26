#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
set -euo pipefail
exec /usr/lib/frr/watchfrr zebra ospf6d bfdd staticd mgmtd
