// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package metrics

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"syscall"
)

const processSupported = true

// userHZ is the unit of the tick fields in /proc/<pid>/stat. The kernel fixes
// it at 100 for userspace on every architecture regardless of CONFIG_HZ, and
// prometheus/procfs hardcodes the same value.
const userHZ = 100

func readProcess() (procSnapshot, error) {
	st, err := parseProcStat("/proc/self/stat")
	if err != nil {
		return procSnapshot{}, err
	}
	var nofile, as syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &nofile); err != nil {
		return procSnapshot{}, fmt.Errorf("getrlimit nofile: %w", err)
	}
	if err := syscall.Getrlimit(syscall.RLIMIT_AS, &as); err != nil {
		return procSnapshot{}, fmt.Errorf("getrlimit as: %w", err)
	}
	s := deriveProcess(st, int64(os.Getpagesize()), nofile, as)
	if s.openFDs, err = countOpenFDs(); err != nil {
		return procSnapshot{}, err
	}
	return s, nil
}

// deriveProcess turns raw readings into published units. It is pure so it
// can be tested with values that tell the fields apart: a test process has
// used ~0 CPU ticks and usually has soft == hard rlimits, so live readings
// cannot distinguish utime from utime+stime, ticks from seconds, or the soft
// limit from the hard one.
func deriveProcess(st procStat, pageSize int64, nofile, as syscall.Rlimit) procSnapshot {
	return procSnapshot{
		cpuSeconds:  float64(st.utime+st.stime) / userHZ,
		virtualMem:  float64(st.vsize),
		residentMem: float64(st.rss * pageSize),
		// The soft limit is what the process actually hits.
		maxFDs: float64(nofile.Cur),
		// RLIM_INFINITY is ^uint64(0); published as 1.8446744073709552e+19,
		// as client_golang does.
		virtualMax: float64(as.Cur),
	}
}

// countOpenFDs uses the Linux 6.2+ fast path — st_size of /proc/self/fd is
// the descriptor count — and falls back to reading the directory. The
// fallback holds one descriptor of its own open while reading, which is not
// counted.
func countOpenFDs() (float64, error) {
	if fi, err := os.Stat("/proc/self/fd"); err == nil && fi.Size() > 0 {
		return float64(fi.Size()), nil
	}
	return countOpenFDsByReadDir()
}

// countOpenFDsByReadDir is the pre-6.2 path. It is a separate function so it
// can be tested on a kernel where the fast path always wins.
func countOpenFDsByReadDir() (float64, error) {
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0, fmt.Errorf("read /proc/self/fd: %w", err)
	}
	return float64(len(ents) - 1), nil
}

type procStat struct {
	utime, stime, starttime, vsize uint64
	rss                            int64
}

// parseProcStat reads the fields this collector needs. comm (field 2) is in
// parentheses and may itself contain spaces and ')', so fields are counted
// from the LAST ')' — splitting the whole line on spaces misreads every
// field after a process named "a b".
func parseProcStat(path string) (procStat, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return procStat{}, fmt.Errorf("read %s: %w", path, err)
	}
	return parseProcStatBytes(data)
}

func parseProcStatBytes(data []byte) (procStat, error) {
	var (
		st  procStat
		err error
	)
	r := bytes.LastIndexByte(data, ')')
	if r < 0 {
		return st, fmt.Errorf("proc stat: no comm terminator")
	}
	f := bytes.Fields(data[r+1:])
	// f[0] is field 3 (state), so field N is f[N-3].
	const (
		fUtime     = 14 - 3
		fStime     = 15 - 3
		fStarttime = 22 - 3
		fVsize     = 23 - 3
		fRSS       = 24 - 3
	)
	if len(f) <= fRSS {
		return st, fmt.Errorf("proc stat: %d fields after comm, need %d", len(f), fRSS+1)
	}
	u := func(i int) (uint64, error) { return strconv.ParseUint(string(f[i]), 10, 64) }
	if st.utime, err = u(fUtime); err != nil {
		return st, fmt.Errorf("proc stat utime: %w", err)
	}
	if st.stime, err = u(fStime); err != nil {
		return st, fmt.Errorf("proc stat stime: %w", err)
	}
	if st.starttime, err = u(fStarttime); err != nil {
		return st, fmt.Errorf("proc stat starttime: %w", err)
	}
	if st.vsize, err = u(fVsize); err != nil {
		return st, fmt.Errorf("proc stat vsize: %w", err)
	}
	if st.rss, err = strconv.ParseInt(string(f[fRSS]), 10, 64); err != nil {
		return st, fmt.Errorf("proc stat rss: %w", err)
	}
	return st, nil
}

// readProcessStartTime is boot time (btime in /proc/stat) plus the process's
// start offset in ticks.
func readProcessStartTime() (float64, error) {
	st, err := parseProcStat("/proc/self/stat")
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, fmt.Errorf("read /proc/stat: %w", err)
	}
	btime, err := parseBtime(data)
	if err != nil {
		return 0, err
	}
	return startTime(btime, st.starttime), nil
}

func startTime(btime, starttimeTicks uint64) float64 {
	return float64(btime) + float64(starttimeTicks)/userHZ
}

func parseBtime(procStat []byte) (uint64, error) {
	for _, line := range bytes.Split(procStat, []byte("\n")) {
		if v, ok := bytes.CutPrefix(line, []byte("btime ")); ok {
			btime, err := strconv.ParseUint(string(bytes.TrimSpace(v)), 10, 64)
			if err != nil {
				return 0, fmt.Errorf("/proc/stat btime: %w", err)
			}
			return btime, nil
		}
	}
	return 0, fmt.Errorf("/proc/stat: no btime")
}
