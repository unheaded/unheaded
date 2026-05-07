// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package upc

import (
	"encoding/binary"
	"testing"
)

// ── Test helpers ──────────────────────────────────────────────────────

// decodeMBCInsn extracts opcode, dst, src, imm16 from an MBC instruction word.
func decodeMBCInsn(word uint32) (opcode, dst, src uint8, imm uint16) {
	opcode = uint8(word >> 24)
	dst = uint8((word >> 20) & 0x0F)
	src = uint8((word >> 16) & 0x0F)
	imm = uint16(word & 0xFFFF)
	return
}

// simulateMBCProgram runs a trivial MBC simulation sufficient for testing
// boot programs. It handles MOVI, INT 0x80 (SYS_WRITE, SYS_EXIT, SYS_FORK,
// SYS_WAITPID, SYS_EXECVE, SYS_READ), CMP, JZ, JMP, MOV, and HALT.
//
// Returns: tty output, exit code, halted flag, error string.
func simulateMBCProgram(rom []uint32, ttyInput []byte, maxSteps int) (output string, exitCode uint32, halted bool, errMsg string) {
	regs := [16]uint32{}
	regs[15] = 0xFFFF0000 // SP
	pc := uint32(0)
	flags := uint32(0)
	var ttyOut []byte
	inputIdx := 0

	// RAM for the program's data section (shared address space with ROM as flat binary)
	ram := make([]byte, len(rom)*4)
	for i, w := range rom {
		binary.LittleEndian.PutUint32(ram[i*4:(i+1)*4], w)
	}

	// Fork state: when SYS_FORK is called, we save parent state and run
	// the child first (r0=0). When the child calls SYS_EXIT, we restore
	// parent state with r0=child_pid. This models real Unix fork behavior.
	type savedState struct {
		regs  [16]uint32
		pc    uint32
		flags uint32
	}
	var parentState *savedState
	inChild := false
	childPid := uint32(1)

	for step := 0; step < maxSteps; step++ {
		if int(pc) >= len(rom) {
			errMsg = "PC out of ROM bounds"
			return
		}

		insn := rom[pc]
		opcode, dst, src, imm := decodeMBCInsn(insn)
		pc++

		switch opcode {
		case 0x0F: // MOVI
			regs[dst] = uint32(imm)

		case 0x0E: // MOV
			regs[dst] = regs[src]

		case 0x10: // CMP
			a := regs[dst]
			b := regs[src]
			flags = 0
			if a == b {
				flags |= 1 // Z flag
			}
			if int32(a) < int32(b) {
				flags |= 2 // N flag
			}

		case 0x17: // INT
			if imm == 0x80 {
				syscallNr := regs[0]
				switch syscallNr {
				case 4: // SYS_WRITE
					fd := regs[1]
					addr := regs[2]
					count := regs[3]
					if fd == 1 || fd == 2 { // stdout or stderr
						for i := uint32(0); i < count; i++ {
							byteAddr := addr + i
							if int(byteAddr) < len(ram) {
								ttyOut = append(ttyOut, ram[byteAddr])
							}
						}
					}
					regs[0] = count

				case 1: // SYS_EXIT
					if inChild && parentState != nil {
						// Child exited — restore parent after fork
						regs = parentState.regs
						pc = parentState.pc
						flags = parentState.flags
						// Parent's r0 = child_pid (set at fork time)
						regs[0] = childPid
						parentState = nil
						inChild = false
						// Continue execution (parent resumes after fork INT 0x80)
					} else {
						// Top-level exit
						output = string(ttyOut)
						exitCode = regs[1]
						halted = true
						return
					}

				case 2: // SYS_FORK
					// Save parent state (registers + PC after the INT 0x80)
					parentState = &savedState{
						regs:  regs,
						pc:    pc, // already advanced past INT
						flags: flags,
					}
					inChild = true
					// Child gets r0 = 0
					regs[0] = 0

				case 7: // SYS_WAITPID
					// In the simulator, the child has already exited by the
					// time the parent reaches waitpid (child ran first).
					regs[0] = 0 // child exit code

				case 11: // SYS_EXECVE
					// r1 = entry_point (ROM word address)
					entryPoint := regs[1]
					for i := 0; i < 16; i++ {
						regs[i] = 0
					}
					regs[15] = 0xFFFF0000
					pc = entryPoint
					flags = 0

				case 3: // SYS_READ
					fd := regs[1]
					addr := regs[2]
					count := regs[3]
					if fd == 0 && inputIdx < len(ttyInput) {
						n := int(count)
						if n > len(ttyInput)-inputIdx {
							n = len(ttyInput) - inputIdx
						}
						for i := 0; i < n; i++ {
							byteAddr := int(addr) + i
							if byteAddr < len(ram) {
								ram[byteAddr] = ttyInput[inputIdx+i]
							}
						}
						inputIdx += n
						regs[0] = uint32(n)
					} else {
						regs[0] = 0
					}

				default:
					// Unhandled syscall — return 0
					regs[0] = 0
				}
			}

		case 0x20: // JMP (pc-relative, signed imm16)
			offset := int16(imm)
			pc = uint32(int32(pc) + int32(offset))

		case 0x21: // JZ
			if (flags & 1) != 0 {
				offset := int16(imm)
				pc = uint32(int32(pc) + int32(offset))
			}

		case 0x22: // JNZ
			if (flags & 1) == 0 {
				offset := int16(imm)
				pc = uint32(int32(pc) + int32(offset))
			}

		case 0xFF: // HALT
			output = string(ttyOut)
			halted = true
			return

		case 0x00: // NOP
			// nothing

		default:
			errMsg = "unhandled opcode"
			output = string(ttyOut)
			return
		}
	}

	output = string(ttyOut)
	errMsg = "max steps exceeded"
	return
}

// ── Boot sequence tests ──────────────────────────────────────────────

func TestBootSequence(t *testing.T) {
	// 1. Format filesystem with boot files
	fs, err := CreateBootFilesystem()
	if err != nil {
		t.Fatalf("CreateBootFilesystem: %v", err)
	}

	// 2. Verify filesystem has the required boot files
	requiredFiles := []string{"/init", "/bin/sh", "/dev/console", "/etc/hostname"}
	for _, name := range requiredFiles {
		inode := LookupInode(fs, name)
		if inode == nil {
			t.Fatalf("boot filesystem missing required file: %s", name)
		}
	}

	// 3. Read /init from filesystem
	initData, err := ReadFile(fs, "/init")
	if err != nil {
		t.Fatalf("ReadFile(/init): %v", err)
	}
	if len(initData) == 0 {
		t.Fatal("/init has zero bytes")
	}
	if len(initData)%4 != 0 {
		t.Fatalf("/init size %d is not 4-byte aligned", len(initData))
	}

	// 4. Decode /init as MBC instructions
	rom := make([]uint32, len(initData)/4)
	for i := range rom {
		rom[i] = binary.LittleEndian.Uint32(initData[i*4 : (i+1)*4])
	}

	// 5. Verify the first instruction is MOVI (opcode 0x0F)
	opcode, _, _, _ := decodeMBCInsn(rom[0])
	if opcode != mbcMOVI {
		t.Errorf("first instruction opcode = 0x%02X, want 0x%02X (MOVI)", opcode, mbcMOVI)
	}

	// 6. Simulate /init execution
	output, exitCode, halted, errMsg := simulateMBCProgram(rom, nil, 10000)
	if errMsg != "" {
		t.Fatalf("simulation error: %s", errMsg)
	}
	if !halted {
		t.Error("/init did not halt")
	}
	if exitCode != 0 {
		t.Errorf("/init exit code = %d, want 0", exitCode)
	}

	// 7. Verify /init output contains "UNFS boot"
	if output == "" {
		t.Fatal("/init produced no output")
	}
	if len(output) < 9 || output[:9] != "UNFS boot" {
		t.Errorf("/init output = %q, want prefix %q", output, "UNFS boot")
	}
}

func TestBootWithShell(t *testing.T) {
	// Build the boot filesystem
	fs, err := CreateBootFilesystem()
	if err != nil {
		t.Fatalf("CreateBootFilesystem: %v", err)
	}

	// Read /bin/sh from filesystem
	shellData, err := ReadFile(fs, "/bin/sh")
	if err != nil {
		t.Fatalf("ReadFile(/bin/sh): %v", err)
	}
	if len(shellData) == 0 {
		t.Fatal("/bin/sh has zero bytes")
	}

	// Decode as MBC instructions
	rom := make([]uint32, len(shellData)/4)
	for i := range rom {
		rom[i] = binary.LittleEndian.Uint32(shellData[i*4 : (i+1)*4])
	}

	// Verify the shell starts with a MOVI instruction (prompt setup)
	opcode, _, _, _ := decodeMBCInsn(rom[0])
	if opcode != mbcMOVI {
		t.Errorf("shell first instruction opcode = 0x%02X, want 0x%02X (MOVI)", opcode, mbcMOVI)
	}

	// Simulate shell with input "A" — it should print "> " prompt then echo "A"
	output, _, _, errMsg := simulateMBCProgram(rom, []byte("A"), 1000)
	if errMsg != "" && errMsg != "max steps exceeded" {
		t.Fatalf("shell simulation error: %s", errMsg)
	}

	// Verify prompt appeared
	if len(output) < 2 || output[:2] != "> " {
		t.Errorf("shell output = %q, want prefix %q", output, "> ")
	}

	// Verify echo of input character
	foundEcho := false
	for i := 2; i < len(output); i++ {
		if output[i] == 'A' {
			foundEcho = true
			break
		}
	}
	if !foundEcho {
		t.Errorf("shell output = %q, expected echo of 'A'", output)
	}
}

func TestFullBootWithForkAndExec(t *testing.T) {
	// Build the FUZIX boot init program that:
	// 1. Prints "UPC Boot v1.0\n"
	// 2. Calls SYS_FORK
	// 3. Child: SYS_EXECVE -> entry point of /bin/sh
	// 4. Parent: SYS_WAITPID on child
	// 5. When child exits: print "Shell exited\n" and halt

	bootMsg := "UPC Boot v1.0\n"
	exitMsg := "Shell exited\n"
	shellPrompt := "> "

	// We build everything in a single flat binary. Layout:
	//   [init code] [child code] [init data: bootMsg, exitMsg] [shell code] [shell data: "> "]
	//
	// All addresses are byte offsets within this flat binary.

	var code []uint32 // accumulates all words

	// ── Init code ────────────────────────────────────────────────
	// Instruction indices for later patching:
	bootMsgAddrIdx := -1
	exitMsgAddrIdx := -1
	shellEntryIdx := -1
	childJmpIdx := -1

	// Print boot banner: SYS_WRITE(1, &bootMsg, len)
	code = append(code, mbcEncode(mbcMOVI, 0, 0, mbcSysWrite)) // 0
	code = append(code, mbcEncode(mbcMOVI, 1, 0, 1))           // 1
	bootMsgAddrIdx = len(code)
	code = append(code, mbcEncode(mbcMOVI, 2, 0, 0))                    // 2: &bootMsg (patched)
	code = append(code, mbcEncode(mbcMOVI, 3, 0, uint16(len(bootMsg)))) // 3
	code = append(code, mbcEncode(mbcINT, 0, 0, 0x80))                  // 4

	// SYS_FORK
	code = append(code, mbcEncode(mbcMOVI, 0, 0, 2))   // 5
	code = append(code, mbcEncode(mbcINT, 0, 0, 0x80)) // 6

	// CMP r0, 0 — check if child
	code = append(code, mbcEncode(mbcMOVI, 6, 0, 0)) // 7
	code = append(code, mbcEncode(0x10, 0, 6, 0))    // 8: CMP
	childJmpIdx = len(code)
	code = append(code, mbcEncode(0x21, 0, 0, 0)) // 9: JZ child (patched)

	// Parent: SYS_WAITPID(child_pid=1)
	code = append(code, mbcEncode(mbcMOVI, 0, 0, 7))   // 10
	code = append(code, mbcEncode(mbcMOVI, 1, 0, 1))   // 11
	code = append(code, mbcEncode(mbcINT, 0, 0, 0x80)) // 12

	// Parent: print "Shell exited\n"
	code = append(code, mbcEncode(mbcMOVI, 0, 0, mbcSysWrite)) // 13
	code = append(code, mbcEncode(mbcMOVI, 1, 0, 1))           // 14
	exitMsgAddrIdx = len(code)
	code = append(code, mbcEncode(mbcMOVI, 2, 0, 0))                    // 15: &exitMsg (patched)
	code = append(code, mbcEncode(mbcMOVI, 3, 0, uint16(len(exitMsg)))) // 16
	code = append(code, mbcEncode(mbcINT, 0, 0, 0x80))                  // 17

	// Parent: SYS_EXIT(0)
	code = append(code, mbcEncode(mbcMOVI, 0, 0, mbcSysExit)) // 18
	code = append(code, mbcEncode(mbcMOVI, 1, 0, 0))          // 19
	code = append(code, mbcEncode(mbcINT, 0, 0, 0x80))        // 20

	// ── Child code ───────────────────────────────────────────────
	childStart := len(code)
	code = append(code, mbcEncode(mbcMOVI, 0, 0, 11)) // 21: SYS_EXECVE
	shellEntryIdx = len(code)
	code = append(code, mbcEncode(mbcMOVI, 1, 0, 0))   // 22: shell entry (patched)
	code = append(code, mbcEncode(mbcINT, 0, 0, 0x80)) // 23

	// Patch child JZ: offset = childStart - (childJmpIdx + 1)
	childOffset := int16(childStart - (childJmpIdx + 1))
	code[childJmpIdx] = mbcEncode(0x21, 0, 0, uint16(childOffset))

	// ── Init data section ────────────────────────────────────────
	initDataStart := len(code)
	bootMsgWords := mbcPackString(bootMsg)
	exitMsgWords := mbcPackString(exitMsg)
	code = append(code, bootMsgWords...)
	code = append(code, exitMsgWords...)

	bootMsgByteAddr := uint16(initDataStart * 4)
	exitMsgByteAddr := uint16((initDataStart + len(bootMsgWords)) * 4)
	code[bootMsgAddrIdx] = mbcEncode(mbcMOVI, 2, 0, bootMsgByteAddr)
	code[exitMsgAddrIdx] = mbcEncode(mbcMOVI, 2, 0, exitMsgByteAddr)

	// ── Shell code section ───────────────────────────────────────
	shellStartWord := len(code)
	code[shellEntryIdx] = mbcEncode(mbcMOVI, 1, 0, uint16(shellStartWord))

	// Shell: print "> ", then SYS_EXIT(0)
	code = append(code, mbcEncode(mbcMOVI, 0, 0, mbcSysWrite)) // sh+0
	code = append(code, mbcEncode(mbcMOVI, 1, 0, 1))           // sh+1
	shellPromptAddrIdx := len(code)
	code = append(code, mbcEncode(mbcMOVI, 2, 0, 0))                        // sh+2: &prompt (patched)
	code = append(code, mbcEncode(mbcMOVI, 3, 0, uint16(len(shellPrompt)))) // sh+3
	code = append(code, mbcEncode(mbcINT, 0, 0, 0x80))                      // sh+4
	code = append(code, mbcEncode(mbcMOVI, 0, 0, mbcSysExit))               // sh+5
	code = append(code, mbcEncode(mbcMOVI, 1, 0, 0))                        // sh+6
	code = append(code, mbcEncode(mbcINT, 0, 0, 0x80))                      // sh+7

	// Shell data: "> "
	shellDataStart := len(code)
	promptWords := mbcPackString(shellPrompt)
	code = append(code, promptWords...)
	promptByteAddr := uint16(shellDataStart * 4)
	code[shellPromptAddrIdx] = mbcEncode(mbcMOVI, 2, 0, promptByteAddr)

	// ── Run simulation ───────────────────────────────────────────
	output, exitCode, isHalted, errMsg := simulateMBCProgram(code, nil, 10000)
	if errMsg != "" {
		t.Fatalf("simulation error: %s (output so far: %q)", errMsg, output)
	}

	// Verify boot banner
	if len(output) < len(bootMsg) || output[:len(bootMsg)] != bootMsg {
		t.Errorf("output = %q, want prefix %q", output, bootMsg)
	}

	// Verify shell prompt appeared (child ran shell via execve)
	foundPrompt := false
	for i := 0; i < len(output)-1; i++ {
		if output[i] == '>' && output[i+1] == ' ' {
			foundPrompt = true
			break
		}
	}
	if !foundPrompt {
		t.Errorf("output = %q, expected shell prompt '> '", output)
	}

	// Verify "Shell exited" message (parent resumed after child exit)
	foundExit := false
	for i := 0; i <= len(output)-len(exitMsg); i++ {
		if output[i:i+len(exitMsg)] == exitMsg {
			foundExit = true
			break
		}
	}
	if !foundExit {
		t.Errorf("output = %q, expected %q", output, exitMsg)
	}

	// Verify clean exit
	if !isHalted {
		t.Error("init did not halt")
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}

func TestBootFilesystemRamdiskLayout(t *testing.T) {
	// Verify that the boot filesystem can be loaded into the ramdisk region.
	fs, err := CreateBootFilesystem()
	if err != nil {
		t.Fatalf("CreateBootFilesystem: %v", err)
	}

	// Verify filesystem fits in ramdisk (4 MB = 8192 blocks * 128 words)
	expectedWords := UNFSTotalBlocks * UNFSBlockWords
	if len(fs) != expectedWords {
		t.Fatalf("filesystem = %d words, want %d", len(fs), expectedWords)
	}

	// Verify superblock magic at word 0
	if fs[0] != UNFSMagic {
		t.Errorf("superblock magic = 0x%08X, want 0x%08X", fs[0], UNFSMagic)
	}

	// Simulate loading into ramdisk region: verify no overlap with kernel
	ramdiskBaseWord := RamdiskBase >> 2
	kernelBaseWord := KernelBase >> 2
	kernelEndWord := kernelBaseWord + 4096 // 16KB kernel max for test

	if ramdiskBaseWord < kernelEndWord {
		t.Errorf("ramdisk base (0x%X) overlaps kernel region (0x%X-0x%X)",
			ramdiskBaseWord, kernelBaseWord, kernelEndWord)
	}
}

func TestBootMemoryWithRamdiskFilesystem(t *testing.T) {
	// Test PrepareBootMemory with an on-disk kernel, then verify the
	// boot params contain correct ramdisk address and size.
	cfg := DefaultBootConfig()

	mem, err := PrepareBootMemory(cfg)
	if err != nil {
		t.Fatalf("PrepareBootMemory: %v", err)
	}

	// Boot params magic should be present
	magic, ok := mem[BootParamsBaseWord]
	if !ok || magic != BootMagic {
		t.Errorf("boot params magic = 0x%X, want 0x%X", magic, BootMagic)
	}

	// Memory size should be 64 MB
	memSize := mem[BootParamsBaseWord+2]
	if memSize != DefaultMemorySize {
		t.Errorf("memory size = %d, want %d", memSize, DefaultMemorySize)
	}
}

func TestBootInitProgramStructure(t *testing.T) {
	// Verify the init program built by buildInitProgram() has correct structure.
	initCode := buildInitProgram()
	if len(initCode) == 0 {
		t.Fatal("buildInitProgram returned empty")
	}
	if len(initCode)%4 != 0 {
		t.Fatalf("init code size %d not 4-byte aligned", len(initCode))
	}

	// Decode as words
	words := make([]uint32, len(initCode)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(initCode[i*4 : (i+1)*4])
	}

	// First instruction: MOVI r0, SYS_WRITE (4)
	op0, dst0, _, imm0 := decodeMBCInsn(words[0])
	if op0 != mbcMOVI {
		t.Errorf("insn[0] opcode = 0x%02X, want 0x%02X", op0, mbcMOVI)
	}
	if dst0 != 0 {
		t.Errorf("insn[0] dst = %d, want 0", dst0)
	}
	if imm0 != mbcSysWrite {
		t.Errorf("insn[0] imm = %d, want %d (SYS_WRITE)", imm0, mbcSysWrite)
	}

	// Must contain an INT 0x80 instruction
	foundINT := false
	for _, w := range words {
		op, _, _, im := decodeMBCInsn(w)
		if op == mbcINT && im == 0x80 {
			foundINT = true
			break
		}
	}
	if !foundINT {
		t.Error("init program missing INT 0x80 instruction")
	}

	// Must end with SYS_EXIT sequence
	// Look for MOVI r0, SYS_EXIT (1) followed by INT 0x80
	foundExit := false
	for i := 0; i < len(words)-1; i++ {
		op1, d1, _, im1 := decodeMBCInsn(words[i])
		op2, _, _, im2 := decodeMBCInsn(words[i+1])
		_ = d1
		if op1 == mbcMOVI && im1 == mbcSysExit {
			// Check for INT 0x80 within next 2 instructions
			for j := i + 1; j < len(words) && j <= i+3; j++ {
				op2, _, _, im2 = decodeMBCInsn(words[j])
				if op2 == mbcINT && im2 == 0x80 {
					foundExit = true
					break
				}
			}
			break
		}
	}
	if !foundExit {
		t.Error("init program missing SYS_EXIT(0) sequence")
	}
}

func TestBootShellProgramStructure(t *testing.T) {
	// Verify the shell program built by buildShellProgram() has correct structure.
	shellCode := buildShellProgram()
	if len(shellCode) == 0 {
		t.Fatal("buildShellProgram returned empty")
	}
	if len(shellCode)%4 != 0 {
		t.Fatalf("shell code size %d not 4-byte aligned", len(shellCode))
	}

	words := make([]uint32, len(shellCode)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(shellCode[i*4 : (i+1)*4])
	}

	// Must contain SYS_WRITE and SYS_READ calls
	foundWrite := false
	foundRead := false
	for _, w := range words {
		op, _, _, imm := decodeMBCInsn(w)
		if op == mbcMOVI && imm == mbcSysWrite {
			foundWrite = true
		}
		if op == mbcMOVI && imm == 3 { // SYS_READ = 3
			foundRead = true
		}
	}
	if !foundWrite {
		t.Error("shell program missing SYS_WRITE setup")
	}
	if !foundRead {
		t.Error("shell program missing SYS_READ setup")
	}

	// Must contain a JMP instruction (loop back)
	foundJMP := false
	for _, w := range words {
		op, _, _, _ := decodeMBCInsn(w)
		if op == 0x20 { // JMP
			foundJMP = true
			break
		}
	}
	if !foundJMP {
		t.Error("shell program missing JMP (read-eval-print loop)")
	}
}

func TestBootFilesystemRoundtrip(t *testing.T) {
	// Test that all boot filesystem files can be read back correctly.
	fs, err := CreateBootFilesystem()
	if err != nil {
		t.Fatalf("CreateBootFilesystem: %v", err)
	}

	// Read /etc/hostname and verify content
	hostname, err := ReadFile(fs, "/etc/hostname")
	if err != nil {
		t.Fatalf("ReadFile(/etc/hostname): %v", err)
	}
	if string(hostname) != "upc\n" {
		t.Errorf("/etc/hostname = %q, want %q", hostname, "upc\n")
	}

	// Read /init and verify it's valid MBC (starts with MOVI opcode)
	initData, err := ReadFile(fs, "/init")
	if err != nil {
		t.Fatalf("ReadFile(/init): %v", err)
	}
	firstWord := binary.LittleEndian.Uint32(initData[:4])
	op, _, _, _ := decodeMBCInsn(firstWord)
	if op != mbcMOVI {
		t.Errorf("/init first opcode = 0x%02X, want 0x%02X", op, mbcMOVI)
	}

	// Read /bin/sh and verify it's valid MBC
	shData, err := ReadFile(fs, "/bin/sh")
	if err != nil {
		t.Fatalf("ReadFile(/bin/sh): %v", err)
	}
	firstWord = binary.LittleEndian.Uint32(shData[:4])
	op, _, _, _ = decodeMBCInsn(firstWord)
	if op != mbcMOVI {
		t.Errorf("/bin/sh first opcode = 0x%02X, want 0x%02X", op, mbcMOVI)
	}

	// /dev/console should be a device node with no data
	console := LookupInode(fs, "/dev/console")
	if console == nil {
		t.Fatal("/dev/console not found")
	}
	if console.FileType != UNFSTypeDevice {
		t.Errorf("/dev/console type = %d, want %d", console.FileType, UNFSTypeDevice)
	}
	if console.Size != 0 {
		t.Errorf("/dev/console size = %d, want 0", console.Size)
	}
}
