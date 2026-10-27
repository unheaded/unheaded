#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
set -euo pipefail
exec /usr/lib/frr/watchfrr zebra ospf6d bfdd staticd mgmtd
