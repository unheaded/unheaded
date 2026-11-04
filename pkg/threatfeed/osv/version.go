// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package osv

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Compare orders two versions of eco: negative, zero or positive. Go and
// crates.io follow Semantic Versioning 2.0 (a leading v and +build are
// ignored); PyPI follows PEP 440.
func Compare(eco, a, b string) (int, error) {
	switch eco {
	case EcoGo, EcoCrates:
		return compareSemver(a, b)
	case EcoPyPI:
		return comparePEP440(a, b)
	}
	return 0, fmt.Errorf("%w: unsupported ecosystem %q", ErrInvalid, eco)
}

func splitSemver(v string) (core []int, pre []string, err error) {
	s := strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = strings.Split(s[i+1:], ".")
		s = s[:i]
	}
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, nil, fmt.Errorf("%w: bad semver %q", ErrInvalid, v)
		}
		core = append(core, n)
	}
	return core, pre, nil
}

func compareInts(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func compareSemver(a, b string) (int, error) {
	ca, pa, err := splitSemver(a)
	if err != nil {
		return 0, err
	}
	cb, pb, err := splitSemver(b)
	if err != nil {
		return 0, err
	}
	if c := compareInts(ca, cb); c != 0 {
		return c, nil
	}
	switch {
	case len(pa) == 0 && len(pb) == 0:
		return 0, nil
	case len(pa) == 0:
		return 1, nil
	case len(pb) == 0:
		return -1, nil
	}
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, xerr := strconv.Atoi(pa[i])
		y, yerr := strconv.Atoi(pb[i])
		switch {
		case xerr == nil && yerr == nil:
			if x != y {
				return compareInts([]int{x}, []int{y}), nil
			}
		case xerr == nil:
			return -1, nil
		case yerr == nil:
			return 1, nil
		default:
			if c := strings.Compare(pa[i], pb[i]); c != 0 {
				return c, nil
			}
		}
	}
	return compareInts([]int{len(pa)}, []int{len(pb)}), nil
}

var pep440 = regexp.MustCompile(`^(?:(\d+)!)?(\d+(?:\.\d+)*)(?:[-_.]?(a|alpha|b|beta|c|rc|pre|preview)[-_.]?(\d*))?(?:[-_.]?(post|rev|r)[-_.]?(\d*))?(?:[-_.]?(dev)[-_.]?(\d*))?$`)

// pep440Key is the sort key: epoch, then release, then these four in order.
type pep440Key struct {
	epoch   int
	release []int
	tail    [4]int // pre kind, pre number, post number, dev number
}

func num(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func parsePEP440(v string) (pep440Key, error) {
	s := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	m := pep440.FindStringSubmatch(s)
	if m == nil {
		return pep440Key{}, fmt.Errorf("%w: bad PEP 440 version %q", ErrInvalid, v)
	}
	k := pep440Key{epoch: num(m[1])}
	for _, p := range strings.Split(m[2], ".") {
		k.release = append(k.release, num(p))
	}
	hasPost, hasDev := m[5] != "", m[7] != ""
	switch m[3] {
	case "a", "alpha":
		k.tail[0], k.tail[1] = -3, num(m[4])
	case "b", "beta":
		k.tail[0], k.tail[1] = -2, num(m[4])
	case "c", "rc", "pre", "preview":
		k.tail[0], k.tail[1] = -1, num(m[4])
	default:
		if hasDev && !hasPost {
			k.tail[0] = -4 // 1.0.dev1 sorts before 1.0a1
		}
	}
	k.tail[2] = -1
	if hasPost {
		k.tail[2] = num(m[6])
	}
	k.tail[3] = math.MaxInt
	if hasDev {
		k.tail[3] = num(m[8])
	}
	return k, nil
}

func comparePEP440(a, b string) (int, error) {
	ka, err := parsePEP440(a)
	if err != nil {
		return 0, err
	}
	kb, err := parsePEP440(b)
	if err != nil {
		return 0, err
	}
	if c := compareInts([]int{ka.epoch}, []int{kb.epoch}); c != 0 {
		return c, nil
	}
	if c := compareInts(ka.release, kb.release); c != 0 {
		return c, nil
	}
	return compareInts(ka.tail[:], kb.tail[:]), nil
}
