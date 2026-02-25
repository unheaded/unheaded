// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "unheaded/proto/unheaded/v1"
)

// sophiaServer implements the Sophia service for dictionary management
type sophiaServer struct {
	pb.UnimplementedSophiaServiceServer
	mockDictionaries map[uint32]*pb.Dictionary
	mu               sync.RWMutex
}

var (
	// Global Sophia instance with mock dictionaries
	sophiaInstance *sophiaServer
	sophiaOnce     sync.Once
)

// getSophiaServer returns the singleton Sophia server instance
func getSophiaServer() *sophiaServer {
	sophiaOnce.Do(func() {
		sophiaInstance = &sophiaServer{
			mockDictionaries: initializeMockDictionaries(),
		}
	})
	return sophiaInstance
}

// initializeMockDictionaries creates pre-seeded dictionaries for testing
func initializeMockDictionaries() map[uint32]*pb.Dictionary {
	now := uint64(time.Now().UnixMilli())

	dictionaries := make(map[uint32]*pb.Dictionary)

	// Dictionary 1: Protocol State Transitions
	dict1 := &pb.Dictionary{
		Id:          1,
		Name:        "Protocol State Transitions",
		Description: "Dictionary for encoding Unheaded protocol state machine transitions",
		CreatedAt:   now,
		Entries: []*pb.DictionaryEntry{
			{
				Key:       "IDLE_TO_ACTIVE",
				Value:     []byte{0x01, 0x02},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "ACTIVE_TO_CLOSING",
				Value:     []byte{0x02, 0x03},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "CLOSING_TO_CLOSED",
				Value:     []byte{0x03, 0x04},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "ANY_TO_IDLE",
				Value:     []byte{0x00, 0x01},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	dictionaries[1] = dict1

	// Dictionary 2: IPv6 Flow Label Mappings
	dict2 := &pb.Dictionary{
		Id:          2,
		Name:        "IPv6 Flow Label Mappings",
		Description: "Dictionary for IPv6 flow label to Unheaded flow mappings",
		CreatedAt:   now,
		Entries: []*pb.DictionaryEntry{
			{
				Key:       "FL_DNS_QUERIES",
				Value:     []byte{0x0A, 0x35, 0x04},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "FL_HTTP_TRAFFIC",
				Value:     []byte{0x0B, 0x50, 0x08},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "FL_HTTPS_TRAFFIC",
				Value:     []byte{0x0C, 0x60, 0x10},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "FL_CONTROL_PLANE",
				Value:     []byte{0x0D, 0x70, 0x20},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "FL_MEASUREMENT",
				Value:     []byte{0x0E, 0x80, 0x40},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	dictionaries[2] = dict2

	// Dictionary 3: Compression Algorithms
	dict3 := &pb.Dictionary{
		Id:          3,
		Name:        "Compression Algorithms",
		Description: "Dictionary for packet compression and transformation codecs",
		CreatedAt:   now,
		Entries: []*pb.DictionaryEntry{
			{
				Key:       "CODEC_IDENTITY",
				Value:     []byte{0x00},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "CODEC_GZIP",
				Value:     []byte{0x01},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "CODEC_BROTLI",
				Value:     []byte{0x02},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "CODEC_ZSTD",
				Value:     []byte{0x03},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				Key:       "CODEC_LZ4",
				Value:     []byte{0x04},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	dictionaries[3] = dict3

	return dictionaries
}

// List returns all available dictionaries with pagination
func (s *sophiaServer) List(ctx context.Context, req *pb.DictionaryListRequest) (*pb.DictionaryListResponse, error) {
	if globalMockMode {
		return s.listMock(req)
	}

	// BPF mode: read from pinned maps
	return s.listBPF(req)
}

// listMock returns dictionaries from in-memory mock storage
func (s *sophiaServer) listMock(req *pb.DictionaryListRequest) (*pb.DictionaryListResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert map to slice for pagination
	allDicts := make([]*pb.Dictionary, 0, len(s.mockDictionaries))
	for _, dict := range s.mockDictionaries {
		allDicts = append(allDicts, dict)
	}

	// Apply pagination
	limit := req.Limit
	if limit == 0 {
		limit = 100
	}
	offset := req.Offset

	if offset > int32(len(allDicts)) {
		offset = int32(len(allDicts))
	}

	end := offset + limit
	if end > int32(len(allDicts)) {
		end = int32(len(allDicts))
	}

	slicedDicts := allDicts[offset:end]

	return &pb.DictionaryListResponse{
		Dictionaries: slicedDicts,
		Total:        int32(len(allDicts)),
	}, nil
}

// listBPF reads dictionaries from BPF pinned maps
func (s *sophiaServer) listBPF(req *pb.DictionaryListRequest) (*pb.DictionaryListResponse, error) {
	// In a real implementation, this would:
	// 1. Open the pinned BPF map at /sys/fs/bpf/unheaded/sophia/maps/SOPHIA_MAP
	// 2. Iterate through all keys (dictionary IDs)
	// 3. Deserialize the DictionaryEntry values from BPF
	// 4. Return paginated results

	// For now, return mock data to indicate BPF mode is active
	return s.listMock(req)
}

// Get retrieves a specific dictionary by ID with all its entries
func (s *sophiaServer) Get(ctx context.Context, req *pb.DictionaryGetRequest) (*pb.DictionaryGetResponse, error) {
	if globalMockMode {
		return s.getMock(req)
	}

	return s.getBPF(req)
}

// getMock retrieves a dictionary from mock storage
func (s *sophiaServer) getMock(req *pb.DictionaryGetRequest) (*pb.DictionaryGetResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dict, found := s.mockDictionaries[req.DictionaryId]
	if !found {
		return &pb.DictionaryGetResponse{
			Found:        false,
			ErrorMessage: fmt.Sprintf("Dictionary %d not found", req.DictionaryId),
		}, nil
	}

	return &pb.DictionaryGetResponse{
		Dictionary: dict,
		Found:      true,
	}, nil
}

// getBPF retrieves a dictionary from BPF pinned maps
func (s *sophiaServer) getBPF(req *pb.DictionaryGetRequest) (*pb.DictionaryGetResponse, error) {
	// In a real implementation, this would:
	// 1. Open the pinned BPF map
	// 2. Lookup the dictionary by ID
	// 3. Deserialize the value
	// 4. Return with found=true/false

	// For now, fall back to mock
	return s.getMock(req)
}

// UpdateDictionary updates an entry in a dictionary (for development/testing)
func (s *sophiaServer) UpdateDictionary(ctx context.Context, dictID uint32, key string, value []byte) error {
	if !globalMockMode {
		return fmt.Errorf("dictionary updates only supported in mock mode")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dict, found := s.mockDictionaries[dictID]
	if !found {
		return fmt.Errorf("dictionary %d not found", dictID)
	}

	now := uint64(time.Now().UnixMilli())

	// Update existing entry or add new one
	found = false
	for _, entry := range dict.Entries {
		if entry.Key == key {
			entry.Value = value
			entry.UpdatedAt = now
			found = true
			break
		}
	}

	if !found {
		dict.Entries = append(dict.Entries, &pb.DictionaryEntry{
			Key:       key,
			Value:     value,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	return nil
}

// SophiaLookup performs a dictionary lookup and returns the encoded value
func (s *sophiaServer) SophiaLookup(dictID uint32, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dict, found := s.mockDictionaries[dictID]
	if !found {
		return nil, fmt.Errorf("dictionary %d not found", dictID)
	}

	for _, entry := range dict.Entries {
		if entry.Key == key {
			return entry.Value, nil
		}
	}

	return nil, fmt.Errorf("key %q not found in dictionary %d", key, dictID)
}

// SophiaInsert inserts a new entry into a dictionary (development only)
func (s *sophiaServer) SophiaInsert(dictID uint32, key string, value []byte) error {
	if !globalMockMode {
		return fmt.Errorf("inserts only supported in mock mode")
	}

	return s.UpdateDictionary(context.Background(), dictID, key, value)
}

// GetBuiltinDictionaries returns information about built-in dictionaries
func GetBuiltinDictionaries() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":          1,
			"name":        "Protocol State Transitions",
			"description": "Kingdom Mode state machine transitions",
			"entry_count": 4,
		},
		{
			"id":          2,
			"name":        "IPv6 Flow Label Mappings",
			"description": "Flow label to Unheaded flow mappings",
			"entry_count": 5,
		},
		{
			"id":          3,
			"name":        "Compression Algorithms",
			"description": "Packet compression codecs",
			"entry_count": 5,
		},
	}
}
