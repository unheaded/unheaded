package dashboard

// WSEbpfHandler receives TelemetryEvent messages from Busboy's gRPC stream
// and broadcasts them to connected WebSocket clients for real-time visualization.
//
// Canvas mapping: Each TelemetryEvent maps to a flow arrow on the packet_flow canvas
// - source_pod → source node
// - dest_pod → dest node
// - verdict → arrow color (green=allow, red=deny, yellow=drop)
// - latency_ns → arrow thickness
//
// This is a scaffold — full implementation in Developer skill session
