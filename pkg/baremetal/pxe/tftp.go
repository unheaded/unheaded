package pxe

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TFTPServer handles TFTP requests for PXE boot files.
type TFTPServer struct {
	address string
	root    string

	running bool
	done    chan struct{}
	mu      sync.RWMutex

	// File cache for frequently accessed files
	cache     map[string]*cachedFile
	cacheMu   sync.RWMutex
	cacheSize int64
	maxCache  int64

	// Stats
	stats *TFTPStats
}

// TFTPStats holds TFTP server statistics.
type TFTPStats struct {
	mu             sync.RWMutex
	TotalRequests  int64     `json:"total_requests"`
	BytesSent      int64     `json:"bytes_sent"`
	Errors         int64     `json:"errors"`
	CacheHits      int64     `json:"cache_hits"`
	CacheMisses    int64     `json:"cache_misses"`
	LastRequest    time.Time `json:"last_request"`
	ActiveTransfers int       `json:"active_transfers"`
}

type cachedFile struct {
	data      []byte
	size      int64
	modTime   time.Time
	lastAccess time.Time
	hits      int64
}

// NewTFTPServer creates a new TFTP server.
func NewTFTPServer(address, root string) (*TFTPServer, error) {
	// Ensure root directory exists
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("creating TFTP root: %w", err)
	}

	// Create pxelinux.cfg directory
	if err := os.MkdirAll(filepath.Join(root, "pxelinux.cfg"), 0755); err != nil {
		return nil, fmt.Errorf("creating pxelinux.cfg: %w", err)
	}

	return &TFTPServer{
		address:  address,
		root:     root,
		done:     make(chan struct{}),
		cache:    make(map[string]*cachedFile),
		maxCache: 256 * 1024 * 1024, // 256MB cache
		stats:    &TFTPStats{},
	}, nil
}

// Start starts the TFTP server.
func (s *TFTPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	// In production, this would start the actual TFTP server
	// using a library like pin/tftp or a custom implementation
	s.running = true

	// Start cache cleanup goroutine
	go s.cacheCleanup()

	return nil
}

// Stop stops the TFTP server.
func (s *TFTPServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	close(s.done)
	s.running = false

	return nil
}

// WriteFile writes a file to the TFTP root.
func (s *TFTPServer) WriteFile(path string, data []byte) error {
	fullPath := filepath.Join(s.root, path)

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	// Invalidate cache
	s.cacheMu.Lock()
	delete(s.cache, path)
	s.cacheMu.Unlock()

	return nil
}

// RemoveFile removes a file from the TFTP root.
func (s *TFTPServer) RemoveFile(path string) error {
	fullPath := filepath.Join(s.root, path)

	// Remove file
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing file: %w", err)
	}

	// Invalidate cache
	s.cacheMu.Lock()
	delete(s.cache, path)
	s.cacheMu.Unlock()

	return nil
}

// ReadFile reads a file from the TFTP root.
func (s *TFTPServer) ReadFile(path string) ([]byte, error) {
	// Check cache first
	s.cacheMu.RLock()
	cached, exists := s.cache[path]
	if exists {
		cached.lastAccess = time.Now()
		cached.hits++
		data := cached.data
		s.cacheMu.RUnlock()

		s.stats.mu.Lock()
		s.stats.CacheHits++
		s.stats.mu.Unlock()

		return data, nil
	}
	s.cacheMu.RUnlock()

	s.stats.mu.Lock()
	s.stats.CacheMisses++
	s.stats.mu.Unlock()

	// Read from disk
	fullPath := filepath.Join(s.root, path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}

	// Add to cache if small enough
	if int64(len(data)) < s.maxCache/10 {
		s.addToCache(path, data)
	}

	return data, nil
}

// addToCache adds a file to the cache.
func (s *TFTPServer) addToCache(path string, data []byte) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	size := int64(len(data))

	// Evict if necessary
	for s.cacheSize+size > s.maxCache && len(s.cache) > 0 {
		s.evictOne()
	}

	s.cache[path] = &cachedFile{
		data:       data,
		size:       size,
		modTime:    time.Now(),
		lastAccess: time.Now(),
	}
	s.cacheSize += size
}

// evictOne evicts the least recently used cache entry.
func (s *TFTPServer) evictOne() {
	var oldestPath string
	var oldestTime time.Time

	for path, file := range s.cache {
		if oldestPath == "" || file.lastAccess.Before(oldestTime) {
			oldestPath = path
			oldestTime = file.lastAccess
		}
	}

	if oldestPath != "" {
		s.cacheSize -= s.cache[oldestPath].size
		delete(s.cache, oldestPath)
	}
}

// cacheCleanup periodically cleans up old cache entries.
func (s *TFTPServer) cacheCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.cleanupCache()
		}
	}
}

// cleanupCache removes old cache entries.
func (s *TFTPServer) cleanupCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	threshold := time.Now().Add(-30 * time.Minute)

	for path, file := range s.cache {
		if file.lastAccess.Before(threshold) {
			s.cacheSize -= file.size
			delete(s.cache, path)
		}
	}
}

// HandleRead handles a TFTP read request.
func (s *TFTPServer) HandleRead(path string, w io.Writer) error {
	s.stats.mu.Lock()
	s.stats.TotalRequests++
	s.stats.ActiveTransfers++
	s.stats.LastRequest = time.Now()
	s.stats.mu.Unlock()

	defer func() {
		s.stats.mu.Lock()
		s.stats.ActiveTransfers--
		s.stats.mu.Unlock()
	}()

	data, err := s.ReadFile(path)
	if err != nil {
		s.stats.mu.Lock()
		s.stats.Errors++
		s.stats.mu.Unlock()
		return err
	}

	n, err := w.Write(data)
	if err != nil {
		s.stats.mu.Lock()
		s.stats.Errors++
		s.stats.mu.Unlock()
		return err
	}

	s.stats.mu.Lock()
	s.stats.BytesSent += int64(n)
	s.stats.mu.Unlock()

	return nil
}

// GetStats returns TFTP server statistics.
func (s *TFTPServer) GetStats() *TFTPStats {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()

	return &TFTPStats{
		TotalRequests:   s.stats.TotalRequests,
		BytesSent:       s.stats.BytesSent,
		Errors:          s.stats.Errors,
		CacheHits:       s.stats.CacheHits,
		CacheMisses:     s.stats.CacheMisses,
		LastRequest:     s.stats.LastRequest,
		ActiveTransfers: s.stats.ActiveTransfers,
	}
}
