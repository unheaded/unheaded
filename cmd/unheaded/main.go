// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the GNU General Public License v3.0 or later.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

package main

import (
	"log"

	"unheaded/pkg/cmdline"
)

func main() {
	cmdline.Parse(cmdline.Spec{
		Name:    "unheaded",
		Purpose: "Liveness stub for the Unheaded module; the operator CLI is unheaded-cli (the Gauntlets).",
	})
	log.Println("✅ unheaded is alive")
}
