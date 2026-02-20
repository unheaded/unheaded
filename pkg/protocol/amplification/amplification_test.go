package amplification

import (
	"testing"
)

func TestRingPathCounterIncrement(t *testing.T) {
	counter := RingPathCounter(5)

	newCounter, err := counter.Increment()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if newCounter != 6 {
		t.Errorf("Expected 6, got %d", newCounter)
	}
}

func TestRingPathCounterIncrementAtMax(t *testing.T) {
	counter := RingPathCounter(DefaultMaxHopCount)

	_, err := counter.Increment()
	if err == nil {
		t.Errorf("Expected error when incrementing at max hop count")
	}
}

func TestRingPathCounterIncrementExceedMax(t *testing.T) {
	counter := RingPathCounter(DefaultMaxHopCount + 1)

	_, err := counter.Increment()
	if err == nil {
		t.Errorf("Expected error when counter exceeds max hop count")
	}
}

func TestShouldDropBelowThreshold(t *testing.T) {
	ringCount := uint16(5)
	threshold := uint16(10)

	if ShouldDrop(ringCount, threshold) {
		t.Errorf("Expected not to drop when ringCount < threshold")
	}
}

func TestShouldDropAtThreshold(t *testing.T) {
	ringCount := uint16(10)
	threshold := uint16(10)

	if ShouldDrop(ringCount, threshold) {
		t.Errorf("Expected not to drop when ringCount == threshold")
	}
}

func TestShouldDropAboveThreshold(t *testing.T) {
	ringCount := uint16(11)
	threshold := uint16(10)

	if !ShouldDrop(ringCount, threshold) {
		t.Errorf("Expected to drop when ringCount > threshold")
	}
}

func TestValidateAmplificationZeroSent(t *testing.T) {
	valid := ValidateAmplification(0, 0)
	if !valid {
		t.Errorf("Expected valid when both sent and received are 0")
	}
}

func TestValidateAmplificationZeroSentNonZeroReceived(t *testing.T) {
	valid := ValidateAmplification(0, 10)
	if valid {
		t.Errorf("Expected invalid when sent is 0 but received is non-zero")
	}
}

func TestValidateAmplificationWithinRatio(t *testing.T) {
	// 300 / 100 = 3, within MaxAmplificationRatio
	valid := ValidateAmplification(100, 300)
	if !valid {
		t.Errorf("Expected valid with ratio 3:1")
	}
}

func TestValidateAmplificationAtMaxRatio(t *testing.T) {
	// 3 * MaxAmplificationRatio = 9
	valid := ValidateAmplification(3, 9)
	if !valid {
		t.Errorf("Expected valid at max ratio")
	}
}

func TestValidateAmplificationExceedsRatio(t *testing.T) {
	// 301 / 100 > 3, exceeds MaxAmplificationRatio
	valid := ValidateAmplification(100, 301)
	if valid {
		t.Errorf("Expected invalid when exceeding MaxAmplificationRatio")
	}
}

func TestAmplificationTrackerRecordSent(t *testing.T) {
	tracker := NewAmplificationTracker()

	tracker.RecordSent()
	tracker.RecordSent()

	if tracker.PacketsSent() != 2 {
		t.Errorf("Expected 2 sent packets, got %d", tracker.PacketsSent())
	}
}

func TestAmplificationTrackerRecordReceived(t *testing.T) {
	tracker := NewAmplificationTracker()

	tracker.RecordReceived()
	tracker.RecordReceived()
	tracker.RecordReceived()

	if tracker.PacketsReceived() != 3 {
		t.Errorf("Expected 3 received packets, got %d", tracker.PacketsReceived())
	}
}

func TestAmplificationTrackerValidateRatio(t *testing.T) {
	tracker := NewAmplificationTracker()

	// Send 10 packets
	for i := 0; i < 10; i++ {
		tracker.RecordSent()
	}

	// Receive 30 packets (ratio = 3, at max)
	for i := 0; i < 30; i++ {
		tracker.RecordReceived()
	}

	if !tracker.ValidateRatio() {
		t.Errorf("Expected valid ratio 3:1")
	}
}

func TestAmplificationTrackerValidateRatioExceeded(t *testing.T) {
	tracker := NewAmplificationTracker()

	tracker.RecordSent()
	tracker.RecordReceived()
	tracker.RecordReceived()
	tracker.RecordReceived()
	tracker.RecordReceived() // ratio = 4

	if tracker.ValidateRatio() {
		t.Errorf("Expected invalid ratio 4:1")
	}
}

func TestAmplificationTrackerShouldDropPacket(t *testing.T) {
	tracker := NewAmplificationTracker()

	if tracker.ShouldDropPacket(15) {
		t.Errorf("Expected not to drop packet with ring count 15 < max hop count 16")
	}

	if tracker.ShouldDropPacket(17) {
		t.Errorf("Expected not to drop packet with ring count 17 == max hop count threshold+1")
	}

	if !tracker.ShouldDropPacket(20) {
		t.Errorf("Expected to drop packet with ring count 20 > max hop count")
	}
}

func TestAmplificationTrackerCustomHopCount(t *testing.T) {
	tracker := NewAmplificationTrackerWithHopCount(8)

	if tracker.ShouldDropPacket(8) {
		t.Errorf("Expected not to drop at threshold")
	}

	if !tracker.ShouldDropPacket(9) {
		t.Errorf("Expected to drop above threshold")
	}
}
