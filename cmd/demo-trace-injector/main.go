// SPDX-License-Identifier: GPL-3.0-or-later
//
// demo-trace-injector — publishes realistic synthetic eBPF trace events to the
// Wotan topics the dashboard ingestor consumes (ebpf.flow.events /
// ebpf.latency.events / ebpf.packet.events), so the Flow Graph, Latency, and
// Events dashboard tabs show live data for a demo/README GIF when the real XDP
// trace pipeline isn't running on this host.
//
// Schemas mirror cmd/dashboard-backend/internal/ebpf/types.go exactly.
// Throwaway demo tool — safe to delete.
package main

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"time"

	wotan "unheaded/pkg/wotan-client"
)

type traceID struct {
	High uint64 `json:"high"`
	Low  uint64 `json:"low"`
}
type flowKey struct {
	SrcAddr  string `json:"src_addr"`
	DstAddr  string `json:"dst_addr"`
	SrcPort  uint16 `json:"src_port"`
	DstPort  uint16 `json:"dst_port"`
	Protocol uint8  `json:"protocol"`
}
type flowState struct {
	TraceID    traceID `json:"trace_id"`
	StartNs    uint64  `json:"start_ns"`
	LastSeenNs uint64  `json:"last_seen_ns"`
	PacketsIn  uint64  `json:"packets_in"`
	PacketsOut uint64  `json:"packets_out"`
	BytesIn    uint64  `json:"bytes_in"`
	BytesOut   uint64  `json:"bytes_out"`
	State      string  `json:"state"`
}
type flowEvent struct {
	TimestampNs uint64    `json:"timestamp_ns"`
	FlowKey     flowKey   `json:"flow_key"`
	FlowState   flowState `json:"flow_state"`
	EventType   string    `json:"event_type"`
}
type meshMeta struct {
	Version       uint8    `json:"version"`
	SrcServiceID  uint8    `json:"src_service_id"`
	DstServiceID  uint8    `json:"dst_service_id"`
	HopCount      uint8    `json:"hop_count"`
	FlowFlags     []string `json:"flow_flags"`
	TraceHash     string   `json:"trace_hash"`
	QosClass      uint16   `json:"qos_class"`
	NatType       string   `json:"nat_type"`
	LatencyHintNs uint32   `json:"latency_hint_ns"`
}
type packetEvent struct {
	TimestampNs uint64    `json:"timestamp_ns"`
	TraceID     traceID   `json:"trace_id"`
	FlowKey     flowKey   `json:"flow_key"`
	PacketLen   uint32    `json:"packet_len"`
	Action      string    `json:"action"`
	Direction   string    `json:"direction"`
	Mesh        *meshMeta `json:"mesh,omitempty"`
}
type latencyEvent struct {
	TimestampNs uint64  `json:"timestamp_ns"`
	TraceID     traceID `json:"trace_id"`
	PID         uint32  `json:"pid"`
	TID         uint32  `json:"tid"`
	LatencyNs   uint64  `json:"latency_ns"`
	Operation   string  `json:"operation"`
}

type svc struct {
	name string
	ip   string
	port uint16
	id   uint8
}

var services = []svc{
	{"monad", "10.10.10.20", 19004, 1},
	{"sophia", "10.10.10.21", 19005, 2},
	{"wotan", "10.10.10.10", 18001, 3},
	{"dashboard", "10.10.10.30", 20000, 4},
	{"kanban", "10.10.10.31", 20001, 5},
	{"timeguru", "10.10.10.22", 19000, 6},
	{"captain", "10.10.10.23", 19002, 7},
	{"architect", "10.10.10.24", 19001, 8},
}

// latency profile per op: base + jitter (nanoseconds)
var latOps = []struct {
	op         string
	base, span uint64
}{
	{"tcp_connect", 800_000, 2_400_000}, // 0.8–3.2 ms
	{"tcp_accept", 60_000, 180_000},     // 60–240 µs
	{"tcp_send", 120_000, 500_000},      // 0.12–0.62 ms
	{"tcp_recv", 150_000, 600_000},      // 0.15–0.75 ms
}

func newTID() traceID { return traceID{High: rand.Uint64(), Low: rand.Uint64()} } // #nosec G404 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)

func main() {
	c, err := wotan.NewClientWithGRPC("localhost:18000", "localhost:18001")
	if err != nil {
		log.Fatalf("wotan client: %v", err)
	}
	ctx := context.Background()
	topics := []string{"ebpf.flow.events", "ebpf.latency.events", "ebpf.packet.events"}
	for _, t := range topics {
		if _, err := c.Subscribe(ctx, t, "trace-collector"); err != nil {
			log.Printf("subscribe %s: %v", t, err)
		}
	}
	log.Printf("demo-trace-injector: publishing to %v", topics)

	start := uint64(time.Now().UnixNano())
	var pubs int
	for {
		now := uint64(time.Now().UnixNano())

		// ── Flows: several live service-to-service connections ──
		for i := 0; i < 14; i++ {
			a := services[rand.Intn(len(services))] // #nosec G404 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
			b := services[rand.Intn(len(services))] // #nosec G404 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
			if a.name == b.name {
				continue
			}
			tid := newTID()
			fe := flowEvent{
				TimestampNs: now,
				// Deterministic src port per service pair keeps the flow-key
				// cardinality bounded (<=56 for 8 services) so the dashboard's
				// flow map stays small instead of growing toward MaxFlows (65536)
				// and OOMing. Random per-event ports created ~unbounded flows.
				FlowKey: flowKey{a.ip, b.ip, uint16(32000) + uint16(a.id)*8 + uint16(b.id), b.port, 6},
				FlowState: flowState{
					TraceID: tid, StartNs: start, LastSeenNs: now,
					PacketsIn:  uint64(rand.Intn(4000) + 200),     // #nosec G404,G115 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
					PacketsOut: uint64(rand.Intn(4000) + 200),     // #nosec G404,G115 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
					BytesIn:    uint64(rand.Intn(900000) + 20000), // #nosec G404,G115 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
					BytesOut:   uint64(rand.Intn(900000) + 20000), // #nosec G404,G115 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
					State:      "established",
				},
				EventType: "update",
			}
			publish(ctx, c, "ebpf.flow.events", fe)

			// A couple of packet events per flow (feeds Events + packet counters).
			for p := 0; p < 2; p++ {
				dir := "ingress"
				if p%2 == 1 {
					dir = "egress"
				}
				pe := packetEvent{
					TimestampNs: now, TraceID: tid,
					FlowKey:   fe.FlowKey,
					PacketLen: uint32(rand.Intn(1400) + 60), // #nosec G404,G115 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
					Action:    "pass", Direction: dir,
					Mesh: &meshMeta{
						Version: 1, SrcServiceID: a.id, DstServiceID: b.id,
						HopCount: uint8(1 + rand.Intn(2)), FlowFlags: []string{"traced"}, // #nosec G404,G115 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
						TraceHash: "0xUNHD", QosClass: 0, NatType: "none",
						LatencyHintNs: uint32(rand.Intn(400000) + 50000), // #nosec G404,G115 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
					},
				}
				publish(ctx, c, "ebpf.packet.events", pe)
			}
			pubs++
		}

		// ── Latency samples across the four TCP operations ──
		for _, lo := range latOps {
			for s := 0; s < 8; s++ {
				le := latencyEvent{
					TimestampNs: now, TraceID: newTID(),
					PID: uint32(1000 + rand.Intn(9000)), TID: uint32(1000 + rand.Intn(9000)), // #nosec G404,G115 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
					LatencyNs: lo.base + uint64(rand.Int63n(int64(lo.span))), // #nosec G404,G115 -- synthetic demo traffic; production trace IDs use UUIDv7 (pkg/trace)
					Operation: lo.op,
				}
				publish(ctx, c, "ebpf.latency.events", le)
			}
		}

		if pubs%140 == 0 {
			log.Printf("demo-trace-injector: alive, ~%d flow batches published", pubs)
		}
		time.Sleep(1 * time.Second)
	}
}

func publish(ctx context.Context, c *wotan.Client, topic string, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := c.Publish(ctx, topic, b); err != nil {
		log.Printf("publish %s: %v", topic, err)
	}
}
