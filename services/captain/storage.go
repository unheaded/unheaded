package captain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileStorage persists decisions to disk with defensive validation
type FileStorage struct {
	basePath string
	mu       sync.RWMutex
	cache    map[string]*Decision
}

// NewFileStorage creates a new file-based storage
func NewFileStorage(basePath string) (*FileStorage, error) {
	if basePath == "" {
		// Default: use CAPTAIN_DATA_DIR env, fall back to /var/lib/unheaded/captain
		basePath = os.Getenv("CAPTAIN_DATA_DIR")
		if basePath == "" {
			basePath = "/var/lib/unheaded/captain"
		}
	}

	// Validate path
	if len(basePath) > 1024 {
		return nil, errors.New("base path too long")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	fs := &FileStorage{
		basePath: basePath,
		cache:    make(map[string]*Decision),
	}

	// Load existing decisions
	if err := fs.loadAll(context.Background()); err != nil {
		// Log but don't fail on load error
		return fs, nil
	}

	return fs, nil
}

// SaveDecision persists a decision to disk
func (fs *FileStorage) SaveDecision(ctx context.Context, d *Decision) error {
	if d == nil {
		return errors.New("decision cannot be nil")
	}
	if d.ID == "" {
		return errors.New("decision must have an ID")
	}

	// Validate decision
	if err := d.Validate(); err != nil {
		return fmt.Errorf("invalid decision: %w", err)
	}

	// Construct file path with safety checks
	filePath, err := fs.safeFilePath(d.ID)
	if err != nil {
		return err
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal decision: %w", err)
	}

	// Validate JSON size
	if len(data) > 1024*1024 {
		return errors.New("decision too large (>1MB)")
	}

	// Write to temporary file first (atomic operation)
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("rename file: %w", err)
	}

	// Update cache
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.cache[d.ID] = d

	return nil
}

// GetDecision retrieves a decision by ID
func (fs *FileStorage) GetDecision(ctx context.Context, id string) (*Decision, error) {
	if id == "" {
		return nil, errors.New("decision ID cannot be empty")
	}

	// Check cache first
	fs.mu.RLock()
	cached, exists := fs.cache[id]
	fs.mu.RUnlock()

	if exists {
		return cached, nil
	}

	// Load from disk
	filePath, err := fs.safeFilePath(id)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("decision not found")
		}
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Validate size
	if len(data) > 1024*1024 {
		return nil, errors.New("decision file too large")
	}

	var decision Decision
	if err := json.Unmarshal(data, &decision); err != nil {
		return nil, fmt.Errorf("unmarshal decision: %w", err)
	}

	// Validate deserialized decision
	if err := decision.Validate(); err != nil {
		return nil, fmt.Errorf("invalid decision data: %w", err)
	}

	// Update cache
	fs.mu.Lock()
	fs.cache[decision.ID] = &decision
	fs.mu.Unlock()

	return &decision, nil
}

// ListDecisions retrieves decisions with pagination
func (fs *FileStorage) ListDecisions(ctx context.Context, limit, offset int) ([]*Decision, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var decisions []*Decision
	for _, d := range fs.cache {
		decisions = append(decisions, d)
	}

	// Handle bounds
	if offset >= len(decisions) {
		return []*Decision{}, nil
	}

	end := offset + limit
	if end > len(decisions) {
		end = len(decisions)
	}

	return decisions[offset:end], nil
}

// DeleteDecision removes a decision
func (fs *FileStorage) DeleteDecision(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("decision ID cannot be empty")
	}

	filePath, err := fs.safeFilePath(id)
	if err != nil {
		return err
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}

	// Remove from cache
	fs.mu.Lock()
	defer fs.mu.Unlock()
	delete(fs.cache, id)

	return nil
}

// loadAll loads all decisions from disk
func (fs *FileStorage) loadAll(ctx context.Context) error {
	entries, err := os.ReadDir(fs.basePath)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process JSON files
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(fs.basePath, entry.Name())

		data, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip on error
		}

		var decision Decision
		if err := json.Unmarshal(data, &decision); err != nil {
			continue // Skip invalid files
		}

		fs.cache[decision.ID] = &decision
	}

	return nil
}

// safeFilePath constructs a safe file path with validation
func (fs *FileStorage) safeFilePath(id string) (string, error) {
	// Validate ID
	if len(id) == 0 || len(id) > 200 {
		return "", errors.New("invalid ID length")
	}

	// Check for path traversal
	if id == "." || id == ".." || id == "/" {
		return "", errors.New("invalid ID")
	}

	// Only allow alphanumeric, underscore, and hyphen
	for _, r := range id {
		if !isValidIDChar(r) {
			return "", fmt.Errorf("invalid character in ID: %c", r)
		}
	}

	path := filepath.Join(fs.basePath, id+".json")

	// Ensure path is within basePath (prevent directory traversal)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	baseAbs, err := filepath.Abs(fs.basePath)
	if err != nil {
		return "", fmt.Errorf("resolve base path: %w", err)
	}

	// Check if path is within basePath
	if !isPathWithin(baseAbs, abs) {
		return "", errors.New("path traversal detected")
	}

	return abs, nil
}

// isValidIDChar checks if a character is valid in an ID
func isValidIDChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '-'
}

// isPathWithin checks if path is within basePath
func isPathWithin(basePath, path string) bool {
	rel, err := filepath.Rel(basePath, path)
	if err != nil {
		return false
	}
	// Check if relative path starts with ..
	return !pathGoesUp(rel)
}

// pathGoesUp checks if path goes up directory
func pathGoesUp(path string) bool {
	return len(path) > 0 && path[0:1] == ".."
}

// ============================================================================
// MEMORY STORAGE (for testing)
// ============================================================================

// MemoryStorage is an in-memory implementation for testing
type MemoryStorage struct {
	decisions map[string]*Decision
	mu        sync.RWMutex
}

// NewMemoryStorage creates a new memory-based storage
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		decisions: make(map[string]*Decision),
	}
}

// SaveDecision persists a decision in memory
func (ms *MemoryStorage) SaveDecision(ctx context.Context, d *Decision) error {
	if d == nil {
		return errors.New("decision cannot be nil")
	}
	if d.ID == "" {
		return errors.New("decision must have an ID")
	}

	if err := d.Validate(); err != nil {
		return fmt.Errorf("invalid decision: %w", err)
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.decisions[d.ID] = d

	return nil
}

// GetDecision retrieves a decision from memory
func (ms *MemoryStorage) GetDecision(ctx context.Context, id string) (*Decision, error) {
	if id == "" {
		return nil, errors.New("decision ID cannot be empty")
	}

	ms.mu.RLock()
	defer ms.mu.RUnlock()

	d, ok := ms.decisions[id]
	if !ok {
		return nil, errors.New("decision not found")
	}

	return d, nil
}

// ListDecisions retrieves decisions from memory
func (ms *MemoryStorage) ListDecisions(ctx context.Context, limit, offset int) ([]*Decision, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var decisions []*Decision
	for _, d := range ms.decisions {
		decisions = append(decisions, d)
	}

	if offset >= len(decisions) {
		return []*Decision{}, nil
	}

	end := offset + limit
	if end > len(decisions) {
		end = len(decisions)
	}

	return decisions[offset:end], nil
}

// DeleteDecision removes a decision from memory
func (ms *MemoryStorage) DeleteDecision(ctx context.Context, id string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.decisions, id)
	return nil
}
