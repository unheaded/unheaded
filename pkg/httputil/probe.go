// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import "net/http"

// ProbeMethods limits a /health or /ready handler to GET and HEAD, the
// methods probes use; anything else is 405 with an Allow header. Services
// used to disagree: some answered 200 to any method (BREW included), others
// refused HEAD. net/http drops the body of a HEAD response by itself.
func ProbeMethods(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h(w, r)
	}
}
