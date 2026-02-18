// Package compute implements the Wotan Memory Service for the Doom-over-IPv6
// compute architecture. It manages the L2 byte-addressable store and
// orchestrates data flow between the BPF L1 cache (RAM_MAP) and persistent
// storage.
//
// Memory hierarchy:
//
//	L0: Monad Scratch (4 bytes, wire speed, in-packet)
//	L1: BPF hash map cache (RAM_MAP, ~100-200ns, per-hop)
//	L2: Wotan ring buffer (this package, ~1-10us, Go heap)
//	L3: WAL (optional, ~100us-1ms, disk)
//	L4: Sophia dictionaries (not implemented yet)
//
// The service subscribes to compute.mem.miss and compute.mem.write events
// from the BPF layer (via Wotan topic pub/sub) and keeps L1 primed.
package compute

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Default configuration values.
const (
	DefaultRingSize  = 4 * 1024 * 1024 // 4 MiB (covers 48 KiB RAM + 4 MiB WAD)
	DefaultPageSize  = 4096            // 4 KiB pages (1024 words)
	DefaultPrefetchN = 2              // prefetch 2 adjacent pages on miss
	DefaultL1MaxPages = 256           // max pages in L1 before eviction
)

// Config holds memory service configuration.
type Config struct {
	RingSize   int    `yaml:"ring_size"`   // L2 store size in bytes
	PageSize   int    `yaml:"page_size"`   // page size in bytes (must be multiple of 4)
	PrefetchN  int    `yaml:"prefetch_n"`  // pages to prefetch around access
	L1MaxPages int    `yaml:"l1_max_pages"` // max pages cached in L1 before LRU eviction
	WALEnabled bool   `yaml:"wal_enabled"` // persist writes to disk
	WALPath    string `yaml:"wal_path"`    // WAL file location
}

// DefaultConfig returns sensible defaults for the memory service.
func DefaultConfig() Config {
	return Config{
		RingSize:   DefaultRingSize,
		PageSize:   DefaultPageSize,
		PrefetchN:  DefaultPrefetchN,
		L1MaxPages: DefaultL1MaxPages,
	}
}

// CacheMissEvent is emitted by the BPF layer when a memory read misses L1.
type CacheMissEvent struct {
	FlowLabel uint32 `json:"flow_label"`
	HopID     uint8  `json:"hop_id"`
	Address   uint32 `json:"address"`   // byte address
	Timestamp int64  `json:"timestamp"` // nanoseconds since boot
}

// CacheWriteEvent is emitted by the BPF layer after a memory write to L1.
type CacheWriteEvent struct {
	FlowLabel uint32 `json:"flow_label"`
	HopID     uint8  `json:"hop_id"`
	Address   uint32 `json:"address"` // byte address
	Value     uint32 `json:"value"`   // written value
	Width     uint8  `json:"width"`   // 1=byte, 2=half, 4=word
	Timestamp int64  `json:"timestamp"`
}

// pageEntry tracks a page cached in a hop's L1.
type pageEntry struct {
	PageAddr   uint32    // page-aligned byte address
	HopID      uint8     // which hop's L1
	Dirty      bool      // modified since loaded from L2
	LastAccess time.Time // for LRU eviction
}

// Stats holds runtime statistics for the memory service.
type Stats struct {
	MissCount    uint64 `json:"miss_count"`
	WriteCount   uint64 `json:"write_count"`
	EvictCount   uint64 `json:"evict_count"`
	PrefetchHits uint64 `json:"prefetch_hits"`
	L2Size       int    `json:"l2_size"`
	L1Pages      int    `json:"l1_pages"`
}

// MemoryService manages the L2 byte-addressable store and L1 BPF cache.
type MemoryService struct {
	config Config

	// L2 store: flat byte-addressable memory.
	mu   sync.RWMutex
	data []byte

	// L1 page table: tracks which pages are in which hop's L1 cache.
	pageTable map[uint32]*pageEntry // key = page-aligned byte address

	// LRU order: most recently accessed at end.
	lruOrder []*pageEntry

	// BPF map interface for reading/writing L1 cache.
	bpfCache BPFMapCache

	// Runtime stats.
	stats Stats
}

// NewMemoryService creates a new memory service with the given config and
// BPF map cache. The L2 store is zero-initialized.
func NewMemoryService(cfg Config, bpfCache BPFMapCache) *MemoryService {
	if cfg.RingSize <= 0 {
		cfg.RingSize = DefaultRingSize
	}
	if cfg.PageSize <= 0 || cfg.PageSize%4 != 0 {
		cfg.PageSize = DefaultPageSize
	}
	if cfg.PrefetchN < 0 {
		cfg.PrefetchN = DefaultPrefetchN
	}
	if cfg.L1MaxPages <= 0 {
		cfg.L1MaxPages = DefaultL1MaxPages
	}

	return &MemoryService{
		config:    cfg,
		data:      make([]byte, cfg.RingSize),
		pageTable: make(map[uint32]*pageEntry),
		lruOrder:  make([]*pageEntry, 0, cfg.L1MaxPages),
		bpfCache:  bpfCache,
	}
}

// HandleCacheMiss processes a cache miss event: loads the requested page
// from L2 into the target hop's L1 BPF map, plus prefetch pages.
func (ms *MemoryService) HandleCacheMiss(event CacheMissEvent) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ms.stats.MissCount++

	pageAddr := ms.alignToPage(event.Address)

	// Load requested page + prefetch pages.
	for i := 0; i <= ms.config.PrefetchN; i++ {
		addr := pageAddr + uint32(i*ms.config.PageSize)
		if int(addr)+ms.config.PageSize > ms.config.RingSize {
			break // don't read past end of L2
		}

		if err := ms.loadPageToL1(addr, event.HopID); err != nil {
			log.Error().
				Err(err).
				Uint32("address", addr).
				Uint8("hop_id", event.HopID).
				Msg("failed to load page to L1")
			if i == 0 {
				return err // fail only if the primary page fails
			}
		}
	}

	return nil
}

// HandleCacheWrite processes a write event: updates L2 with the written data
// and marks the page dirty for WAL flush.
func (ms *MemoryService) HandleCacheWrite(event CacheWriteEvent) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ms.stats.WriteCount++

	addr := event.Address
	if int(addr) >= ms.config.RingSize {
		return nil // out of bounds, ignore
	}

	// Write to L2 store based on width.
	switch event.Width {
	case 1: // byte
		if int(addr) < len(ms.data) {
			ms.data[addr] = byte(event.Value)
		}
	case 2: // halfword
		if int(addr)+1 < len(ms.data) {
			binary.LittleEndian.PutUint16(ms.data[addr:], uint16(event.Value))
		}
	case 4: // word
		if int(addr)+3 < len(ms.data) {
			binary.LittleEndian.PutUint32(ms.data[addr:], event.Value)
		}
	}

	// Mark page dirty in page table.
	pageAddr := ms.alignToPage(addr)
	if entry, ok := ms.pageTable[pageAddr]; ok {
		entry.Dirty = true
		entry.LastAccess = time.Now()
		ms.touchLRU(entry)
	}

	return nil
}

// LoadData writes raw bytes into the L2 store at the given byte address.
// Used for initial data loading (e.g., WAD files, ROM images).
func (ms *MemoryService) LoadData(addr uint32, data []byte) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	end := int(addr) + len(data)
	if end > ms.config.RingSize {
		return &MemoryError{
			Op:      "load_data",
			Address: addr,
			Msg:     "data exceeds L2 store size",
		}
	}

	copy(ms.data[addr:end], data)
	return nil
}

// ReadData reads raw bytes from the L2 store (for testing/diagnostics).
func (ms *MemoryService) ReadData(addr uint32, length int) []byte {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	end := int(addr) + length
	if end > len(ms.data) {
		end = len(ms.data)
	}
	if int(addr) >= len(ms.data) {
		return nil
	}

	out := make([]byte, end-int(addr))
	copy(out, ms.data[addr:end])
	return out
}

// ReadWord reads a single 32-bit word from L2 at the given byte address.
func (ms *MemoryService) ReadWord(byteAddr uint32) uint32 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if int(byteAddr)+3 >= len(ms.data) {
		return 0
	}
	return binary.LittleEndian.Uint32(ms.data[byteAddr:])
}

// GetStats returns a snapshot of runtime statistics.
func (ms *MemoryService) GetStats() Stats {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	s := ms.stats
	s.L2Size = ms.config.RingSize
	s.L1Pages = len(ms.pageTable)
	return s
}

// StageAllToL1 loads every non-zero page from L2 into L1, up to L1MaxPages.
// Useful for pre-warming the cache before execution starts.
func (ms *MemoryService) StageAllToL1(hopID uint8) (int, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	staged := 0
	for addr := uint32(0); int(addr)+ms.config.PageSize <= ms.config.RingSize; addr += uint32(ms.config.PageSize) {
		if staged >= ms.config.L1MaxPages {
			break
		}
		// Check if page has any non-zero data.
		hasData := false
		for _, b := range ms.data[addr : int(addr)+ms.config.PageSize] {
			if b != 0 {
				hasData = true
				break
			}
		}
		if !hasData {
			continue
		}
		if err := ms.loadPageToL1(addr, hopID); err != nil {
			return staged, err
		}
		staged++
	}
	return staged, nil
}

// Run starts the memory service event loop. It subscribes to compute.mem.*
// events via the provided channels and processes them until ctx is cancelled.
func (ms *MemoryService) Run(ctx context.Context, missCh <-chan CacheMissEvent, writeCh <-chan CacheWriteEvent) {
	log.Info().
		Int("ring_size", ms.config.RingSize).
		Int("page_size", ms.config.PageSize).
		Int("prefetch_n", ms.config.PrefetchN).
		Int("l1_max_pages", ms.config.L1MaxPages).
		Msg("memory_service_started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("memory_service_stopped")
			return

		case event, ok := <-missCh:
			if !ok {
				return
			}
			if err := ms.HandleCacheMiss(event); err != nil {
				log.Error().Err(err).Uint32("addr", event.Address).Msg("cache_miss_handler_error")
			}

		case event, ok := <-writeCh:
			if !ok {
				return
			}
			if err := ms.HandleCacheWrite(event); err != nil {
				log.Error().Err(err).Uint32("addr", event.Address).Msg("cache_write_handler_error")
			}
		}
	}
}

// --- Internal methods ---

// alignToPage rounds a byte address down to the nearest page boundary.
func (ms *MemoryService) alignToPage(addr uint32) uint32 {
	return addr & ^uint32(ms.config.PageSize-1)
}

// loadPageToL1 reads a page from L2 and writes it into the BPF map cache.
// Caller must hold ms.mu.
func (ms *MemoryService) loadPageToL1(pageAddr uint32, hopID uint8) error {
	// Evict if L1 is full AND this page isn't already cached.
	_, alreadyCached := ms.pageTable[pageAddr]
	if !alreadyCached && len(ms.pageTable) >= ms.config.L1MaxPages {
		if err := ms.evictLRU(); err != nil {
			return err
		}
	}

	end := int(pageAddr) + ms.config.PageSize
	if end > len(ms.data) {
		end = len(ms.data)
	}

	pageBytes := ms.data[pageAddr:end]
	wordsPerPage := ms.config.PageSize / 4
	words := make([]uint32, wordsPerPage)
	for i := 0; i < wordsPerPage && i*4 < len(pageBytes); i++ {
		if i*4+4 <= len(pageBytes) {
			words[i] = binary.LittleEndian.Uint32(pageBytes[i*4:])
		}
	}

	startWordAddr := pageAddr >> 2
	if err := ms.bpfCache.WritePage(startWordAddr, words); err != nil {
		return err
	}

	// Update page table.
	now := time.Now()
	entry, exists := ms.pageTable[pageAddr]
	if exists {
		entry.HopID = hopID
		entry.LastAccess = now
		ms.touchLRU(entry)
	} else {
		entry = &pageEntry{
			PageAddr:   pageAddr,
			HopID:      hopID,
			LastAccess: now,
		}
		ms.pageTable[pageAddr] = entry
		ms.lruOrder = append(ms.lruOrder, entry)
	}

	return nil
}

// evictLRU removes the least recently used page from L1.
// Caller must hold ms.mu.
func (ms *MemoryService) evictLRU() error {
	if len(ms.lruOrder) == 0 {
		return nil
	}

	// Find LRU (first entry in lruOrder).
	victim := ms.lruOrder[0]
	ms.lruOrder = ms.lruOrder[1:]

	// If dirty, write back to L2 before eviction.
	if victim.Dirty {
		if err := ms.writeBackPage(victim); err != nil {
			log.Error().Err(err).Uint32("page", victim.PageAddr).Msg("writeback_failed_during_evict")
		}
	}

	// Delete page entries from BPF map.
	wordsPerPage := ms.config.PageSize / 4
	startWord := victim.PageAddr >> 2
	for i := 0; i < wordsPerPage; i++ {
		_ = ms.bpfCache.DeleteWord(startWord + uint32(i))
	}

	delete(ms.pageTable, victim.PageAddr)
	ms.stats.EvictCount++

	return nil
}

// writeBackPage reads the current page data from L1 (BPF map) back to L2.
// Caller must hold ms.mu.
func (ms *MemoryService) writeBackPage(entry *pageEntry) error {
	wordsPerPage := ms.config.PageSize / 4
	startWord := entry.PageAddr >> 2

	for i := 0; i < wordsPerPage; i++ {
		word, err := ms.bpfCache.ReadWord(startWord + uint32(i))
		if err != nil {
			return err
		}
		byteOff := int(entry.PageAddr) + i*4
		if byteOff+4 <= len(ms.data) {
			binary.LittleEndian.PutUint32(ms.data[byteOff:], word)
		}
	}

	entry.Dirty = false
	return nil
}

// touchLRU moves an entry to the end of the LRU list (most recently used).
// Caller must hold ms.mu.
func (ms *MemoryService) touchLRU(entry *pageEntry) {
	for i, e := range ms.lruOrder {
		if e == entry {
			ms.lruOrder = append(ms.lruOrder[:i], ms.lruOrder[i+1:]...)
			ms.lruOrder = append(ms.lruOrder, entry)
			return
		}
	}
	// Not found (shouldn't happen), append.
	ms.lruOrder = append(ms.lruOrder, entry)
}

// MemoryError is returned for memory service operations.
type MemoryError struct {
	Op      string
	Address uint32
	Msg     string
}

func (e *MemoryError) Error() string {
	return e.Op + ": " + e.Msg
}

// --- Event JSON helpers ---

// ParseCacheMissEvent parses a JSON-encoded cache miss event from a Wotan
// topic message payload.
func ParseCacheMissEvent(payload string) (CacheMissEvent, error) {
	var event CacheMissEvent
	err := json.Unmarshal([]byte(payload), &event)
	return event, err
}

// ParseCacheWriteEvent parses a JSON-encoded cache write event.
func ParseCacheWriteEvent(payload string) (CacheWriteEvent, error) {
	var event CacheWriteEvent
	err := json.Unmarshal([]byte(payload), &event)
	return event, err
}
