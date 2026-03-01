/* SPDX-License-Identifier: GPL-2.0-only */
/* AF_XDP Bridge — C header for Go/C FFI
 * Part of: Unheaded Kingdom project
 *
 * Provides opaque handle to Rust AF_XDP engine.
 * Build: link against libaf_xdp.a (static) or libaf_xdp.so (dynamic).
 */

#ifndef AF_XDP_H
#define AF_XDP_H

#include <stdint.h>

/* ── Error codes ─────────────────────────────────────────────────────────── */

#define AFXDP_OK               0
#define AFXDP_ERR_INVALID_ARGS -1
#define AFXDP_ERR_NO_MEMORY    -2
#define AFXDP_ERR_SOCKET       -3
#define AFXDP_ERR_UMEM         -4

/* ── Types ───────────────────────────────────────────────────────────────── */

typedef struct {
    void *engine;
} afxdp_handle_t;

typedef struct {
    uint64_t rx_packets;
    uint64_t tx_packets;
    uint64_t rx_bytes;
    uint64_t tx_bytes;
    uint64_t rx_drops;
    uint64_t fill_empty;
    uint64_t comp_full;
} afxdp_stats_t;

/* ── Functions ───────────────────────────────────────────────────────────── */

#ifdef __cplusplus
extern "C" {
#endif

/**
 * Create AF_XDP engine on interface and queue.
 * @param iface      NIC interface name (e.g., "eth0")
 * @param queue      RX queue ID
 * @param frame_count Number of UMEM frames (0 = default 4096)
 * @return Handle pointer, or NULL on error.
 */
afxdp_handle_t *afxdp_create(
    const char *iface,
    unsigned int queue,
    unsigned int frame_count
);

/**
 * Receive packets into buffer.
 * @param handle     Engine handle
 * @param buf        Output buffer for packet data
 * @param buf_size   Buffer capacity in bytes
 * @param batch_size Maximum packets to receive
 * @return Bytes written to buf, or negative error code.
 */
int afxdp_recv(
    afxdp_handle_t *handle,
    char *buf,
    unsigned int buf_size,
    unsigned int batch_size
);

/**
 * Send packet from buffer.
 * @param handle     Engine handle
 * @param buf        Packet data to send
 * @param buf_size   Packet size in bytes
 * @return Number of packets sent (0 or 1), or negative error code.
 */
int afxdp_send(
    afxdp_handle_t *handle,
    const char *buf,
    unsigned int buf_size
);

/**
 * Get statistics snapshot.
 * @param handle     Engine handle
 * @param stats      Output statistics structure
 * @return 0 on success, negative error code on failure.
 */
int afxdp_stats(
    afxdp_handle_t *handle,
    afxdp_stats_t *stats
);

/**
 * Poll for RX readiness.
 * @param handle     Engine handle
 * @param timeout_ms Timeout in milliseconds (-1 = block)
 * @return 1 if ready, 0 if timeout, negative on error.
 */
int afxdp_poll(
    afxdp_handle_t *handle,
    int timeout_ms
);

/**
 * Destroy engine and free all resources.
 * @param handle     Engine handle (invalid after this call)
 * @return 0 on success, negative error code on failure.
 */
int afxdp_destroy(afxdp_handle_t *handle);

#ifdef __cplusplus
}
#endif

#endif /* AF_XDP_H */
