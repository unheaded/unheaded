// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"io"
	"log/slog"

	"unheaded/pkg/modules"
)

// workerModuleOff reports whether a -worker run (interactive, -once or
// -overnight) must do nothing because the Zhen module's switch is off
// (ADR-149). The overnight run is a user unit outside unheaded-zhen.target,
// so the switch cannot stop it; this check does. When it returns true it has
// written one JSON log line to w; the caller exits 0 before any pool opens,
// so no card is claimed or moved.
func workerModuleOff(wf workerFlags, w io.Writer) bool {
	if !wf.enabled || modules.Enabled("zhen") {
		return false
	}
	mode := "interactive"
	switch {
	case wf.overnight:
		mode = "overnight"
	case wf.once:
		mode = "once"
	}
	slog.New(slog.NewJSONHandler(w, nil)).Info(
		"the zhen module is off: the worker claims no card (ADR-149)",
		"service", "zhen-agentd", "module", "zhen", "mode", mode, "modules_dir", modules.Dir())
	return true
}
