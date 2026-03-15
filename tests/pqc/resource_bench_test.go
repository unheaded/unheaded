// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Resource utilization and max transmission speed benchmarks for the PQC
// subsystem. Designed as scientific experiments with hypotheses, systematic
// measurement, and custom metric reporting.
//
// Run with:
//
//	go test ./tests/pqc/... -bench='Benchmark(Max|Concurrency|Step|Memory|Sustained|Transmission|KEM.*Rate|PolicyEngine|WotanState)' \
//	  -benchtime=1s -timeout=10m -run='^$' -v
package pqc_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/crypto/pqc"
	"unheaded/pkg/monad"
	pqcPolicy "unheaded/pkg/pqc_policy"
	"unheaded/pkg/wotan"
	"unheaded/services/shield"
)

// ---------------------------------------------------------------------------
// Experiment 1: Max Packet Verification Throughput (Parallel)
//
// Hypothesis: Cached verification (no crypto) should exceed 1M packets/sec
// on a modern multi-core CPU. Full crypto verification is bounded by ML-DSA
// verify cost (~100µs) and should achieve ~10K-50K packets/sec with
// parallelism.
// ---------------------------------------------------------------------------

func BenchmarkMaxVerificationThroughput(b *testing.B) {
	b.Run("Cached", func(b *testing.B) {
		v := shield.NewPQCVerifier()
		// No crypto verify function — tests cached fast path only.

		// Pre-load signature and key entries for N flows.
		const numFlows = 64
		for i := uint32(0); i < numFlows; i++ {
			sig := make([]byte, 3309)
			h := sha256.Sum256(sig)
			ph := buildBenchPseudoHeader(i, uint16(i), [4]byte(h[:4]), 0)
			v.LoadSignature(i, &shield.SigEntry{
				AlgoID:    0x02,
				ParamSet:  0x02,
				Tier:      0x01,
				Signature: sig,
				CreatedAt: time.Now(),
			})
			v.LoadKey(uint16(i), &shield.KeyEntry{
				AlgoID:    0x02,
				ParamSet:  0x02,
				TrustTier: 0x01,
				Status:    0x01,
				PublicKey: ph,
			})
		}

		// Pre-compute a zeroed signature's hash for all entries.
		zeroSig := make([]byte, 3309)
		zeroHash := sha256.Sum256(zeroSig)
		hashPfx := [4]byte(zeroHash[:4])

		var goroutineCounter int64
		b.ReportAllocs()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			// Each goroutine gets its own flow AND unique SrcSvcID to avoid
			// replay window mutex contention (flowKey = "SrcSvcID-DstSvcID").
			gID := atomic.AddInt64(&goroutineCounter, 1)
			myFlow := uint32(gID % numFlows)
			mySrc := uint16(1000 + gID) // Unique per goroutine → unique flowKey.
			seq := uint16(1)
			for pb.Next() {
				pkt := &shield.PQCPacketInfo{
					Flags:        monad.FlagPQCActive,
					SigRef:       myFlow,
					KeyRef:       uint16(myFlow),
					HashPfx:      hashPfx,
					SeqNum:       seq,
					SrcSvcID:     mySrc,
					DstSvcID:     101,
					PseudoHeader: buildBenchPseudoHeader(myFlow, uint16(myFlow), hashPfx, seq),
				}
				v.Verify(pkt)
				seq++
				if seq == 0 {
					seq = 1 // Avoid wrap-around replay rejection.
				}
			}
		})
	})

	b.Run("FullCrypto", func(b *testing.B) {
		kp, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
		if err != nil {
			b.Fatalf("keygen: %v", err)
		}

		v := shield.NewPQCVerifier()
		v.SetCryptoVerify(realCryptoVerify)

		var ops int64
		var goroutineCounter2 int64
		b.ReportAllocs()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			seq := uint16(1)
			goroutineID := atomic.AddInt64(&goroutineCounter2, 1)
			sigBase := uint32(goroutineID * 100000)
			for pb.Next() {
				sigRef := sigBase + uint32(seq)
				sp := makeSignedPacket(signer, kp.PublicKey, sigRef, 0, seq, 100, 101)
				loadIntoVerifier(v, sp, 0x02, 0x02, 0x01, 0x01)
				v.Verify(sp.Pkt)
				seq++
				if seq == 0 {
					seq = 1
				}
				atomic.AddInt64(&ops, 1)
			}
		})

		b.StopTimer()
		totalOps := atomic.LoadInt64(&ops)
		b.ReportMetric(float64(totalOps)/b.Elapsed().Seconds(), "packets/sec")
	})
}

// ---------------------------------------------------------------------------
// Experiment 2: Goroutine Concurrency Scaling
//
// Hypothesis: Cached verification throughput should scale near-linearly up to
// GOMAXPROCS, then plateau. Lock contention in the replay tracker
// (sync.Mutex) is the expected bottleneck.
// ---------------------------------------------------------------------------

func BenchmarkConcurrencyScaling(b *testing.B) {
	levels := []int{1, 2, 4, 8, 12}
	if runtime.NumCPU() >= 16 {
		levels = append(levels, 16, 32)
	}

	for _, level := range levels {
		b.Run(fmt.Sprintf("goroutines-%d", level), func(b *testing.B) {
			v := shield.NewPQCVerifier()

			// Pre-load entries.
			const numEntries = 128
			for i := uint32(0); i < numEntries; i++ {
				sig := make([]byte, 3309)
				h := sha256.Sum256(sig)
				v.LoadSignature(i, &shield.SigEntry{
					AlgoID: 0x02, ParamSet: 0x02, Tier: 0x01,
					Signature: sig, CreatedAt: time.Now(),
				})
				v.LoadKey(uint16(i%65536), &shield.KeyEntry{
					AlgoID: 0x02, ParamSet: 0x02, TrustTier: 0x01,
					Status:    0x01,
					PublicKey: buildBenchPseudoHeader(i, uint16(i), [4]byte(h[:4]), 0),
				})
			}

			b.ReportAllocs()
			b.SetParallelism(level)
			b.ResetTimer()

			var concCounter int64
		b.RunParallel(func(pb *testing.PB) {
				flowID := atomic.AddInt64(&concCounter, 1)
				seq := uint16(1)
				sigRef := uint32(flowID % numEntries)
				keyRef := uint16(sigRef)
				mySrc := uint16(2000 + flowID)
				sig := make([]byte, 3309)
				h := sha256.Sum256(sig)

				for pb.Next() {
					pkt := &shield.PQCPacketInfo{
						Flags:        monad.FlagPQCActive,
						SigRef:       sigRef,
						KeyRef:       keyRef,
						HashPfx:      [4]byte(h[:4]),
						SeqNum:       seq,
						SrcSvcID:     mySrc,
						DstSvcID:     101,
						PseudoHeader: buildBenchPseudoHeader(sigRef, keyRef, [4]byte(h[:4]), seq),
					}
					v.Verify(pkt)
					seq++
					if seq == 0 {
						seq = 1
					}
				}
			})
		})
	}
}

// ---------------------------------------------------------------------------
// Experiment 3: Per-Step Cost Breakdown (7-Point Pipeline)
//
// Hypothesis: Step 5 (cryptographic verify) dominates at ~100µs. All other
// steps should be <1µs. Steps 1 (flag check) and 6 (HashPfx) are the
// cheapest at <10ns.
// ---------------------------------------------------------------------------

func BenchmarkVerificationStepCosts(b *testing.B) {
	// Step 1: Flag check — single byte bitmask test.
	b.Run("Step1_FlagCheck", func(b *testing.B) {
		flags := byte(monad.FlagPQCActive)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = monad.IsPQCActive(flags)
		}
	})

	// Step 2: SigRef lookup — map access.
	b.Run("Step2_SigRefLookup", func(b *testing.B) {
		v := shield.NewPQCVerifier()
		for i := uint32(0); i < 1000; i++ {
			v.LoadSignature(i, &shield.SigEntry{
				AlgoID: 0x02, ParamSet: 0x02, Tier: 0x01,
				Signature: make([]byte, 3309), CreatedAt: time.Now(),
			})
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Verify triggers SigRef lookup at step 2.
			// We use a packet that passes step 1 but we measure the full
			// path — isolating step 2 requires the lookup itself.
			pkt := &shield.PQCPacketInfo{
				Flags:  monad.FlagPQCActive,
				SigRef: uint32(i % 1000),
				KeyRef: 0,
				SeqNum: uint16(i),
			}
			v.Verify(pkt) // Will fail at step 3 (no key) — measures steps 1+2+3 fail
		}
	})

	// Step 5: Cryptographic verify — the expensive step.
	b.Run("Step5_CryptoVerify_MLDSA65", func(b *testing.B) {
		kp, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
		if err != nil {
			b.Fatalf("keygen: %v", err)
		}
		message := make([]byte, 52)
		sig, err := signer.Sign(message)
		if err != nil {
			b.Fatalf("sign: %v", err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			realCryptoVerify(0x02, message, sig, kp.PublicKey)
		}
	})

	// Step 6: HashPfx match — SHA-256 prefix comparison.
	b.Run("Step6_HashPfxMatch", func(b *testing.B) {
		sig := make([]byte, 3309)
		h := sha256.Sum256(sig)
		expected := [4]byte(h[:4])
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			actual := sha256.Sum256(sig)
			_ = expected == [4]byte(actual[:4])
		}
	})

	// Step 7: SeqNum replay check — bitmap window.
	b.Run("Step7_SeqNumReplay", func(b *testing.B) {
		w := shield.NewPacketBitmapWindow()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w.Check("bench-flow", uint16(i))
		}
	})
}

// ---------------------------------------------------------------------------
// Experiment 4: Memory Footprint Per Flow
//
// Hypothesis: Each tracked flow in PacketBitmapWindow costs ~80 bytes
// (string key + bitmapEntry struct + map overhead). SecurityHardening
// wraps multiple trackers, so per-flow cost should be ~200-300 bytes.
// ---------------------------------------------------------------------------

func BenchmarkMemoryPerFlow(b *testing.B) {
	flowCounts := []int{100, 1000, 10000}

	b.Run("ReplayWindow", func(b *testing.B) {
		for _, n := range flowCounts {
			b.Run(fmt.Sprintf("flows-%d", n), func(b *testing.B) {
				b.ReportAllocs()
				for iter := 0; iter < b.N; iter++ {
					b.StopTimer()
					runtime.GC()
					var before runtime.MemStats
					runtime.ReadMemStats(&before)

					w := shield.NewPacketBitmapWindow()
					b.StartTimer()

					for i := 0; i < n; i++ {
						w.Check(fmt.Sprintf("flow-%d", i), 1)
					}

					b.StopTimer()
					var after runtime.MemStats
					runtime.ReadMemStats(&after)
					heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
					if heapGrowth > 0 {
						b.ReportMetric(float64(heapGrowth)/float64(n), "bytes/flow")
					}
					b.StartTimer()
				}
			})
		}
	})

	b.Run("TOCTOUCache", func(b *testing.B) {
		for _, n := range flowCounts {
			b.Run(fmt.Sprintf("entries-%d", n), func(b *testing.B) {
				b.ReportAllocs()
				for iter := 0; iter < b.N; iter++ {
					b.StopTimer()
					runtime.GC()
					var before runtime.MemStats
					runtime.ReadMemStats(&before)

					cache := shield.NewTOCTOUCache(30 * time.Second)
					b.StartTimer()

					for i := 0; i < n; i++ {
						cache.Pin(fmt.Sprintf("flow-%d", i), uint32(i))
					}

					b.StopTimer()
					var after runtime.MemStats
					runtime.ReadMemStats(&after)
					heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
					if heapGrowth > 0 {
						b.ReportMetric(float64(heapGrowth)/float64(n), "bytes/entry")
					}
					b.StartTimer()
				}
			})
		}
	})

	b.Run("SecurityHardening", func(b *testing.B) {
		for _, n := range flowCounts {
			b.Run(fmt.Sprintf("flows-%d", n), func(b *testing.B) {
				b.ReportAllocs()
				for iter := 0; iter < b.N; iter++ {
					b.StopTimer()
					runtime.GC()
					var before runtime.MemStats
					runtime.ReadMemStats(&before)

					sh := shield.NewSecurityHardening()
					b.StartTimer()

					for i := 0; i < n; i++ {
						pkt := &shield.PQCPacketInfo{
							Flags:  monad.FlagPQCActive,
							SigRef: uint32(i),
							KeyRef: uint16(i % 65536),
							SeqNum: 1,
						}
						sh.Validate(pkt, fmt.Sprintf("flow-%d", i))
					}

					b.StopTimer()
					var after runtime.MemStats
					runtime.ReadMemStats(&after)
					heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
					if heapGrowth > 0 {
						b.ReportMetric(float64(heapGrowth)/float64(n), "bytes/flow")
					}
					b.StartTimer()
				}
			})
		}
	})
}

// ---------------------------------------------------------------------------
// Experiment 5: Sustained Load Memory Pressure
//
// Hypothesis: Under sustained verification (100K packets), heap growth
// should be bounded — no per-packet leak. GC cycles should stabilize.
// ---------------------------------------------------------------------------

func BenchmarkSustainedLoadMemory(b *testing.B) {
	const totalPackets = 100_000
	const numFlows = 256

	b.Run("100K_Packets", func(b *testing.B) {
		b.ReportAllocs()

		for iter := 0; iter < b.N; iter++ {
			b.StopTimer()
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)

			v := shield.NewPQCVerifier()
			// Pre-load entries for all flows.
			for f := uint32(0); f < numFlows; f++ {
				sig := make([]byte, 3309)
				h := sha256.Sum256(sig)
				v.LoadSignature(f, &shield.SigEntry{
					AlgoID: 0x02, ParamSet: 0x02, Tier: 0x01,
					Signature: sig, CreatedAt: time.Now(),
				})
				v.LoadKey(uint16(f), &shield.KeyEntry{
					AlgoID: 0x02, ParamSet: 0x02, TrustTier: 0x01,
					Status:    0x01,
					PublicKey: buildBenchPseudoHeader(f, uint16(f), [4]byte(h[:4]), 0),
				})
			}

			b.StartTimer()

			for i := 0; i < totalPackets; i++ {
				flow := uint32(i % numFlows)
				seq := uint16(i/numFlows + 1)
				sig := make([]byte, 3309)
				h := sha256.Sum256(sig)
				pkt := &shield.PQCPacketInfo{
					Flags:        monad.FlagPQCActive,
					SigRef:       flow,
					KeyRef:       uint16(flow),
					HashPfx:      [4]byte(h[:4]),
					SeqNum:       seq,
					SrcSvcID:     100,
					DstSvcID:     101,
					PseudoHeader: buildBenchPseudoHeader(flow, uint16(flow), [4]byte(h[:4]), seq),
				}
				v.Verify(pkt)
			}

			b.StopTimer()

			var after runtime.MemStats
			runtime.ReadMemStats(&after)

			heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			gcCycles := after.NumGC - before.NumGC
			allocTotal := after.TotalAlloc - before.TotalAlloc

			b.ReportMetric(float64(heapGrowth), "heap_growth_bytes")
			b.ReportMetric(float64(gcCycles), "gc_cycles")
			b.ReportMetric(float64(allocTotal), "alloc_total_bytes")
			b.ReportMetric(float64(allocTotal)/float64(totalPackets), "bytes/packet")
			b.StartTimer()
		}
	})
}

// ---------------------------------------------------------------------------
// Experiment 6: Wire Format Transmission Speed
//
// Hypothesis: PQCValue marshal/unmarshal operates at memory bandwidth —
// expected >1 GB/s for 12-byte operations. PseudoHeader build at >100 MB/s.
// Full pipeline (marshal + verify + unmarshal) bounded by verification cost.
// ---------------------------------------------------------------------------

func BenchmarkTransmissionSpeed(b *testing.B) {
	b.Run("PQCValue_Marshal", func(b *testing.B) {
		v := monad.PQCValue{SigRef: 42, KeyRef: 7, HashPfx: [4]byte{0xDE, 0xAD, 0xBE, 0xEF}, SeqNum: 100}
		b.SetBytes(monad.PQCValueSize)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = v.Marshal()
		}
	})

	b.Run("PQCValue_Unmarshal", func(b *testing.B) {
		v := monad.PQCValue{SigRef: 42, KeyRef: 7, HashPfx: [4]byte{0xDE, 0xAD, 0xBE, 0xEF}, SeqNum: 100}
		buf := v.Marshal()
		b.SetBytes(monad.PQCValueSize)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			monad.UnmarshalPQCValue(buf[:])
		}
	})

	b.Run("PseudoHeader_Build", func(b *testing.B) {
		srcIP := [16]byte{0x20, 0x01, 0x0d, 0xb8}
		dstIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
		monadPfx := [8]byte{0x01, 0x0A, 0x0B, 0x00, 0x03, 0x01, 0x01, monad.FlagPQCActive}
		pqcVal := monad.PQCValue{SigRef: 1, KeyRef: 1, SeqNum: 1}
		pqcValBuf := pqcVal.Marshal()
		b.SetBytes(monad.PseudoHeaderSize)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ph := monad.BuildPseudoHeader(srcIP, dstIP, monadPfx, pqcValBuf)
			_ = ph.Marshal()
		}
	})

	b.Run("PQCValue_RoundTrip", func(b *testing.B) {
		v := monad.PQCValue{SigRef: 42, KeyRef: 7, HashPfx: [4]byte{0xDE, 0xAD, 0xBE, 0xEF}, SeqNum: 100}
		b.SetBytes(monad.PQCValueSize * 2) // marshal + unmarshal
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := v.Marshal()
			monad.UnmarshalPQCValue(buf[:])
		}
	})

	b.Run("FullPipeline_12BytePacket", func(b *testing.B) {
		// Marshal PQCValue → Build PseudoHeader → SHA-256 HashPfx → SeqNum check.
		// Represents per-packet wire format processing cost without crypto.
		w := shield.NewPacketBitmapWindow()
		srcIP := [16]byte{0x20, 0x01, 0x0d, 0xb8}
		dstIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
		monadPfx := [8]byte{0x01, 0x0A, 0x0B, 0x00, 0x03, 0x01, 0x01, monad.FlagPQCActive}

		b.SetBytes(monad.PQCValueSize + monad.PseudoHeaderSize) // 12 + 52 = 64 bytes processed
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := monad.PQCValue{SigRef: 1, KeyRef: 1, HashPfx: [4]byte{0xAA, 0xBB, 0xCC, 0xDD}, SeqNum: uint16(i)}
			buf := v.Marshal()
			monad.UnmarshalPQCValue(buf[:])
			ph := monad.BuildPseudoHeader(srcIP, dstIP, monadPfx, buf)
			_ = ph.Marshal()
			w.Check("pipeline-flow", uint16(i))
		}
	})
}

// ---------------------------------------------------------------------------
// Experiment 7: KEM Tunnel Establishment Rate
//
// Hypothesis: ML-KEM-768 tunnel establishment should achieve >1000
// tunnels/sec sequentially and >5000 tunnels/sec with parallelism,
// bounded by KEM keygen + encap + decap + HKDF.
// ---------------------------------------------------------------------------

func BenchmarkKEMTunnelEstablishmentRate(b *testing.B) {
	params := []struct {
		name  string
		param pqc.ParameterSet
	}{
		{"ML-KEM-512", pqc.MLKEM512},
		{"ML-KEM-768", pqc.MLKEM768},
		{"ML-KEM-1024", pqc.MLKEM1024},
	}

	for _, p := range params {
		b.Run(p.name+"/Sequential", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				neg, err := pqc.NewTunnelNegotiator(p.param)
				if err != nil {
					b.Fatalf("NewTunnelNegotiator: %v", err)
				}

				kp, err := pqc.GenerateMLKEMKeyPair(p.param)
				if err != nil {
					b.Fatalf("keygen: %v", err)
				}

				initResult, err := neg.InitiatorHandshake(kp.PublicKey, nil, "bench")
				if err != nil {
					b.Fatalf("initiator: %v", err)
				}

				respResult, err := neg.ResponderHandshake(
					initResult.Ciphertext, kp.SecretKey, nil, "bench",
				)
				if err != nil {
					b.Fatalf("responder: %v", err)
				}

				initResult.Zero()
				respResult.Zero()
			}
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "tunnels/sec")
		})

		b.Run(p.name+"/Parallel", func(b *testing.B) {
			b.ReportAllocs()
			var ops int64
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					neg, err := pqc.NewTunnelNegotiator(p.param)
					if err != nil {
						b.Fatalf("NewTunnelNegotiator: %v", err)
					}

					kp, err := pqc.GenerateMLKEMKeyPair(p.param)
					if err != nil {
						b.Fatalf("keygen: %v", err)
					}

					initResult, err := neg.InitiatorHandshake(kp.PublicKey, nil, "bench")
					if err != nil {
						b.Fatalf("initiator: %v", err)
					}

					respResult, err := neg.ResponderHandshake(
						initResult.Ciphertext, kp.SecretKey, nil, "bench",
					)
					if err != nil {
						b.Fatalf("responder: %v", err)
					}

					initResult.Zero()
					respResult.Zero()
					atomic.AddInt64(&ops, 1)
				}
			})

			b.StopTimer()
			totalOps := atomic.LoadInt64(&ops)
			b.ReportMetric(float64(totalOps)/b.Elapsed().Seconds(), "tunnels/sec")
		})
	}
}

// ---------------------------------------------------------------------------
// Experiment 8: Policy Engine Under Load
//
// Hypothesis: Policy check is O(1) map lookup + constant 7-point check.
// Scaling from 10 → 1000 policies should not affect lookup latency.
// Parallel checks should scale linearly due to sync.RWMutex read-lock.
// ---------------------------------------------------------------------------

func BenchmarkPolicyEngineLoad(b *testing.B) {
	policyCounts := []int{10, 100, 1000}

	for _, n := range policyCounts {
		b.Run(fmt.Sprintf("policies-%d/Sequential", n), func(b *testing.B) {
			checker := pqcPolicy.NewChecker()
			for i := 0; i < n; i++ {
				checker.SetPolicy(&pqcPolicy.AppPolicy{
					ServiceID:    uint16(i),
					ServiceName:  fmt.Sprintf("service-%d", i),
					AllowedAlgos: []uint8{0x01, 0x02, 0x03},
					MinNISTLevel: pqcPolicy.NISTLevel2,
					MaxKeyAgeSec: 3600,
					RequirePQC:   true,
				})
			}

			ctx := context.Background()
			req := pqcPolicy.PolicyCheckRequest{
				ServiceID:    uint16(n / 2), // Hit middle of range.
				AlgoID:       0x02,
				ParamSet:     0x02,
				PQCVerified:  true,
				KeyCreatedAt: time.Now(),
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				checker.Check(ctx, req)
			}
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "checks/sec")
		})

		b.Run(fmt.Sprintf("policies-%d/Parallel", n), func(b *testing.B) {
			checker := pqcPolicy.NewChecker()
			for i := 0; i < n; i++ {
				checker.SetPolicy(&pqcPolicy.AppPolicy{
					ServiceID:    uint16(i),
					ServiceName:  fmt.Sprintf("service-%d", i),
					AllowedAlgos: []uint8{0x01, 0x02, 0x03},
					MinNISTLevel: pqcPolicy.NISTLevel2,
					MaxKeyAgeSec: 3600,
					RequirePQC:   true,
				})
			}

			ctx := context.Background()
			var ops int64
			var policyGoroutineCounter int64

			b.ReportAllocs()
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				localID := atomic.AddInt64(&policyGoroutineCounter, 1)
				svcID := uint16(localID % int64(n))
				for pb.Next() {
					checker.Check(ctx, pqcPolicy.PolicyCheckRequest{
						ServiceID:    svcID,
						AlgoID:       0x02,
						ParamSet:     0x02,
						PQCVerified:  true,
						KeyCreatedAt: time.Now(),
					})
					atomic.AddInt64(&ops, 1)
				}
			})

			b.StopTimer()
			totalOps := atomic.LoadInt64(&ops)
			b.ReportMetric(float64(totalOps)/b.Elapsed().Seconds(), "checks/sec")
		})
	}
}

// ---------------------------------------------------------------------------
// Experiment 9: Wotan State Contention
//
// Hypothesis: PQCStateManager uses sync.RWMutex. Read-heavy workloads
// (90% read) should scale near-linearly. Write-heavy workloads (90% write)
// will contend. CAS-only workloads reveal atomic retry cost.
// ---------------------------------------------------------------------------

func BenchmarkWotanStateContention(b *testing.B) {
	b.Run("ReadHeavy_90R_10W", func(b *testing.B) {
		mgr := wotan.NewPQCStateManager()
		mgr.Write(wotan.PQCState{
			SigCount: 100, KeyCount: 50, VerifyPass: 1000,
			ActiveTier: 0x01, PolicyMode: 0x02,
		})

		b.ReportAllocs()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%10 == 0 {
					// 10% writes.
					mgr.IncrVerifyPass()
				} else {
					// 90% reads.
					_ = mgr.Read()
				}
				i++
			}
		})
	})

	b.Run("WriteHeavy_10R_90W", func(b *testing.B) {
		mgr := wotan.NewPQCStateManager()

		b.ReportAllocs()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%10 == 0 {
					// 10% reads.
					_ = mgr.Read()
				} else {
					// 90% writes.
					mgr.IncrVerifyPass()
				}
				i++
			}
		})
	})

	b.Run("CAS_Contention", func(b *testing.B) {
		mgr := wotan.NewPQCStateManager()
		mgr.Write(wotan.PQCState{ActiveTier: 0x01})

		b.ReportAllocs()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				current := mgr.Read()
				desired := current
				desired.VerifyPass++
				mgr.CAS(current, desired)
			}
		})
	})

	b.Run("SingleThread_Baseline", func(b *testing.B) {
		mgr := wotan.NewPQCStateManager()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mgr.IncrVerifyPass()
			_ = mgr.Read()
		}
	})
}
