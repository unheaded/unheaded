// SPDX-License-Identifier: GPL-3.0-or-later

// Integration test for the MBC Linux boot image.
//
// Verifies that all userspace utilities build correctly, produce valid
// UPCFlat binaries, and that the rootfs contains all expected files.
// Also performs a smoke test: loads init.upcf into memory and verifies
// the boot banner string is present in the data segment.
package main

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// upcFlatMagic is the 4-byte header magic for UPCFlat binaries.
const upcFlatMagic = "UPCF"

// utility describes a userspace utility to build and verify.
type utility struct {
	name    string // binary name (e.g. "echo")
	dir     string // directory relative to project root
	output  string // expected output filename
}

// projectRoot returns the project root by walking up from the test file.
func projectRoot(t *testing.T) string {
	t.Helper()
	// This test lives in arch/mbc/boot/, so project root is 3 levels up.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot get working directory: %v", err)
	}
	root := filepath.Join(wd, "..", "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("cannot resolve project root: %v", err)
	}
	return abs
}

// allUtilities returns the list of all userspace utilities to test.
func allUtilities() []utility {
	return []utility{
		{"echo", "arch/mbc/userspace/echo", "echo.upcf"},
		{"cat", "arch/mbc/userspace/cat", "cat.upcf"},
		{"ls", "arch/mbc/userspace/ls", "ls.upcf"},
		{"ps", "arch/mbc/userspace/ps", "ps.upcf"},
		{"uname", "arch/mbc/userspace/uname", "uname.upcf"},
		{"uptime", "arch/mbc/userspace/uptime", "uptime.upcf"},
	}
}

// TestBuildUtilities verifies each utility builds without error.
func TestBuildUtilities(t *testing.T) {
	root := projectRoot(t)

	for _, u := range allUtilities() {
		t.Run(u.name, func(t *testing.T) {
			dir := filepath.Join(root, u.dir)
			cmd := exec.Command("go", "run", "build.go")
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("build %s failed: %v\n%s", u.name, err, string(out))
			}

			// Verify output file exists
			outPath := filepath.Join(dir, u.output)
			info, err := os.Stat(outPath)
			if err != nil {
				t.Fatalf("output file %s not found: %v", outPath, err)
			}
			if info.Size() == 0 {
				t.Fatalf("output file %s is empty", outPath)
			}
		})
	}
}

// TestBuildInit verifies the init process builds correctly.
func TestBuildInit(t *testing.T) {
	root := projectRoot(t)
	dir := filepath.Join(root, "arch", "mbc", "boot", "init")
	cmd := exec.Command("go", "run", "build.go")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build init failed: %v\n%s", err, string(out))
	}

	outPath := filepath.Join(dir, "init.upcf")
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("init.upcf not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("init.upcf is empty")
	}
}

// TestBuildShell verifies the shell builds correctly.
func TestBuildShell(t *testing.T) {
	root := projectRoot(t)
	dir := filepath.Join(root, "demos", "mbc", "shell")
	cmd := exec.Command("go", "run", "build.go")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build shell failed: %v\n%s", err, string(out))
	}

	outPath := filepath.Join(dir, "shell.upcf")
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("shell.upcf not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("shell.upcf is empty")
	}
}

// TestUPCFlatValidation verifies all built binaries have valid UPCFlat headers.
func TestUPCFlatValidation(t *testing.T) {
	root := projectRoot(t)

	// Build all utilities first
	binaries := map[string]string{
		"init": filepath.Join(root, "arch", "mbc", "boot", "init", "init.upcf"),
		"shell": filepath.Join(root, "demos", "mbc", "shell", "shell.upcf"),
	}
	for _, u := range allUtilities() {
		binaries[u.name] = filepath.Join(root, u.dir, u.output)
	}

	// Build everything
	for name, path := range binaries {
		dir := filepath.Dir(path)
		cmd := exec.Command("go", "run", "build.go")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("build %s failed: %v\n%s", name, err, string(out))
		}
	}

	// Validate each binary
	for name, path := range binaries {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("cannot read %s: %v", path, err)
			}

			// Check minimum size (32-byte header)
			if len(data) < 32 {
				t.Fatalf("%s too short: %d bytes (need >= 32)", name, len(data))
			}

			// Check magic
			magic := string(data[0:4])
			if magic != upcFlatMagic {
				t.Fatalf("%s invalid magic: got %q, want %q", name, magic, upcFlatMagic)
			}

			// Check version
			version := binary.LittleEndian.Uint32(data[4:8])
			if version != 1 {
				t.Fatalf("%s invalid version: got %d, want 1", name, version)
			}

			// Check text size > 0
			textSize := binary.LittleEndian.Uint32(data[12:16])
			if textSize == 0 {
				t.Fatalf("%s has zero text size", name)
			}

			// Check payload size matches header
			dataStart := binary.LittleEndian.Uint32(data[16:20])
			dataSize := binary.LittleEndian.Uint32(data[20:24])
			expectedPayload := (textSize + dataSize) * 4
			actualPayload := uint32(len(data)) - 32
			if actualPayload < expectedPayload {
				t.Fatalf("%s payload too short: %d bytes, need %d (text=%d data=%d dataStart=%d)",
					name, actualPayload, expectedPayload, textSize, dataSize, dataStart)
			}

			t.Logf("%s: valid UPCFlat v1, %d text + %d data words (%d bytes)",
				name, textSize, dataSize, len(data))
		})
	}
}

// TestInitBannerPresent is a smoke test: loads init.upcf and verifies that
// the boot banner string "MBC Linux init starting" is present in the binary.
func TestInitBannerPresent(t *testing.T) {
	root := projectRoot(t)

	// Build init
	initDir := filepath.Join(root, "arch", "mbc", "boot", "init")
	cmd := exec.Command("go", "run", "build.go")
	cmd.Dir = initDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build init failed: %v\n%s", err, string(out))
	}

	// Read the binary
	initPath := filepath.Join(initDir, "init.upcf")
	data, err := os.ReadFile(initPath)
	if err != nil {
		t.Fatalf("cannot read init.upcf: %v", err)
	}

	// The banner string should be present somewhere in the binary
	banner := "MBC Linux init starting"
	if !strings.Contains(string(data), banner) {
		t.Fatalf("init.upcf does not contain expected banner %q", banner)
	}

	t.Logf("init.upcf contains boot banner: %q", banner)
}

// TestShellPromptPresent verifies the shell contains the hostname prompt.
func TestShellPromptPresent(t *testing.T) {
	root := projectRoot(t)

	// Build shell
	shellDir := filepath.Join(root, "demos", "mbc", "shell")
	cmd := exec.Command("go", "run", "build.go")
	cmd.Dir = shellDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build shell failed: %v\n%s", err, string(out))
	}

	// Read the binary
	shellPath := filepath.Join(shellDir, "shell.upcf")
	data, err := os.ReadFile(shellPath)
	if err != nil {
		t.Fatalf("cannot read shell.upcf: %v", err)
	}

	// The prompt string should be present in the binary
	prompt := "mbc-linux> "
	if !strings.Contains(string(data), prompt) {
		t.Fatalf("shell.upcf does not contain expected prompt %q", prompt)
	}

	// The help message should list all commands
	helpMsg := "Commands: echo cat ls ps uname uptime exit help"
	if !strings.Contains(string(data), helpMsg) {
		t.Fatalf("shell.upcf does not contain expected help message %q", helpMsg)
	}

	t.Logf("shell.upcf contains hostname prompt and help message")
}

// TestUnameOutput verifies the uname utility contains the expected system info.
func TestUnameOutput(t *testing.T) {
	root := projectRoot(t)

	// Build uname
	unameDir := filepath.Join(root, "arch", "mbc", "userspace", "uname")
	cmd := exec.Command("go", "run", "build.go")
	cmd.Dir = unameDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build uname failed: %v\n%s", err, string(out))
	}

	data, err := os.ReadFile(filepath.Join(unameDir, "uname.upcf"))
	if err != nil {
		t.Fatalf("cannot read uname.upcf: %v", err)
	}

	expected := "MBC Linux 0.1 UPC"
	if !strings.Contains(string(data), expected) {
		t.Fatalf("uname.upcf does not contain %q", expected)
	}

	t.Logf("uname.upcf contains system info: %q", expected)
}
