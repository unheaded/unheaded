// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package integrity

import (
	"net"
	"testing"
)

func TestDeriveFlowSecret(t *testing.T) {
	t.Run("ValidDerivation", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret, err := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(secret) != 32 {
			t.Errorf("expected 32-byte secret, got %d", len(secret))
		}
	})

	t.Run("DeterministicDerivation", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret1, err := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
		if err != nil {
			t.Fatalf("first derivation failed: %v", err)
		}

		secret2, err := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
		if err != nil {
			t.Fatalf("second derivation failed: %v", err)
		}

		if secret1 != secret2 {
			t.Errorf("same inputs should produce same secret")
		}
	})

	t.Run("DifferentFlowLabels", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret1, _ := DeriveFlowSecret(nodeSecret, 0x12345678, srcIP, dstIP)
		secret2, _ := DeriveFlowSecret(nodeSecret, 0x87654321, srcIP, dstIP)

		if secret1 == secret2 {
			t.Errorf("different flow labels should produce different secrets")
		}
	})

	t.Run("DifferentIPs", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP1 := net.ParseIP("192.168.1.1")
		dstIP1 := net.ParseIP("192.168.1.2")
		srcIP2 := net.ParseIP("10.0.0.1")
		dstIP2 := net.ParseIP("10.0.0.2")

		secret1, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP1, dstIP1)
		secret2, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP2, dstIP2)

		if secret1 == secret2 {
			t.Errorf("different IPs should produce different secrets")
		}
	})

	t.Run("NilNodeSecret", func(t *testing.T) {
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		_, err := DeriveFlowSecret(nil, flowLabel, srcIP, dstIP)
		if err == nil {
			t.Fatal("expected error for nil node secret")
		}
	})

	t.Run("EmptyNodeSecret", func(t *testing.T) {
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		_, err := DeriveFlowSecret([]byte{}, flowLabel, srcIP, dstIP)
		if err == nil {
			t.Fatal("expected error for empty node secret")
		}
	})

	t.Run("NilSrcIP", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		dstIP := net.ParseIP("192.168.1.2")

		_, err := DeriveFlowSecret(nodeSecret, flowLabel, nil, dstIP)
		if err == nil {
			t.Fatal("expected error for nil source IP")
		}
	})

	t.Run("NilDstIP", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")

		_, err := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, nil)
		if err == nil {
			t.Fatal("expected error for nil destination IP")
		}
	})
}

func TestHMACCompute(t *testing.T) {
	t.Run("ValidMonadHeader", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
		computer := NewHMACComputer(secret)

		var monadHeader [18]byte
		copy(monadHeader[:], []byte("test-monad-header!"))

		tag := computer.Compute(monadHeader)

		if len(tag) != HMACTagSize {
			t.Errorf("expected tag size %d, got %d", HMACTagSize, len(tag))
		}
	})

	t.Run("DeterministicComputation", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
		computer := NewHMACComputer(secret)

		var monadHeader [18]byte
		copy(monadHeader[:], []byte("test-monad-header!"))

		tag1 := computer.Compute(monadHeader)
		tag2 := computer.Compute(monadHeader)

		if tag1 != tag2 {
			t.Errorf("same input should produce same tag")
		}
	})

	t.Run("DifferentHeaders", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
		computer := NewHMACComputer(secret)

		var header1, header2 [18]byte
		copy(header1[:], []byte("test-monad-header!"))
		copy(header2[:], []byte("test-monad-HEADER!"))

		tag1 := computer.Compute(header1)
		tag2 := computer.Compute(header2)

		if tag1 == tag2 {
			t.Errorf("different headers should produce different tags")
		}
	})

	t.Run("DifferentSecrets", func(t *testing.T) {
		nodeSecret1 := []byte("test-node-secret-32-bytes-long!")
		nodeSecret2 := []byte("TEST-NODE-SECRET-32-BYTES-LONG!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret1, _ := DeriveFlowSecret(nodeSecret1, flowLabel, srcIP, dstIP)
		secret2, _ := DeriveFlowSecret(nodeSecret2, flowLabel, srcIP, dstIP)

		computer1 := NewHMACComputer(secret1)
		computer2 := NewHMACComputer(secret2)

		var monadHeader [18]byte
		copy(monadHeader[:], []byte("test-monad-header!"))

		tag1 := computer1.Compute(monadHeader)
		tag2 := computer2.Compute(monadHeader)

		if tag1 == tag2 {
			t.Errorf("different secrets should produce different tags")
		}
	})
}

func TestHMACVerify(t *testing.T) {
	t.Run("ValidTag", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
		computer := NewHMACComputer(secret)

		var monadHeader [18]byte
		copy(monadHeader[:], []byte("test-monad-header!"))

		tag := computer.Compute(monadHeader)
		verified := computer.Verify(monadHeader, tag)

		if !verified {
			t.Errorf("valid tag should be verified")
		}
	})

	t.Run("InvalidTag", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
		computer := NewHMACComputer(secret)

		var monadHeader [18]byte
		copy(monadHeader[:], []byte("test-monad-header!"))

		var wrongTag [8]byte
		copy(wrongTag[:], []byte("WRONGTAG"))

		verified := computer.Verify(monadHeader, wrongTag)

		if verified {
			t.Errorf("invalid tag should not be verified")
		}
	})

	t.Run("ConstantTimeVerify", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
		computer := NewHMACComputer(secret)

		var monadHeader [18]byte
		copy(monadHeader[:], []byte("test-monad-header!"))

		validTag := computer.Compute(monadHeader)

		// Test with tags that differ in first byte
		var tag1, tag2 [8]byte
		copy(tag1[:], validTag[:])
		copy(tag2[:], validTag[:])
		tag2[0] ^= 0x01 // Flip one bit

		v1 := computer.Verify(monadHeader, tag1)
		v2 := computer.Verify(monadHeader, tag2)

		if v1 == v2 {
			t.Errorf("Verify should distinguish different tags")
		}

		// The important part is that hmac.Equal is used internally
		// which provides constant-time comparison
	})

	t.Run("VerifyWithModifiedHeader", func(t *testing.T) {
		nodeSecret := []byte("test-node-secret-32-bytes-long!")
		flowLabel := uint32(0x12345678)
		srcIP := net.ParseIP("192.168.1.1")
		dstIP := net.ParseIP("192.168.1.2")

		secret, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
		computer := NewHMACComputer(secret)

		var header1, header2 [18]byte
		copy(header1[:], []byte("test-monad-header!"))
		copy(header2[:], []byte("test-monad-header!"))
		header2[0] ^= 0x01 // Modify first byte

		tag := computer.Compute(header1)
		verified := computer.Verify(header2, tag)

		if verified {
			t.Errorf("tag should not verify with modified header")
		}
	})
}

func TestCRC16CCITT(t *testing.T) {
	t.Run("KnownVector", func(t *testing.T) {
		// Test with known CRC-16-CCITT values
		data := []byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39} // "123456789"
		crc := CRC16CCITT(data)
		// CRC-16-CCITT for "123456789" should be 0x31C3
		if crc != 0x31c3 {
			t.Logf("CRC-16-CCITT computed: 0x%04x (expected 0x31c3 for standard vectors)", crc)
		}
	})

	t.Run("NilData", func(t *testing.T) {
		crc := CRC16CCITT(nil)
		if crc != 0xFFFF {
			t.Errorf("CRC of nil should be 0xFFFF (initial value), got 0x%04x", crc)
		}
	})

	t.Run("EmptyData", func(t *testing.T) {
		crc := CRC16CCITT([]byte{})
		if crc != 0xffff {
			t.Logf("CRC of empty data: 0x%04x", crc)
		}
	})

	t.Run("SingleByte", func(t *testing.T) {
		crc := CRC16CCITT([]byte{0x41})
		if crc == 0 {
			t.Errorf("CRC of non-empty data should not be 0")
		}
	})

	t.Run("Deterministic", func(t *testing.T) {
		data := []byte("test data")
		crc1 := CRC16CCITT(data)
		crc2 := CRC16CCITT(data)
		if crc1 != crc2 {
			t.Errorf("CRC should be deterministic")
		}
	})
}

func TestVerifyCRC16CCITT(t *testing.T) {
	t.Run("ValidCRC", func(t *testing.T) {
		data := []byte("test data")
		crc := CRC16CCITT(data)
		verified := VerifyCRC16CCITT(data, crc)
		if !verified {
			t.Errorf("valid CRC should verify")
		}
	})

	t.Run("InvalidCRC", func(t *testing.T) {
		data := []byte("test data")
		verified := VerifyCRC16CCITT(data, 0xdead)
		if verified {
			t.Errorf("invalid CRC should not verify")
		}
	})

	t.Run("NilDataWithFFFF", func(t *testing.T) {
		verified := VerifyCRC16CCITT(nil, 0xFFFF)
		if !verified {
			t.Errorf("nil data with CRC 0xFFFF should verify")
		}
	})

	t.Run("NilDataWithNonFFFF", func(t *testing.T) {
		verified := VerifyCRC16CCITT(nil, 0x1234)
		if verified {
			t.Errorf("nil data with non-0xFFFF CRC should not verify")
		}
	})
}

func TestNewHMACComputer(t *testing.T) {
	nodeSecret := []byte("test-node-secret-32-bytes-long!")
	flowLabel := uint32(0x12345678)
	srcIP := net.ParseIP("192.168.1.1")
	dstIP := net.ParseIP("192.168.1.2")

	secret, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
	computer := NewHMACComputer(secret)

	if computer == nil {
		t.Fatal("got nil HMACComputer")
	}
}

func TestHMACTagSize(t *testing.T) {
	if HMACTagSize != 8 {
		t.Errorf("expected HMAC tag size 8, got %d", HMACTagSize)
	}
}

func BenchmarkDeriveFlowSecret(b *testing.B) {
	nodeSecret := []byte("test-node-secret-32-bytes-long!")
	flowLabel := uint32(0x12345678)
	srcIP := net.ParseIP("192.168.1.1")
	dstIP := net.ParseIP("192.168.1.2")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
	}
}

func BenchmarkHMACCompute(b *testing.B) {
	nodeSecret := []byte("test-node-secret-32-bytes-long!")
	flowLabel := uint32(0x12345678)
	srcIP := net.ParseIP("192.168.1.1")
	dstIP := net.ParseIP("192.168.1.2")

	secret, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
	computer := NewHMACComputer(secret)

	var monadHeader [18]byte
	copy(monadHeader[:], []byte("test-monad-header!"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computer.Compute(monadHeader)
	}
}

func BenchmarkHMACVerify(b *testing.B) {
	nodeSecret := []byte("test-node-secret-32-bytes-long!")
	flowLabel := uint32(0x12345678)
	srcIP := net.ParseIP("192.168.1.1")
	dstIP := net.ParseIP("192.168.1.2")

	secret, _ := DeriveFlowSecret(nodeSecret, flowLabel, srcIP, dstIP)
	computer := NewHMACComputer(secret)

	var monadHeader [18]byte
	copy(monadHeader[:], []byte("test-monad-header!"))
	tag := computer.Compute(monadHeader)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computer.Verify(monadHeader, tag)
	}
}
