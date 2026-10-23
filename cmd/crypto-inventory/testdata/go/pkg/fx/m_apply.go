// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import (
	"crypto/tls"

	pq "unheaded/pkg/tls"
)

// (m) ApplyStrict on a literal argument: the profile wins over the 1.2 floor.
func strictDirect() *tls.Config {
	return pq.ApplyStrict(&tls.Config{
		MinVersion: tls.VersionTLS12,
	})
}

// (m) ApplyPreferred through a variable drops the classical pin.
func preferredVar() *tls.Config {
	cfg := &tls.Config{
		CurvePreferences: []tls.CurveID{tls.X25519},
	}
	other := &tls.Config{}
	_ = other
	pq.ApplyPreferred(cfg)
	return cfg
}

type holder struct {
	min uint16
	cfg *tls.Config
}

// (m) ApplyStrict on a field a literal with a run-time floor was stored in.
func (h *holder) strictField() {
	h.cfg = &tls.Config{
		MinVersion: h.min,
	}
	pq.ApplyStrict(h.cfg)
}
