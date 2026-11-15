// SPDX-License-Identifier: GPL-3.0-or-later

package sophia

// The listed hand-written HKDF: found by its function, not by a constant.
func hkdfExtract(ikm, salt []byte) []byte { return append(ikm, salt...) }
