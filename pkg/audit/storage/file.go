// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"unheaded/pkg/audit"
)

// FileStorage implements file-based audit storage with rotation.
type FileStorage struct {
	mu          sync.RWMutex
	baseDir     string
	currentFile *os.File
	writer      *bufio.Writer
	lastHash    string

	// Rotation settings
	maxFileSize   int64
	rotateDaily   bool
	currentDate   string
	currentSize   int64
	retentionDays int
}

// FileStorageConfig holds configuration for file storage.
type FileStorageConfig struct {
	BaseDir       string `json:"base_dir"`
	MaxFileSize   int64  `json:"max_file_size"`
	RotateDaily   bool   `json:"rotate_daily"`
	RetentionDays int    `json:"retention_days"`
}

// NewFileStorage creates a new file-based storage.
func NewFileStorage(config *FileStorageConfig) (*FileStorage, error) {
	if config.BaseDir == "" {
		return nil, fmt.Errorf("base directory is required")
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(config.BaseDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	if config.MaxFileSize <= 0 {
		config.MaxFileSize = 100 * 1024 * 1024 // 100MB default
	}
	if config.RetentionDays <= 0 {
		config.RetentionDays = 90
	}

	fs := &FileStorage{
		baseDir:       config.BaseDir,
		maxFileSize:   config.MaxFileSize,
		rotateDaily:   config.RotateDaily,
		retentionDays: config.RetentionDays,
	}

	// Load last hash from existing files
	if err := fs.loadLastHash(); err != nil {
		// Ignore error, start fresh
		fs.lastHash = ""
	}

	// Open current log file
	if err := fs.openCurrentFile(); err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return fs, nil
}

// Store persists an audit event.
func (fs *FileStorage) Store(ctx context.Context, event *audit.AuditEvent) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Check if rotation is needed
	if err := fs.checkRotation(); err != nil {
		return fmt.Errorf("rotation check failed: %w", err)
	}

	// Serialize event
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Write with newline
	n, err := fs.writer.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}
	_, err = fs.writer.WriteString("\n")
	if err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	fs.currentSize += int64(n + 1)
	fs.lastHash = event.Hash

	return fs.writer.Flush()
}

// StoreBatch persists multiple events.
func (fs *FileStorage) StoreBatch(ctx context.Context, events []*audit.AuditEvent) error {
	for _, event := range events {
		if err := fs.Store(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// Get retrieves a single event by ID.
func (fs *FileStorage) Get(ctx context.Context, id string) (*audit.AuditEvent, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	files, err := fs.listLogFiles()
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		event, err := fs.findEventInFile(file, id)
		if err == nil && event != nil {
			return event, nil
		}
	}

	return nil, fmt.Errorf("event not found: %s", id)
}

// Query retrieves events matching the query.
func (fs *FileStorage) Query(ctx context.Context, query *audit.AuditQuery) ([]*audit.AuditEvent, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	files, err := fs.listLogFiles()
	if err != nil {
		return nil, err
	}

	var results []*audit.AuditEvent

	for _, file := range files {
		events, err := fs.queryFile(file, query)
		if err != nil {
			continue // Skip problematic files
		}
		results = append(results, events...)
	}

	// Sort by timestamp
	sort.Slice(results, func(i, j int) bool {
		if query.Descending {
			return results[i].Timestamp.After(results[j].Timestamp)
		}
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	// Apply pagination
	if query.Offset > 0 && query.Offset < len(results) {
		results = results[query.Offset:]
	}
	if query.Limit > 0 && query.Limit < len(results) {
		results = results[:query.Limit]
	}

	return results, nil
}

// GetLastHash returns the hash of the most recent event.
func (fs *FileStorage) GetLastHash(ctx context.Context) (string, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.lastHash, nil
}

// Count returns the number of events matching the query.
func (fs *FileStorage) Count(ctx context.Context, query *audit.AuditQuery) (int64, error) {
	events, err := fs.Query(ctx, query)
	if err != nil {
		return 0, err
	}
	return int64(len(events)), nil
}

// Close releases resources.
func (fs *FileStorage) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.writer != nil {
		_ = fs.writer.Flush()
	}
	if fs.currentFile != nil {
		return fs.currentFile.Close()
	}
	return nil
}

// Rotate forces a log rotation.
func (fs *FileStorage) Rotate() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.rotate()
}

// openCurrentFile opens the current log file.
func (fs *FileStorage) openCurrentFile() error {
	fs.currentDate = time.Now().UTC().Format("2006-01-02")
	filename := filepath.Join(fs.baseDir, fmt.Sprintf("audit-%s.log", fs.currentDate))

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}

	// Get current file size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}

	fs.currentFile = file
	fs.writer = bufio.NewWriter(file)
	fs.currentSize = info.Size()

	return nil
}

// checkRotation checks if rotation is needed and performs it.
func (fs *FileStorage) checkRotation() error {
	needRotation := false

	// Check size-based rotation
	if fs.currentSize >= fs.maxFileSize {
		needRotation = true
	}

	// Check daily rotation
	if fs.rotateDaily {
		today := time.Now().UTC().Format("2006-01-02")
		if today != fs.currentDate {
			needRotation = true
		}
	}

	if needRotation {
		return fs.rotate()
	}

	return nil
}

// rotate performs log rotation.
func (fs *FileStorage) rotate() error {
	if fs.writer != nil {
		_ = fs.writer.Flush()
	}
	if fs.currentFile != nil {
		fs.currentFile.Close()
	}

	// Rename current file with timestamp if size-based rotation
	if fs.currentSize >= fs.maxFileSize {
		oldName := filepath.Join(fs.baseDir, fmt.Sprintf("audit-%s.log", fs.currentDate))
		newName := filepath.Join(fs.baseDir, fmt.Sprintf("audit-%s-%d.log", fs.currentDate, time.Now().Unix()))
		os.Rename(oldName, newName)
	}

	return fs.openCurrentFile()
}

// loadLastHash loads the last hash from existing files.
func (fs *FileStorage) loadLastHash() error {
	files, err := fs.listLogFiles()
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return nil
	}

	// Read last line of most recent file
	lastFile := files[len(files)-1]
	file, err := os.Open(lastFile)
	if err != nil {
		return err
	}
	defer file.Close()

	var lastLine string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lastLine = scanner.Text()
	}

	if lastLine == "" {
		return nil
	}

	var event audit.AuditEvent
	if err := json.Unmarshal([]byte(lastLine), &event); err != nil {
		return err
	}

	fs.lastHash = event.Hash
	return nil
}

// listLogFiles returns sorted list of log files.
func (fs *FileStorage) listLogFiles() ([]string, error) {
	pattern := filepath.Join(fs.baseDir, "audit-*.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// findEventInFile searches for an event by ID in a file.
func (fs *FileStorage) findEventInFile(filename, id string) (*audit.AuditEvent, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max line size

	for scanner.Scan() {
		var event audit.AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.ID == id {
			return &event, nil
		}
	}

	return nil, nil
}

// queryFile queries events from a single file.
func (fs *FileStorage) queryFile(filename string, query *audit.AuditQuery) ([]*audit.AuditEvent, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var results []*audit.AuditEvent
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		var event audit.AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if matchesQuery(&event, query) {
			results = append(results, &event)
		}
	}

	return results, scanner.Err()
}

// matchesQuery checks if an event matches query criteria.
func matchesQuery(event *audit.AuditEvent, query *audit.AuditQuery) bool {
	if !query.StartTime.IsZero() && event.Timestamp.Before(query.StartTime) {
		return false
	}
	if !query.EndTime.IsZero() && event.Timestamp.After(query.EndTime) {
		return false
	}

	if len(query.Types) > 0 {
		found := false
		for _, t := range query.Types {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(query.ActorIDs) > 0 && event.Actor != nil {
		found := false
		for _, id := range query.ActorIDs {
			if event.Actor.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// NewFileStorageFromConfig creates file storage from generic config map.
func NewFileStorageFromConfig(config map[string]interface{}) (audit.Storage, error) {
	cfg := &FileStorageConfig{}

	if v, ok := config["base_dir"].(string); ok {
		cfg.BaseDir = v
	}
	if v, ok := config["max_file_size"].(float64); ok {
		cfg.MaxFileSize = int64(v)
	}
	if v, ok := config["rotate_daily"].(bool); ok {
		cfg.RotateDaily = v
	}
	if v, ok := config["retention_days"].(float64); ok {
		cfg.RetentionDays = int(v)
	}

	return NewFileStorage(cfg)
}

func init() {
	audit.DefaultRegistry.RegisterStorage("file", NewFileStorageFromConfig)
}

// FileReader provides streaming read access to audit files.
type FileReader struct {
	files   []string
	current int
	file    *os.File
	scanner *bufio.Scanner
}

// NewFileReader creates a reader for audit files.
func NewFileReader(baseDir string) (*FileReader, error) {
	pattern := filepath.Join(baseDir, "audit-*.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	return &FileReader{
		files:   files,
		current: -1,
	}, nil
}

// Next returns the next event from the files.
func (r *FileReader) Next() (*audit.AuditEvent, error) {
	for {
		if r.scanner == nil {
			if err := r.openNextFile(); err != nil {
				return nil, err
			}
		}

		if r.scanner.Scan() {
			var event audit.AuditEvent
			if err := json.Unmarshal(r.scanner.Bytes(), &event); err != nil {
				continue // Skip malformed lines
			}
			return &event, nil
		}

		// End of current file, try next
		r.closeCurrentFile()
		r.scanner = nil
	}
}

func (r *FileReader) openNextFile() error {
	r.current++
	if r.current >= len(r.files) {
		return io.EOF
	}

	file, err := os.Open(r.files[r.current])
	if err != nil {
		return err
	}

	r.file = file
	r.scanner = bufio.NewScanner(file)
	r.scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	return nil
}

func (r *FileReader) closeCurrentFile() {
	if r.file != nil {
		r.file.Close()
		r.file = nil
	}
}

// Close releases resources.
func (r *FileReader) Close() error {
	r.closeCurrentFile()
	return nil
}
