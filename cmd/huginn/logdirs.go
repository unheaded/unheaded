// SPDX-License-Identifier: GPL-3.0-or-later
//
// logdirs.go — ADR-092 §5 log disk metrics for huginn.
//
// host_disk_* dedups mounts by device, so /var/log on the same device as /var
// is never reported under its own name. These series answer the two questions
// ADR-092 asks directly: how much room does the filesystem holding /var/log
// have left, and which service under /var/log/unheaded/ is using it.
//
// Emitted metrics (all labelled host=):
//   huginn_log_fs_size_bytes{path}     size of the filesystem holding path
//   huginn_log_fs_avail_bytes{path}    bytes available to unprivileged writers
//   huginn_log_root_ok                 1 if /var/log/unheaded was read, 0 if
//                                      absent or unreadable (never a silent 0 B)
//   huginn_log_dir_bytes{dir}          bytes in regular files under each
//                                      service directory; dir="." for files
//                                      directly in the root
//   huginn_log_dir_truncated{dir}      1 if the walk hit its file budget, so
//                                      the size is a lower bound

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	logFSPath = "/var/log"
	logRoot   = "/var/log/unheaded"
	// logWalkBudget bounds the files visited per directory per collection. A
	// service that writes more files than this is itself the anomaly; the
	// truncated flag says the size is a floor.
	logWalkBudget = 10000
)

// LogDir is one service directory's size.
type LogDir struct {
	Dir       string
	Bytes     uint64
	Truncated bool
}

// LogSnapshot is one collection of the log disk metrics.
type LogSnapshot struct {
	FSPath  string
	FSOK    bool
	FSSize  uint64
	FSAvail uint64
	RootOK  bool
	Dirs    []LogDir
}

func collectLogs(fsPath, root string, budget int) LogSnapshot {
	s := LogSnapshot{FSPath: fsPath}
	var st syscall.Statfs_t
	if syscall.Statfs(fsPath, &st) == nil && st.Blocks > 0 {
		s.FSOK = true
		s.FSSize = st.Blocks * uint64(st.Bsize)  // #nosec G115 -- Bsize is a positive block size
		s.FSAvail = st.Bavail * uint64(st.Bsize) // #nosec G115 -- Bsize is a positive block size
	}
	s.Dirs, s.RootOK = logDirs(root, budget)
	return s
}

// logDirs sizes each directory directly under root, plus "." for files
// directly in root. Symlinks are never followed: a link out of the log tree
// would make huginn report (and walk) something that is not a log.
func logDirs(root string, budget int) ([]LogDir, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, false
	}
	top := LogDir{Dir: "."}
	var out []LogDir
	for _, e := range entries {
		switch {
		case e.Type().IsRegular():
			if fi, err := e.Info(); err == nil {
				top.Bytes += uint64(fi.Size()) // #nosec G115 -- a regular file's size is non-negative
			}
		case e.IsDir():
			out = append(out, sizeDir(filepath.Join(root, e.Name()), e.Name(), budget))
		}
	}
	out = append(out, top)
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out, true
}

func sizeDir(path, name string, budget int) LogDir {
	d := LogDir{Dir: name}
	seen := 0
	// WalkDir does not follow symlinks; unreadable subtrees are skipped, not fatal.
	_ = filepath.WalkDir(path, func(_ string, e fs.DirEntry, err error) error {
		if err != nil {
			if e != nil && e.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !e.Type().IsRegular() {
			return nil
		}
		if seen >= budget {
			d.Truncated = true
			return fs.SkipAll
		}
		seen++
		if fi, err := e.Info(); err == nil {
			d.Bytes += uint64(fi.Size()) // #nosec G115 -- a regular file's size is non-negative
		}
		return nil
	})
	return d
}

func promTextLogs(host string, s LogSnapshot) string {
	var b strings.Builder
	if s.FSOK {
		lbl := fmt.Sprintf("{host=%q,path=%q}", host, s.FSPath)
		fmt.Fprintf(&b, "huginn_log_fs_size_bytes%s %g\n", lbl, float64(s.FSSize))
		fmt.Fprintf(&b, "huginn_log_fs_avail_bytes%s %g\n", lbl, float64(s.FSAvail))
	}
	ok := 0
	if s.RootOK {
		ok = 1
	}
	fmt.Fprintf(&b, "huginn_log_root_ok{host=%q} %d\n", host, ok)
	for _, d := range s.Dirs {
		lbl := fmt.Sprintf("{host=%q,dir=%q}", host, d.Dir)
		fmt.Fprintf(&b, "huginn_log_dir_bytes%s %g\n", lbl, float64(d.Bytes))
		t := 0
		if d.Truncated {
			t = 1
		}
		fmt.Fprintf(&b, "huginn_log_dir_truncated%s %d\n", lbl, t)
	}
	return b.String()
}
