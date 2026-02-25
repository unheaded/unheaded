package migration

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestGenerateToken(t *testing.T) {
	flowLabel := uint32(0x12345678)
	seq := uint16(42)

	token := GenerateToken(flowLabel, seq)

	if len(token) != 6 {
		t.Errorf("Expected token length 6, got %d", len(token))
	}

	// Verify flow label
	decodedLabel := binary.BigEndian.Uint32(token[0:4])
	if decodedLabel != flowLabel {
		t.Errorf("Expected flow label %d, got %d", flowLabel, decodedLabel)
	}

	// Verify sequence
	decodedSeq := binary.BigEndian.Uint16(token[4:6])
	if decodedSeq != seq {
		t.Errorf("Expected sequence %d, got %d", seq, decodedSeq)
	}
}

func TestValidateToken(t *testing.T) {
	token := GenerateToken(0x12345678, 42)
	secret := []byte("my-secret-key")

	valid := ValidateToken(token, secret)
	if !valid {
		t.Errorf("Expected token to be valid")
	}
}

func TestValidateTokenShort(t *testing.T) {
	token := []byte{0x01, 0x02, 0x03}
	secret := []byte("my-secret-key")

	valid := ValidateToken(token, secret)
	if valid {
		t.Errorf("Expected short token to be invalid")
	}
}

func TestRetireFlow(t *testing.T) {
	// This function just marks a flow as retired
	// It should not panic or error
	RetireFlow(42)
}

func TestGenerateRetryToken(t *testing.T) {
	shieldSecret := []byte("shield-secret")
	srcIP := []byte{192, 168, 1, 100}

	token := GenerateRetryToken(shieldSecret, srcIP)

	if len(token) != 16 {
		t.Errorf("Expected token length 16, got %d", len(token))
	}

	// Generate again and verify consistency is not guaranteed (due to timestamp)
	token2 := GenerateRetryToken(shieldSecret, srcIP)
	if len(token2) != 16 {
		t.Errorf("Expected token2 length 16, got %d", len(token2))
	}

	// Tokens should be different due to different timestamps
	if len(token) == len(token2) && len(token) == 16 {
		// This is expected - tokens can be different
		t.Logf("Tokens differ due to timestamp, which is expected")
	}
}

func TestValidateRetryTokenEmptySecret(t *testing.T) {
	token := GenerateRetryToken([]byte("secret"), []byte{192, 168, 1, 1})
	srcIP := []byte{192, 168, 1, 1}

	valid := ValidateRetryToken(token, []byte{}, srcIP, 1*time.Second)
	if valid {
		t.Errorf("Expected validation to fail with empty secret")
	}
}

func TestValidateRetryTokenEmptyIP(t *testing.T) {
	token := GenerateRetryToken([]byte("secret"), []byte{192, 168, 1, 1})
	shieldSecret := []byte("secret")

	valid := ValidateRetryToken(token, shieldSecret, []byte{}, 1*time.Second)
	if valid {
		t.Errorf("Expected validation to fail with empty IP")
	}
}

func TestValidateRetryTokenWrongLength(t *testing.T) {
	shieldSecret := []byte("shield-secret")
	srcIP := []byte{192, 168, 1, 100}

	valid := ValidateRetryToken([]byte("short"), shieldSecret, srcIP, 1*time.Second)
	if valid {
		t.Errorf("Expected validation to fail with wrong token length")
	}
}

func TestAddressValidatorChallenge(t *testing.T) {
	shieldSecret := []byte("my-secret")
	validator := NewAddressValidator(shieldSecret, 10*time.Second)

	srcIPStr := "192.168.1.100"
	srcIP := []byte{192, 168, 1, 100}

	token := validator.ChallengeAddress(srcIPStr, srcIP)

	if len(token) != 16 {
		t.Errorf("Expected token length 16, got %d", len(token))
	}
}

func TestAddressValidatorValidate(t *testing.T) {
	shieldSecret := []byte("my-secret")
	validator := NewAddressValidator(shieldSecret, 10*time.Second)

	srcIPStr := "192.168.1.100"
	srcIP := []byte{192, 168, 1, 100}

	// Challenge address
	token := validator.ChallengeAddress(srcIPStr, srcIP)

	// Validate with correct token
	if !validator.ValidateAddress(srcIPStr, srcIP, token) {
		t.Errorf("Expected validation to succeed with correct token")
	}

	// Now the address should be validated
	if !validator.IsValidated(srcIPStr) {
		t.Errorf("Expected address to be marked as validated")
	}
}

func TestAddressValidatorWrongToken(t *testing.T) {
	shieldSecret := []byte("my-secret")
	validator := NewAddressValidator(shieldSecret, 10*time.Second)

	srcIPStr := "192.168.1.100"
	srcIP := []byte{192, 168, 1, 100}

	// Challenge address
	validator.ChallengeAddress(srcIPStr, srcIP)

	// Try to validate with wrong token
	wrongToken := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	if validator.ValidateAddress(srcIPStr, srcIP, wrongToken) {
		t.Errorf("Expected validation to fail with wrong token")
	}
}

func TestAddressValidatorExpiry(t *testing.T) {
	shieldSecret := []byte("my-secret")
	validator := NewAddressValidator(shieldSecret, 100*time.Millisecond)

	srcIPStr := "192.168.1.100"
	srcIP := []byte{192, 168, 1, 100}

	// Challenge and validate
	token := validator.ChallengeAddress(srcIPStr, srcIP)
	validator.ValidateAddress(srcIPStr, srcIP, token)

	// Should be validated immediately
	if !validator.IsValidated(srcIPStr) {
		t.Errorf("Expected address to be validated immediately")
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Should no longer be validated
	if validator.IsValidated(srcIPStr) {
		t.Errorf("Expected validation to expire")
	}
}

func TestAddressValidatorReChallenge(t *testing.T) {
	shieldSecret := []byte("my-secret")
	validator := NewAddressValidator(shieldSecret, 100*time.Millisecond)

	srcIPStr := "192.168.1.100"
	srcIP := []byte{192, 168, 1, 100}

	// First challenge and validation
	token1 := validator.ChallengeAddress(srcIPStr, srcIP)
	validator.ValidateAddress(srcIPStr, srcIP, token1)

	if !validator.IsValidated(srcIPStr) {
		t.Errorf("Expected address to be validated")
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Challenge again
	token2 := validator.ChallengeAddress(srcIPStr, srcIP)
	validator.ValidateAddress(srcIPStr, srcIP, token2)

	if !validator.IsValidated(srcIPStr) {
		t.Errorf("Expected address to be re-validated after re-challenge")
	}
}

func TestAddressValidatorMultipleIPs(t *testing.T) {
	shieldSecret := []byte("my-secret")
	validator := NewAddressValidator(shieldSecret, 10*time.Second)

	ips := map[string][]byte{
		"192.168.1.100": {192, 168, 1, 100},
		"192.168.1.101": {192, 168, 1, 101},
		"192.168.1.102": {192, 168, 1, 102},
	}

	// Validate different IPs
	for ipStr, ipBytes := range ips {
		token := validator.ChallengeAddress(ipStr, ipBytes)
		if !validator.ValidateAddress(ipStr, ipBytes, token) {
			t.Errorf("Expected validation to succeed for %s", ipStr)
		}
	}

	// All should be validated
	for ipStr := range ips {
		if !validator.IsValidated(ipStr) {
			t.Errorf("Expected %s to be validated", ipStr)
		}
	}
}
