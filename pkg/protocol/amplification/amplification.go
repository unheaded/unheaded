// Package amplification provides ring path counter tracking and amplification validation
// for the Unheaded protocol.
package amplification

import (
	"fmt"
)

const (
	// MaxAmplificationRatio is the maximum amplification ratio allowed (like QUIC).
	MaxAmplificationRatio = 3

	// DefaultMaxHopCount is the default maximum hop count for ring paths.
	DefaultMaxHopCount uint8 = 16
)

// RingPathCounter represents a 2-byte counter field in the Monad header.
type RingPathCounter uint16

// Increment increments the ring path counter and returns the new value.
// Returns an error if the counter would exceed the maximum allowed value.
func (rpc RingPathCounter) Increment() (RingPathCounter, error) {
	if rpc >= uint16(DefaultMaxHopCount) {
		return 0, fmt.Errorf("ring path counter %d exceeds maximum hop count %d", rpc, DefaultMaxHopCount)
	}
	return rpc + 1, nil
}

// ShouldDrop determines whether a packet should be dropped based on the ring count
// and the provided threshold.
func ShouldDrop(ringCount uint16, threshold uint16) bool {
	return ringCount > threshold
}

// ValidateAmplification validates that the amplification ratio is within acceptable limits.
// Amplification ratio = packetsReceived / packetsSent
// Returns true if the ratio is within MaxAmplificationRatio.
func ValidateAmplification(packetsSent, packetsReceived uint64) bool {
	if packetsSent == 0 {
		return packetsReceived == 0
	}
	ratio := packetsReceived / packetsSent
	return ratio <= MaxAmplificationRatio
}

// AmplificationTracker tracks sent and received packets for amplification validation.
type AmplificationTracker struct {
	packetsSent     uint64
	packetsReceived uint64
	maxHopCount     uint8
}

// NewAmplificationTracker creates a new amplification tracker with default max hop count.
func NewAmplificationTracker() *AmplificationTracker {
	return &AmplificationTracker{
		maxHopCount: DefaultMaxHopCount,
	}
}

// NewAmplificationTrackerWithHopCount creates a new tracker with custom max hop count.
func NewAmplificationTrackerWithHopCount(maxHopCount uint8) *AmplificationTracker {
	return &AmplificationTracker{
		maxHopCount: maxHopCount,
	}
}

// RecordSent increments the sent packet counter.
func (at *AmplificationTracker) RecordSent() {
	at.packetsSent++
}

// RecordReceived increments the received packet counter.
func (at *AmplificationTracker) RecordReceived() {
	at.packetsReceived++
}

// ValidateRatio checks if the current amplification ratio is acceptable.
func (at *AmplificationTracker) ValidateRatio() bool {
	return ValidateAmplification(at.packetsSent, at.packetsReceived)
}

// ShouldDropPacket determines if a packet with the given ring count should be dropped.
func (at *AmplificationTracker) ShouldDropPacket(ringCount uint16) bool {
	threshold := uint16(at.maxHopCount)
	return ShouldDrop(ringCount, threshold)
}

// PacketsSent returns the current count of sent packets.
func (at *AmplificationTracker) PacketsSent() uint64 {
	return at.packetsSent
}

// PacketsReceived returns the current count of received packets.
func (at *AmplificationTracker) PacketsReceived() uint64 {
	return at.packetsReceived
}
