// SPDX-License-Identifier: GPL-3.0-or-later
//
// cmd/test_batch — scratch tool for batch-reading the Doom screen BPF map.
// Marshal note 2026-05-07: this file's production-vs-scratch status is
// undetermined per docs/compliance/spdx-coverage-audit-2026-05-06.md
// recommendation #2; SPDX added unconditionally so the audit gap closes
// regardless of the eventual disposition.

package main

import (
	"encoding/binary"
	"fmt"
	"os"

	bpf "unheaded/internal/bpf"
)

func main() {
	screenMap, err := bpf.OpenMap("/sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP")
	if err != nil {
		fmt.Fprintf(os.Stderr, "OpenMap: %v\n", err)
		os.Exit(1)
	}
	defer screenMap.Close()

	// Batch read
	_, values, count, err := screenMap.LookupBatch(16000, 4, 1)
	if err != nil {
		fmt.Printf("Batch error: %v\n", err)
	}
	fmt.Printf("Batch returned %d entries\n", count)

	batchNonZero := 0
	for i := uint32(0); i < count; i++ {
		if values[i] != 0 {
			batchNonZero++
		}
	}
	fmt.Printf("Batch non-zero: %d/%d\n", batchNonZero, count)

	// Individual read for comparison (sample 200 entries)
	individualNonZero := 0
	keyBuf := make([]byte, 4)
	for i := uint32(0); i < 200; i++ {
		idx := i * 320 // first pixel of each row
		binary.LittleEndian.PutUint32(keyBuf, idx)
		val, err := screenMap.LookupElem(keyBuf, 1)
		if err != nil {
			continue
		}
		if val[0] != 0 {
			individualNonZero++
		}
	}
	fmt.Printf("Individual non-zero (1st pixel of each row): %d/200\n", individualNonZero)

	// Sample row 100 individual
	fmt.Printf("Row 100 (individual read, first 20): ")
	for col := 0; col < 20; col++ {
		idx := uint32(100*320 + col)
		binary.LittleEndian.PutUint32(keyBuf, idx)
		val, _ := screenMap.LookupElem(keyBuf, 1)
		if val != nil {
			fmt.Printf("%02X ", val[0])
		} else {
			fmt.Printf("?? ")
		}
	}
	fmt.Println()

	// Sample row 100 from batch
	fmt.Printf("Row 100 (batch read, first 20): ")
	for col := 0; col < 20; col++ {
		idx := 100*320 + col
		if idx < int(count) {
			fmt.Printf("%02X ", values[idx])
		} else {
			fmt.Printf("-- ")
		}
	}
	fmt.Println()
}
