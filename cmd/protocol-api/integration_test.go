// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"testing"

	pb "unheaded/proto/unheaded/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// setupMockMode enables mock mode for integration tests and returns a cleanup function.
func setupMockMode(t *testing.T) {
	t.Helper()
	globalMockMode = true
}

// ============================================================================
// Test 1: Monad Encode -> Decode roundtrip
// ============================================================================

func TestIntegrationMonadEncodeDecodeRoundtrip(t *testing.T) {
	setupMockMode(t)
	ctx := context.Background()
	monadSvc := &monadServer{}

	tests := []struct {
		name          string
		flowLabel     uint32
		kingdomMode   pb.KingdomMode
		timestamp     uint64
		sophiaDictID  uint32
		wotanSequence uint32
	}{
		{
			name:          "active flow with typical values",
			flowLabel:     0x12345,
			kingdomMode:   pb.KingdomMode_KINGDOM_MODE_ACTIVE,
			timestamp:     1740000000000,
			sophiaDictID:  1,
			wotanSequence: 42,
		},
		{
			name:          "idle flow with zero fields",
			flowLabel:     0x00000,
			kingdomMode:   pb.KingdomMode_KINGDOM_MODE_IDLE,
			timestamp:     0,
			sophiaDictID:  0,
			wotanSequence: 0,
		},
		{
			name:          "closing flow at field boundaries",
			flowLabel:     0xFFFFF, // max 20-bit
			kingdomMode:   pb.KingdomMode_KINGDOM_MODE_CLOSING,
			timestamp:     0xFFFFFFFFFF, // max 40-bit
			sophiaDictID:  0xFFFF,       // max 16-bit
			wotanSequence: 0xFFFF,       // max 16-bit in compact encoding
		},
		{
			name:          "closed flow with mid-range values",
			flowLabel:     0x80000,
			kingdomMode:   pb.KingdomMode_KINGDOM_MODE_CLOSED,
			timestamp:     9999999999,
			sophiaDictID:  256,
			wotanSequence: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encReq := &pb.EncodeRequest{
				FlowLabel:     tt.flowLabel,
				KingdomMode:   tt.kingdomMode,
				Timestamp:     tt.timestamp,
				SophiaDictId:  tt.sophiaDictID,
				WotanSequence: tt.wotanSequence,
			}

			encResp, err := monadSvc.Encode(ctx, encReq)
			if err != nil {
				t.Fatalf("Encode() error: %v", err)
			}
			if !encResp.Success {
				t.Fatalf("Encode() success=false, error: %s", encResp.ErrorMessage)
			}
			if len(encResp.Data) != MonadPayloadSize {
				t.Fatalf("Encode() data length=%d, want %d", len(encResp.Data), MonadPayloadSize)
			}

			// Decode
			decReq := &pb.DecodeRequest{Data: encResp.Data}
			decResp, err := monadSvc.Decode(ctx, decReq)
			if err != nil {
				t.Fatalf("Decode() error: %v", err)
			}
			if !decResp.Valid {
				t.Fatalf("Decode() valid=false, error: %s", decResp.ErrorMessage)
			}
			if !decResp.CrcValid {
				t.Fatalf("Decode() CRC invalid after roundtrip")
			}

			// Verify fields match (with field-width masking)
			expectedFlowLabel := tt.flowLabel & 0xFFFFF
			if decResp.FlowLabel != expectedFlowLabel {
				t.Errorf("FlowLabel: got %d, want %d", decResp.FlowLabel, expectedFlowLabel)
			}
			if decResp.KingdomMode != tt.kingdomMode {
				t.Errorf("KingdomMode: got %v, want %v", decResp.KingdomMode, tt.kingdomMode)
			}
			expectedTimestamp := tt.timestamp & 0xFFFFFFFFFF
			if decResp.Timestamp != expectedTimestamp {
				t.Errorf("Timestamp: got %d, want %d", decResp.Timestamp, expectedTimestamp)
			}
			expectedSophia := tt.sophiaDictID & 0xFFFF
			if decResp.SophiaDictId != expectedSophia {
				t.Errorf("SophiaDictId: got %d, want %d", decResp.SophiaDictId, expectedSophia)
			}
			expectedWotan := tt.wotanSequence & 0xFFFF
			if decResp.WotanSequence != expectedWotan {
				t.Errorf("WotanSequence: got %d, want %d", decResp.WotanSequence, expectedWotan)
			}
			// CRC stored in response should match the one from encoding
			if decResp.Crc16 != encResp.Crc16 {
				t.Errorf("CRC16 mismatch: decode=%d, encode=%d", decResp.Crc16, encResp.Crc16)
			}
		})
	}
}

// ============================================================================
// Test 2: Monad Encode -> Validate CRC
// ============================================================================

func TestIntegrationMonadEncodeValidate(t *testing.T) {
	setupMockMode(t)
	ctx := context.Background()
	monadSvc := &monadServer{}

	// Encode a Monad register
	encReq := &pb.EncodeRequest{
		FlowLabel:     0xABCDE,
		KingdomMode:   pb.KingdomMode_KINGDOM_MODE_ACTIVE,
		Timestamp:     1740000000000,
		SophiaDictId:  2,
		WotanSequence: 100,
	}

	encResp, err := monadSvc.Encode(ctx, encReq)
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	if !encResp.Success {
		t.Fatalf("Encode() failed: %s", encResp.ErrorMessage)
	}

	// Validate the encoded data (Validate is an alias for Decode that checks CRC)
	valReq := &pb.DecodeRequest{Data: encResp.Data}
	valResp, err := monadSvc.Validate(ctx, valReq)
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
	if !valResp.CrcValid {
		t.Errorf("Validate() CRC invalid for freshly encoded data")
	}
	if !valResp.Valid {
		t.Errorf("Validate() valid=false for freshly encoded data")
	}

	// Now corrupt one byte and verify validation fails
	corruptedData := make([]byte, len(encResp.Data))
	copy(corruptedData, encResp.Data)
	corruptedData[5] ^= 0xFF // Flip bits in the middle of the payload

	corruptReq := &pb.DecodeRequest{Data: corruptedData}
	corruptResp, err := monadSvc.Validate(ctx, corruptReq)
	if err != nil {
		t.Fatalf("Validate() error on corrupted data: %v", err)
	}
	if corruptResp.CrcValid {
		t.Errorf("Validate() CRC valid for corrupted data -- should have been invalid")
	}
}

// ============================================================================
// Test 3: Sophia Dictionary operations
// ============================================================================

func TestIntegrationSophiaDictionaryOperations(t *testing.T) {
	setupMockMode(t)
	ctx := context.Background()

	// Create a fresh sophiaServer with mock dictionaries
	sophiaSvc := &sophiaServer{
		mockDictionaries: initializeMockDictionaries(),
	}

	// Step 1: List dictionaries -- should have 3 pre-seeded
	listReq := &pb.DictionaryListRequest{Limit: 100, Offset: 0}
	listResp, err := sophiaSvc.List(ctx, listReq)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if listResp.Total != 3 {
		t.Fatalf("List() total=%d, want 3", listResp.Total)
	}
	if len(listResp.Dictionaries) != 3 {
		t.Fatalf("List() returned %d dictionaries, want 3", len(listResp.Dictionaries))
	}

	// Step 2: Get dictionary 1 by ID
	getReq := &pb.DictionaryGetRequest{DictionaryId: 1}
	getResp, err := sophiaSvc.Get(ctx, getReq)
	if err != nil {
		t.Fatalf("Get(1) error: %v", err)
	}
	if !getResp.Found {
		t.Fatalf("Get(1) found=false, want true")
	}
	if getResp.Dictionary.Name != "Protocol State Transitions" {
		t.Errorf("Get(1) name=%q, want %q", getResp.Dictionary.Name, "Protocol State Transitions")
	}
	originalEntryCount := len(getResp.Dictionary.Entries)
	if originalEntryCount != 4 {
		t.Errorf("Get(1) entries=%d, want 4", originalEntryCount)
	}

	// Step 3: Update an existing entry
	err = sophiaSvc.UpdateDictionary(ctx, 1, "IDLE_TO_ACTIVE", []byte{0xAA, 0xBB})
	if err != nil {
		t.Fatalf("UpdateDictionary() error: %v", err)
	}

	// Step 4: Add a new entry
	err = sophiaSvc.UpdateDictionary(ctx, 1, "EMERGENCY_RESET", []byte{0xFF, 0x00})
	if err != nil {
		t.Fatalf("UpdateDictionary() new entry error: %v", err)
	}

	// Step 5: Lookup the updated entry
	val, err := sophiaSvc.SophiaLookup(1, "IDLE_TO_ACTIVE")
	if err != nil {
		t.Fatalf("SophiaLookup(IDLE_TO_ACTIVE) error: %v", err)
	}
	if !bytes.Equal(val, []byte{0xAA, 0xBB}) {
		t.Errorf("SophiaLookup(IDLE_TO_ACTIVE) got %x, want aabb", val)
	}

	// Step 6: Lookup the new entry
	val, err = sophiaSvc.SophiaLookup(1, "EMERGENCY_RESET")
	if err != nil {
		t.Fatalf("SophiaLookup(EMERGENCY_RESET) error: %v", err)
	}
	if !bytes.Equal(val, []byte{0xFF, 0x00}) {
		t.Errorf("SophiaLookup(EMERGENCY_RESET) got %x, want ff00", val)
	}

	// Step 7: Verify entry count increased
	getResp2, err := sophiaSvc.Get(ctx, getReq)
	if err != nil {
		t.Fatalf("Get(1) after update error: %v", err)
	}
	if len(getResp2.Dictionary.Entries) != originalEntryCount+1 {
		t.Errorf("Entry count after add: got %d, want %d", len(getResp2.Dictionary.Entries), originalEntryCount+1)
	}

	// Step 8: Get non-existent dictionary returns found=false
	getReq404 := &pb.DictionaryGetRequest{DictionaryId: 999}
	getResp404, err := sophiaSvc.Get(ctx, getReq404)
	if err != nil {
		t.Fatalf("Get(999) error: %v", err)
	}
	if getResp404.Found {
		t.Errorf("Get(999) found=true, want false")
	}

	// Step 9: Lookup non-existent key returns error
	_, err = sophiaSvc.SophiaLookup(1, "NONEXISTENT_KEY")
	if err == nil {
		t.Errorf("SophiaLookup(NONEXISTENT_KEY) expected error, got nil")
	}
}

// ============================================================================
// Test 4: Wotan Write -> Read roundtrip
// ============================================================================

func TestIntegrationWotanWriteReadRoundtrip(t *testing.T) {
	setupMockMode(t)
	ctx := context.Background()

	// Create a fresh wotanServer with empty storage
	wotanSvc := &wotanServer{
		mockStorage: make(map[uint32]*pb.MemorySlot),
	}

	// Step 1: Read a non-existent flow -- should return found=false
	readReq := &pb.ReadRequest{FlowLabel: 0x42}
	readResp, err := wotanSvc.Read(ctx, readReq)
	if err != nil {
		t.Fatalf("Read(0x42) error: %v", err)
	}
	if readResp.Found {
		t.Fatalf("Read(0x42) found=true before any write")
	}

	// Step 2: Write 32 bytes of state data
	stateData := make([]byte, 32)
	for i := range stateData {
		stateData[i] = byte(i + 0x10)
	}
	writeReq := &pb.WriteRequest{
		FlowLabel:      0x42,
		StateData:      stateData,
		SequenceNumber: 7,
	}

	writeResp, err := wotanSvc.Write(ctx, writeReq)
	if err != nil {
		t.Fatalf("Write(0x42) error: %v", err)
	}
	if !writeResp.Success {
		t.Fatalf("Write(0x42) success=false: %s", writeResp.ErrorMessage)
	}
	if writeResp.FlowLabel != 0x42 {
		t.Errorf("Write(0x42) flow_label=%d, want 66", writeResp.FlowLabel)
	}
	if writeResp.WrittenAt == 0 {
		t.Errorf("Write(0x42) written_at=0, expected timestamp")
	}

	// Step 3: Read it back
	readResp, err = wotanSvc.Read(ctx, readReq)
	if err != nil {
		t.Fatalf("Read(0x42) after write error: %v", err)
	}
	if !readResp.Found {
		t.Fatalf("Read(0x42) found=false after write")
	}
	if readResp.Slot == nil {
		t.Fatalf("Read(0x42) slot is nil")
	}
	if readResp.Slot.FlowLabel != 0x42 {
		t.Errorf("Read slot flow_label=%d, want 66", readResp.Slot.FlowLabel)
	}
	if readResp.Slot.SequenceNumber != 7 {
		t.Errorf("Read slot sequence_number=%d, want 7", readResp.Slot.SequenceNumber)
	}
	if !bytes.Equal(readResp.Slot.StateData, stateData) {
		t.Errorf("Read slot state_data mismatch: got %x, want %x", readResp.Slot.StateData, stateData)
	}

	// Step 4: Overwrite with new data and verify update
	newStateData := make([]byte, 32)
	for i := range newStateData {
		newStateData[i] = byte(0xFF - i)
	}
	writeReq2 := &pb.WriteRequest{
		FlowLabel:      0x42,
		StateData:      newStateData,
		SequenceNumber: 8,
	}

	writeResp2, err := wotanSvc.Write(ctx, writeReq2)
	if err != nil {
		t.Fatalf("Write(0x42) overwrite error: %v", err)
	}
	if !writeResp2.Success {
		t.Fatalf("Write(0x42) overwrite success=false: %s", writeResp2.ErrorMessage)
	}

	readResp2, err := wotanSvc.Read(ctx, readReq)
	if err != nil {
		t.Fatalf("Read(0x42) after overwrite error: %v", err)
	}
	if !bytes.Equal(readResp2.Slot.StateData, newStateData) {
		t.Errorf("Overwritten state_data mismatch")
	}
	if readResp2.Slot.SequenceNumber != 8 {
		t.Errorf("Overwritten sequence_number=%d, want 8", readResp2.Slot.SequenceNumber)
	}

	// Step 5: Write with invalid state_data length -- should fail gracefully
	badWriteReq := &pb.WriteRequest{
		FlowLabel:      0x99,
		StateData:      []byte{0x01, 0x02, 0x03}, // Only 3 bytes, needs 32
		SequenceNumber: 1,
	}
	badWriteResp, err := wotanSvc.Write(ctx, badWriteReq)
	if err != nil {
		t.Fatalf("Write(bad length) error: %v", err)
	}
	if badWriteResp.Success {
		t.Errorf("Write(bad length) success=true, want false")
	}

	// Step 6: Verify flow count
	if wotanSvc.GetFlowCount() != 1 {
		t.Errorf("Flow count=%d, want 1 (bad write should not have been stored)", wotanSvc.GetFlowCount())
	}
}

// ============================================================================
// Test 5: Flow lifecycle
// ============================================================================

func TestIntegrationFlowLifecycle(t *testing.T) {
	setupMockMode(t)
	ctx := context.Background()

	// Create a fresh flowServer with empty state
	flowSvc := &flowServer{
		mockFlows: make(map[uint32]*pb.FlowState),
	}

	// Step 1: List flows -- should be empty
	listResp, err := flowSvc.List(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if listResp.Total != 0 {
		t.Fatalf("List() total=%d, want 0", listResp.Total)
	}

	// Step 2: Create flow 1
	flow1, err := flowSvc.CreateFlow(ctx, 0x100, pb.KingdomMode_KINGDOM_MODE_IDLE, "2001:db8::1", "2001:db8::2", 1)
	if err != nil {
		t.Fatalf("CreateFlow(0x100) error: %v", err)
	}
	if flow1.FlowLabel != 0x100 {
		t.Errorf("CreateFlow flow_label=%d, want 256", flow1.FlowLabel)
	}
	if flow1.KingdomMode != pb.KingdomMode_KINGDOM_MODE_IDLE {
		t.Errorf("CreateFlow kingdom_mode=%v, want IDLE", flow1.KingdomMode)
	}
	if flow1.CreatedAt == 0 {
		t.Errorf("CreateFlow created_at=0, expected timestamp")
	}

	// Step 3: Create flow 2
	_, err = flowSvc.CreateFlow(ctx, 0x200, pb.KingdomMode_KINGDOM_MODE_ACTIVE, "2001:db8::3", "2001:db8::4", 2)
	if err != nil {
		t.Fatalf("CreateFlow(0x200) error: %v", err)
	}

	// Step 4: Create flow 3
	_, err = flowSvc.CreateFlow(ctx, 0x300, pb.KingdomMode_KINGDOM_MODE_ACTIVE, "2001:db8::5", "2001:db8::6", 3)
	if err != nil {
		t.Fatalf("CreateFlow(0x300) error: %v", err)
	}

	// Step 5: List flows -- should be 3
	listResp, err = flowSvc.List(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if listResp.Total != 3 {
		t.Errorf("List() total=%d, want 3", listResp.Total)
	}
	if len(listResp.Flows) != 3 {
		t.Errorf("List() flows=%d, want 3", len(listResp.Flows))
	}

	// Step 6: Update flow 1 state: IDLE -> ACTIVE
	err = flowSvc.UpdateFlowState(ctx, 0x100, pb.KingdomMode_KINGDOM_MODE_ACTIVE)
	if err != nil {
		t.Fatalf("UpdateFlowState(0x100, ACTIVE) error: %v", err)
	}

	// Step 7: Verify the state change
	flowState, err := flowSvc.GetState(ctx, &pb.ReadRequest{FlowLabel: 0x100})
	if err != nil {
		t.Fatalf("GetState(0x100) error: %v", err)
	}
	if flowState.KingdomMode != pb.KingdomMode_KINGDOM_MODE_ACTIVE {
		t.Errorf("GetState(0x100) kingdom_mode=%v, want ACTIVE", flowState.KingdomMode)
	}

	// Step 8: Update flow 1 state: ACTIVE -> CLOSING
	err = flowSvc.UpdateFlowState(ctx, 0x100, pb.KingdomMode_KINGDOM_MODE_CLOSING)
	if err != nil {
		t.Fatalf("UpdateFlowState(0x100, CLOSING) error: %v", err)
	}

	// Step 9: Verify GetFlowCount
	if flowSvc.GetFlowCount() != 3 {
		t.Errorf("GetFlowCount()=%d, want 3", flowSvc.GetFlowCount())
	}

	// Step 10: Get statistics
	stats := flowSvc.GetStatistics()
	if stats.TotalFlows != 3 {
		t.Errorf("Stats.TotalFlows=%d, want 3", stats.TotalFlows)
	}
	// We have 2 ACTIVE and 1 CLOSING now
	if stats.ActiveFlows != 2 {
		t.Errorf("Stats.ActiveFlows=%d, want 2", stats.ActiveFlows)
	}
	if stats.ClosingFlows != 1 {
		t.Errorf("Stats.ClosingFlows=%d, want 1", stats.ClosingFlows)
	}

	// Step 11: Close a flow
	err = flowSvc.CloseFlow(ctx, 0x200)
	if err != nil {
		t.Fatalf("CloseFlow(0x200) error: %v", err)
	}

	// Verify it moved to CLOSING state
	flowState2, err := flowSvc.GetState(ctx, &pb.ReadRequest{FlowLabel: 0x200})
	if err != nil {
		t.Fatalf("GetState(0x200) error: %v", err)
	}
	if flowState2.KingdomMode != pb.KingdomMode_KINGDOM_MODE_CLOSING {
		t.Errorf("GetState(0x200) after close: kingdom_mode=%v, want CLOSING", flowState2.KingdomMode)
	}

	// Step 12: Delete a flow
	err = flowSvc.DeleteFlow(ctx, 0x300)
	if err != nil {
		t.Fatalf("DeleteFlow(0x300) error: %v", err)
	}

	// Step 13: Verify count decreased
	if flowSvc.GetFlowCount() != 2 {
		t.Errorf("GetFlowCount() after delete=%d, want 2", flowSvc.GetFlowCount())
	}

	// Step 14: Update non-existent flow returns error
	err = flowSvc.UpdateFlowState(ctx, 0x999, pb.KingdomMode_KINGDOM_MODE_ACTIVE)
	if err == nil {
		t.Errorf("UpdateFlowState(0x999) expected error, got nil")
	}
}

// ============================================================================
// Test 6: Anamnesis event tracking
// ============================================================================

func TestIntegrationAnamnesisEventTracking(t *testing.T) {
	setupMockMode(t)
	ctx := context.Background()

	// Create a fresh anamnesisServer with empty event log
	amnesisSvc := &anamnesisServer{
		mockEventLog: make([]*pb.Event, 0),
		subscribers:  make([]chan *pb.Event, 0),
	}

	// Step 1: Query empty log -- should return 0 events
	queryReq := &pb.EventQuery{Limit: 100}
	queryResp, err := amnesisSvc.Query(ctx, queryReq)
	if err != nil {
		t.Fatalf("Query() empty error: %v", err)
	}
	if queryResp.TotalCount != 0 {
		t.Fatalf("Query() empty total_count=%d, want 0", queryResp.TotalCount)
	}

	// Step 2: Publish a FLOW_CREATE event
	event1 := &pb.Event{
		Type:      pb.Event_EVENT_TYPE_FLOW_CREATE,
		FlowLabel: 0x100,
		Payload:   []byte("Flow 0x100 created"),
		Source:    "test",
		Metadata: map[string]string{
			"kingdom_mode": "IDLE",
			"source_addr":  "2001:db8::1",
		},
	}
	amnesisSvc.PublishEvent(event1)

	// Step 3: Publish a FLOW_UPDATE event
	event2 := &pb.Event{
		Type:      pb.Event_EVENT_TYPE_FLOW_UPDATE,
		FlowLabel: 0x100,
		Payload:   []byte("Flow 0x100 state changed"),
		Source:    "test",
		Metadata: map[string]string{
			"old_state": "IDLE",
			"new_state": "ACTIVE",
		},
	}
	amnesisSvc.PublishEvent(event2)

	// Step 4: Publish a PACKET_RECEIVED event for a different flow
	event3 := &pb.Event{
		Type:      pb.Event_EVENT_TYPE_PACKET_RECEIVED,
		FlowLabel: 0x200,
		Payload:   []byte("Packet received: 64 bytes"),
		Source:    "kernel",
		Metadata: map[string]string{
			"packet_size": "64",
		},
	}
	amnesisSvc.PublishEvent(event3)

	// Step 5: Publish another PACKET_RECEIVED for flow 0x100
	event4 := &pb.Event{
		Type:      pb.Event_EVENT_TYPE_PACKET_RECEIVED,
		FlowLabel: 0x100,
		Payload:   []byte("Packet received: 128 bytes"),
		Source:    "kernel",
		Metadata: map[string]string{
			"packet_size": "128",
		},
	}
	amnesisSvc.PublishEvent(event4)

	// Step 6: Publish an ERROR event
	event5 := &pb.Event{
		Type:      pb.Event_EVENT_TYPE_ERROR,
		FlowLabel: 0x100,
		Payload:   []byte("CRC check failed"),
		Source:    "system",
	}
	amnesisSvc.PublishEvent(event5)

	// Step 7: Verify total event count
	if amnesisSvc.GetEventCount() != 5 {
		t.Fatalf("GetEventCount()=%d, want 5", amnesisSvc.GetEventCount())
	}

	// Step 8: Query all events -- should return 5
	queryResp, err = amnesisSvc.Query(ctx, queryReq)
	if err != nil {
		t.Fatalf("Query() all error: %v", err)
	}
	if queryResp.TotalCount != 5 {
		t.Errorf("Query() all total_count=%d, want 5", queryResp.TotalCount)
	}

	// Step 9: Query by event type -- PACKET_RECEIVED only
	typeQuery := &pb.EventQuery{
		EventTypes: []pb.Event_EventType{pb.Event_EVENT_TYPE_PACKET_RECEIVED},
		Limit:      100,
	}
	typeResp, err := amnesisSvc.Query(ctx, typeQuery)
	if err != nil {
		t.Fatalf("Query(PACKET_RECEIVED) error: %v", err)
	}
	if typeResp.TotalCount != 2 {
		t.Errorf("Query(PACKET_RECEIVED) total_count=%d, want 2", typeResp.TotalCount)
	}
	for _, evt := range typeResp.Events {
		if evt.Type != pb.Event_EVENT_TYPE_PACKET_RECEIVED {
			t.Errorf("Query(PACKET_RECEIVED) returned event type=%v, want PACKET_RECEIVED", evt.Type)
		}
	}

	// Step 10: Query by flow label -- 0x100 only
	flowQuery := &pb.EventQuery{
		FlowLabels: []uint32{0x100},
		Limit:      100,
	}
	flowResp, err := amnesisSvc.Query(ctx, flowQuery)
	if err != nil {
		t.Fatalf("Query(flow=0x100) error: %v", err)
	}
	if flowResp.TotalCount != 4 {
		t.Errorf("Query(flow=0x100) total_count=%d, want 4", flowResp.TotalCount)
	}
	for _, evt := range flowResp.Events {
		if evt.FlowLabel != 0x100 {
			t.Errorf("Query(flow=0x100) returned event with flow_label=%d", evt.FlowLabel)
		}
	}

	// Step 11: Query by flow label AND event type
	combinedQuery := &pb.EventQuery{
		FlowLabels: []uint32{0x100},
		EventTypes: []pb.Event_EventType{pb.Event_EVENT_TYPE_FLOW_CREATE},
		Limit:      100,
	}
	combinedResp, err := amnesisSvc.Query(ctx, combinedQuery)
	if err != nil {
		t.Fatalf("Query(flow=0x100, type=FLOW_CREATE) error: %v", err)
	}
	if combinedResp.TotalCount != 1 {
		t.Errorf("Query(flow=0x100, type=FLOW_CREATE) total_count=%d, want 1", combinedResp.TotalCount)
	}

	// Step 12: Use helper methods to verify
	flowEvents := amnesisSvc.GetEventsByFlowLabel(0x100)
	if len(flowEvents) != 4 {
		t.Errorf("GetEventsByFlowLabel(0x100)=%d, want 4", len(flowEvents))
	}

	errorEvents := amnesisSvc.GetEventsByType(pb.Event_EVENT_TYPE_ERROR)
	if len(errorEvents) != 1 {
		t.Errorf("GetEventsByType(ERROR)=%d, want 1", len(errorEvents))
	}
	if string(errorEvents[0].Payload) != "CRC check failed" {
		t.Errorf("Error event payload=%q, want %q", string(errorEvents[0].Payload), "CRC check failed")
	}

	// Step 13: Verify statistics
	stats := amnesisSvc.GetStatistics()
	if stats.TotalEvents != 5 {
		t.Errorf("Stats.TotalEvents=%d, want 5", stats.TotalEvents)
	}
	if stats.FlowCount != 2 {
		t.Errorf("Stats.FlowCount=%d, want 2 (0x100 and 0x200)", stats.FlowCount)
	}
	if stats.EventTypeCount["EVENT_TYPE_PACKET_RECEIVED"] != 2 {
		t.Errorf("Stats.EventTypeCount[PACKET_RECEIVED]=%d, want 2", stats.EventTypeCount["EVENT_TYPE_PACKET_RECEIVED"])
	}
	if stats.EventTypeCount["EVENT_TYPE_FLOW_CREATE"] != 1 {
		t.Errorf("Stats.EventTypeCount[FLOW_CREATE]=%d, want 1", stats.EventTypeCount["EVENT_TYPE_FLOW_CREATE"])
	}
}

// ============================================================================
// Cross-service pipeline: Flow Create -> Monad Encode -> Wotan Store -> Read
// ============================================================================

func TestIntegrationCrossServicePipeline(t *testing.T) {
	setupMockMode(t)
	ctx := context.Background()

	monadSvc := &monadServer{}
	wotanSvc := &wotanServer{
		mockStorage: make(map[uint32]*pb.MemorySlot),
	}
	flowSvc := &flowServer{
		mockFlows: make(map[uint32]*pb.FlowState),
	}

	// Step 1: Create a flow
	flowLabel := uint32(0xABC)
	_, err := flowSvc.CreateFlow(ctx, flowLabel, pb.KingdomMode_KINGDOM_MODE_ACTIVE, "2001:db8::A", "2001:db8::B", 2)
	if err != nil {
		t.Fatalf("CreateFlow error: %v", err)
	}

	// Step 2: Encode a Monad register for this flow
	encReq := &pb.EncodeRequest{
		FlowLabel:     flowLabel,
		KingdomMode:   pb.KingdomMode_KINGDOM_MODE_ACTIVE,
		Timestamp:     1740000000000,
		SophiaDictId:  2,
		WotanSequence: 1,
	}
	encResp, err := monadSvc.Encode(ctx, encReq)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	if !encResp.Success {
		t.Fatalf("Encode failed: %s", encResp.ErrorMessage)
	}

	// Step 3: Store the encoded Monad in Wotan (pad to 32 bytes for state storage)
	stateData := make([]byte, 32)
	copy(stateData, encResp.Data) // Copy 20-byte Monad into 32-byte state slot

	writeReq := &pb.WriteRequest{
		FlowLabel:      flowLabel,
		StateData:      stateData,
		SequenceNumber: 1,
	}
	writeResp, err := wotanSvc.Write(ctx, writeReq)
	if err != nil {
		t.Fatalf("Wotan Write error: %v", err)
	}
	if !writeResp.Success {
		t.Fatalf("Wotan Write failed: %s", writeResp.ErrorMessage)
	}

	// Step 4: Read back from Wotan
	readResp, err := wotanSvc.Read(ctx, &pb.ReadRequest{FlowLabel: flowLabel})
	if err != nil {
		t.Fatalf("Wotan Read error: %v", err)
	}
	if !readResp.Found {
		t.Fatalf("Wotan Read found=false")
	}

	// Step 5: Extract the 20-byte Monad from the 32-byte state and decode it
	storedMonad := readResp.Slot.StateData[:MonadPayloadSize]
	decResp, err := monadSvc.Decode(ctx, &pb.DecodeRequest{Data: storedMonad})
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if !decResp.Valid {
		t.Fatalf("Decoded Monad invalid after Wotan roundtrip: %s", decResp.ErrorMessage)
	}
	if !decResp.CrcValid {
		t.Fatalf("Decoded Monad CRC invalid after Wotan roundtrip")
	}

	// Step 6: Verify decoded fields match original encode request
	if decResp.FlowLabel != (flowLabel & 0xFFFFF) {
		t.Errorf("Pipeline FlowLabel: got %d, want %d", decResp.FlowLabel, flowLabel&0xFFFFF)
	}
	if decResp.KingdomMode != pb.KingdomMode_KINGDOM_MODE_ACTIVE {
		t.Errorf("Pipeline KingdomMode: got %v, want ACTIVE", decResp.KingdomMode)
	}
	if decResp.SophiaDictId != 2 {
		t.Errorf("Pipeline SophiaDictId: got %d, want 2", decResp.SophiaDictId)
	}

	// Step 7: Verify the flow is still tracked
	flowState, err := flowSvc.GetState(ctx, &pb.ReadRequest{FlowLabel: flowLabel})
	if err != nil {
		t.Fatalf("GetState error: %v", err)
	}
	if flowState.FlowLabel != flowLabel {
		t.Errorf("Flow state label: got %d, want %d", flowState.FlowLabel, flowLabel)
	}
}
