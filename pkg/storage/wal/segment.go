// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wal

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

// Segment represents a single WAL segment file.
type Segment struct {
	path       string
	file       *os.File
	writer     *bufio.Writer
	mu         sync.Mutex
	size       int64
	syncWrites bool
	closed     bool
	firstSeq   uint64
	lastSeq    uint64
}

// CreateSegment creates a new segment file.
func CreateSegment(path string, syncWrites bool) (*Segment, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("create segment: %w", err)
	}

	return &Segment{
		path:       path,
		file:       file,
		writer:     bufio.NewWriterSize(file, 64*1024), // 64KB buffer
		syncWrites: syncWrites,
	}, nil
}

// OpenSegment opens an existing segment file.
func OpenSegment(path string, syncWrites bool) (*Segment, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open segment: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat segment: %w", err)
	}

	segment := &Segment{
		path:       path,
		file:       file,
		writer:     bufio.NewWriterSize(file, 64*1024),
		size:       stat.Size(),
		syncWrites: syncWrites,
	}

	// Scan to find first and last sequence
	if err := segment.scanSequences(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("scan sequences: %w", err)
	}

	// Seek to end for appending
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("seek to end: %w", err)
	}

	return segment, nil
}

// scanSequences scans the segment to find first and last sequence numbers.
func (s *Segment) scanSequences() error {
	if s.size == 0 {
		return nil
	}

	// Seek to beginning
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReader(s.file)
	header := make([]byte, entryHeaderSize)
	first := true

	for {
		// Read header
		n, err := io.ReadFull(reader, header)
		if err == io.EOF || n == 0 {
			break
		}
		if err != nil {
			return err
		}

		seq := binary.BigEndian.Uint64(header[0:8])
		length := binary.BigEndian.Uint32(header[8:12])

		if first {
			s.firstSeq = seq
			first = false
		}
		s.lastSeq = seq

		// Skip data
		if _, err := reader.Discard(int(length)); err != nil {
			return err
		}
	}

	return nil
}

// Write writes an entry to the segment.
func (s *Segment) Write(seq uint64, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrCorruptedEntry
	}

	entry := encodeEntry(seq, data)

	n, err := s.writer.Write(entry)
	if err != nil {
		return fmt.Errorf("write entry: %w", err)
	}

	atomic.AddInt64(&s.size, int64(n))

	// Update sequence tracking
	if s.firstSeq == 0 {
		s.firstSeq = seq
	}
	s.lastSeq = seq

	if s.syncWrites {
		if err := s.writer.Flush(); err != nil {
			return fmt.Errorf("flush writer: %w", err)
		}
		if err := s.file.Sync(); err != nil {
			return fmt.Errorf("sync file: %w", err)
		}
	}

	return nil
}

// Read reads an entry by sequence number.
func (s *Segment) Read(seq uint64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrCorruptedEntry
	}

	// Flush any pending writes
	if err := s.writer.Flush(); err != nil {
		return nil, fmt.Errorf("flush writer: %w", err)
	}

	// Seek to beginning
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(s.file)
	header := make([]byte, entryHeaderSize)

	for {
		// Read header
		n, err := io.ReadFull(reader, header)
		if err == io.EOF || n == 0 {
			return nil, ErrSequenceNotFound
		}
		if err != nil {
			return nil, err
		}

		entrySeq := binary.BigEndian.Uint64(header[0:8])
		length := binary.BigEndian.Uint32(header[8:12])
		crc := binary.BigEndian.Uint32(header[12:16])

		if entrySeq == seq {
			// Found it - read data
			data := make([]byte, length)
			if _, err := io.ReadFull(reader, data); err != nil {
				return nil, err
			}

			// Verify CRC
			if crc32ChecksumIEEE(data) != crc {
				return nil, ErrCorruptedEntry
			}

			return data, nil
		}

		// Skip data
		if _, err := reader.Discard(int(length)); err != nil {
			return nil, err
		}
	}
}

// Scan iterates over entries in the segment.
func (s *Segment) Scan(startSeq uint64, fn func(seq uint64, data []byte) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrCorruptedEntry
	}

	// Flush any pending writes
	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("flush writer: %w", err)
	}

	// Seek to beginning
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReader(s.file)
	header := make([]byte, entryHeaderSize)

	for {
		// Read header
		n, err := io.ReadFull(reader, header)
		if err == io.EOF || n == 0 {
			return nil
		}
		if err != nil {
			return err
		}

		seq := binary.BigEndian.Uint64(header[0:8])
		length := binary.BigEndian.Uint32(header[8:12])
		crc := binary.BigEndian.Uint32(header[12:16])

		// Read data
		data := make([]byte, length)
		if _, err := io.ReadFull(reader, data); err != nil {
			return err
		}

		// Skip entries before startSeq
		if seq < startSeq {
			continue
		}

		// Verify CRC
		if crc32ChecksumIEEE(data) != crc {
			return ErrCorruptedEntry
		}

		if err := fn(seq, data); err != nil {
			return err
		}
	}
}

// Sync flushes and syncs the segment to disk.
func (s *Segment) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("flush writer: %w", err)
	}

	if err := s.file.Sync(); err != nil {
		return fmt.Errorf("sync file: %w", err)
	}

	return nil
}

// Close closes the segment.
func (s *Segment) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("flush writer: %w", err)
	}

	if err := s.file.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	return nil
}

// Path returns the segment file path.
func (s *Segment) Path() string {
	return s.path
}

// Size returns the current segment size.
func (s *Segment) Size() int64 {
	return atomic.LoadInt64(&s.size)
}

// FirstSequence returns the first sequence number in the segment.
func (s *Segment) FirstSequence() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.firstSeq
}

// LastSequence returns the last sequence number in the segment.
func (s *Segment) LastSequence() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSeq
}

// EntryCount returns the number of entries in the segment.
func (s *Segment) EntryCount() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, ErrCorruptedEntry
	}

	// Flush any pending writes
	if err := s.writer.Flush(); err != nil {
		return 0, fmt.Errorf("flush writer: %w", err)
	}

	// Seek to beginning
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	reader := bufio.NewReader(s.file)
	header := make([]byte, entryHeaderSize)
	count := 0

	for {
		n, err := io.ReadFull(reader, header)
		if err == io.EOF || n == 0 {
			break
		}
		if err != nil {
			return 0, err
		}

		length := binary.BigEndian.Uint32(header[8:12])
		count++

		// Skip data
		if _, err := reader.Discard(int(length)); err != nil {
			return 0, err
		}
	}

	return count, nil
}

// Truncate truncates the segment file.
func (s *Segment) Truncate(size int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrCorruptedEntry
	}

	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("flush writer: %w", err)
	}

	if err := s.file.Truncate(size); err != nil {
		return fmt.Errorf("truncate file: %w", err)
	}

	atomic.StoreInt64(&s.size, size)

	// Re-scan sequences after truncation
	return s.scanSequences()
}

// Repair attempts to repair a corrupted segment.
func (s *Segment) Repair() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrCorruptedEntry
	}

	// Flush any pending writes
	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("flush writer: %w", err)
	}

	// Seek to beginning
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReader(s.file)
	header := make([]byte, entryHeaderSize)
	validSize := int64(0)

	for {
		n, err := io.ReadFull(reader, header)
		if err == io.EOF || n == 0 {
			break
		}
		if err != nil || n < entryHeaderSize {
			// Corruption detected - truncate here
			break
		}

		length := binary.BigEndian.Uint32(header[8:12])
		crc := binary.BigEndian.Uint32(header[12:16])

		// Read data
		data := make([]byte, length)
		if _, err := io.ReadFull(reader, data); err != nil {
			break
		}

		// Verify CRC
		if crc32ChecksumIEEE(data) != crc {
			break
		}

		// Entry is valid
		validSize += int64(entryHeaderSize + len(data))
	}

	// Truncate to valid size
	if validSize < s.size {
		if err := s.file.Truncate(validSize); err != nil {
			return fmt.Errorf("truncate file: %w", err)
		}
		atomic.StoreInt64(&s.size, validSize)
	}

	// Re-scan sequences
	return s.scanSequences()
}

// crc32ChecksumIEEE calculates CRC32 checksum.
func crc32ChecksumIEEE(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// ReadAll reads all entries from the segment.
func (s *Segment) ReadAll() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrCorruptedEntry
	}

	// Flush any pending writes
	if err := s.writer.Flush(); err != nil {
		return nil, fmt.Errorf("flush writer: %w", err)
	}

	// Seek to beginning
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(s.file)
	header := make([]byte, entryHeaderSize)
	var entries []Entry

	for {
		n, err := io.ReadFull(reader, header)
		if err == io.EOF || n == 0 {
			break
		}
		if err != nil {
			return nil, err
		}

		seq := binary.BigEndian.Uint64(header[0:8])
		length := binary.BigEndian.Uint32(header[8:12])
		crc := binary.BigEndian.Uint32(header[12:16])

		// Read data
		data := make([]byte, length)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}

		// Verify CRC
		if crc32ChecksumIEEE(data) != crc {
			return nil, ErrCorruptedEntry
		}

		entries = append(entries, Entry{
			Sequence: seq,
			Data:     data,
		})
	}

	return entries, nil
}

// IsEmpty returns true if the segment has no entries.
func (s *Segment) IsEmpty() bool {
	return atomic.LoadInt64(&s.size) == 0
}

// IsFull returns true if the segment has reached the maximum size.
func (s *Segment) IsFull(maxSize int64) bool {
	return atomic.LoadInt64(&s.size) >= maxSize
}
