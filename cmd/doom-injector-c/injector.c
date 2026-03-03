// SPDX-License-Identifier: MIT
// Fast Doom packet injector — raw AF_PACKET, minimal overhead.
// Sends pre-built IPv6+HBH+Monad "CPU tick" packets at maximum speed.
//
// Usage: sudo nsenter --net=/var/run/netns/monad0 \
//          ./injector veth01 <src-mac> <dst-mac> [count] [delay_us]
//
// With delay_us=0 and CPU pinning (taskset -c 0), this can hit 50K+ pkt/s
// on a single core, vs ~900 pkt/s from the Go injector.

#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <time.h>
#include <signal.h>
#include <errno.h>
#include <sys/socket.h>
#include <sys/ioctl.h>
#include <net/if.h>
#include <linux/if_packet.h>
#include <linux/if_ether.h>
#include <arpa/inet.h>

static volatile int running = 1;

static void sighandler(int sig) {
    (void)sig;
    running = 0;
}

static int parse_mac(const char *str, unsigned char mac[6]) {
    return sscanf(str, "%hhx:%hhx:%hhx:%hhx:%hhx:%hhx",
                  &mac[0], &mac[1], &mac[2], &mac[3], &mac[4], &mac[5]) == 6 ? 0 : -1;
}

int main(int argc, char *argv[]) {
    if (argc < 4) {
        fprintf(stderr, "Usage: %s <iface> <src-mac> <dst-mac> [count] [delay_us]\n", argv[0]);
        return 1;
    }

    const char *iface = argv[1];
    unsigned char src_mac[6], dst_mac[6];
    if (parse_mac(argv[2], src_mac) || parse_mac(argv[3], dst_mac)) {
        fprintf(stderr, "Invalid MAC address format (expected xx:xx:xx:xx:xx:xx)\n");
        return 1;
    }

    long long count = (argc > 4) ? atoll(argv[4]) : 1000000000LL;
    long delay_us = (argc > 5) ? atol(argv[5]) : 0;

    signal(SIGINT, sighandler);
    signal(SIGTERM, sighandler);

    // Open raw AF_PACKET socket
    int sock = socket(AF_PACKET, SOCK_RAW, htons(ETH_P_IPV6));
    if (sock < 0) {
        perror("socket");
        return 1;
    }

    // Get interface index
    struct ifreq ifr;
    memset(&ifr, 0, sizeof(ifr));
    strncpy(ifr.ifr_name, iface, IFNAMSIZ - 1);
    if (ioctl(sock, SIOCGIFINDEX, &ifr) < 0) {
        perror("ioctl SIOCGIFINDEX");
        close(sock);
        return 1;
    }
    int ifindex = ifr.ifr_ifindex;

    // Bind to interface
    struct sockaddr_ll sll;
    memset(&sll, 0, sizeof(sll));
    sll.sll_family = AF_PACKET;
    sll.sll_protocol = htons(ETH_P_IPV6);
    sll.sll_ifindex = ifindex;
    if (bind(sock, (struct sockaddr *)&sll, sizeof(sll)) < 0) {
        perror("bind");
        close(sock);
        return 1;
    }

    // ── Build the packet once ────────────────────────────────────────────────
    // Ethernet(14) + IPv6(40) + HBH(24) = 78 bytes total
    unsigned char pkt[78];
    memset(pkt, 0, sizeof(pkt));

    // Ethernet header
    memcpy(pkt + 0, dst_mac, 6);
    memcpy(pkt + 6, src_mac, 6);
    pkt[12] = 0x86;
    pkt[13] = 0xDD;

    // IPv6 header (40 bytes at offset 14)
    unsigned char *ip6 = pkt + 14;
    // Version(4) + TC(8) + Flow Label(20) = 0x600000DE
    ip6[0] = 0x60;
    ip6[1] = 0x00;
    ip6[2] = 0x00;
    ip6[3] = 0xDE;  // Flow label low byte = instance_id
    // Payload length = 24 (HBH extension)
    ip6[4] = 0x00;
    ip6[5] = 0x18;
    // Next header = 0 (Hop-by-Hop)
    ip6[6] = 0x00;
    // Hop limit = 255
    ip6[7] = 0xFF;
    // Source: fd00:003f:0075:0000::0001
    ip6[8]  = 0xfd; ip6[9]  = 0x00; ip6[10] = 0x00; ip6[11] = 0x3f;
    ip6[12] = 0x00; ip6[13] = 0x75; ip6[14] = 0x00; ip6[15] = 0x00;
    ip6[16] = 0x00; ip6[17] = 0x00; ip6[18] = 0x00; ip6[19] = 0x00;
    ip6[20] = 0x00; ip6[21] = 0x00; ip6[22] = 0x00; ip6[23] = 0x01;
    // Destination: fd00:dead::0001
    ip6[24] = 0xfd; ip6[25] = 0x00; ip6[26] = 0xde; ip6[27] = 0xad;
    ip6[28] = 0x00; ip6[29] = 0x00; ip6[30] = 0x00; ip6[31] = 0x00;
    ip6[32] = 0x00; ip6[33] = 0x00; ip6[34] = 0x00; ip6[35] = 0x00;
    ip6[36] = 0x00; ip6[37] = 0x00; ip6[38] = 0x00; ip6[39] = 0x01;

    // HBH Options header (24 bytes at offset 54)
    unsigned char *hbh = pkt + 54;
    hbh[0] = 59;   // Next header = No Next Header
    hbh[1] = 2;    // Hdr ext len = 2 (24 total bytes)
    hbh[2] = 0x3E; // Opt type = MONAD_OPT_TYPE
    hbh[3] = 20;   // Opt data len = 20 bytes

    // Monad (20 bytes at offset 58)
    unsigned char *monad = pkt + 58;
    monad[0] = 0x01;  // version
    monad[7] = 0x02;  // flags = CUSTOM

    // ── Inject loop ──────────────────────────────────────────────────────────
    struct sockaddr_ll dst;
    memset(&dst, 0, sizeof(dst));
    dst.sll_family = AF_PACKET;
    dst.sll_protocol = htons(ETH_P_IPV6);
    dst.sll_ifindex = ifindex;
    dst.sll_halen = 6;
    memcpy(dst.sll_addr, dst_mac, 6);

    struct timespec delay_ts = {0, delay_us * 1000L};

    long long sent = 0;
    long long errors = 0;
    struct timespec start, now;
    clock_gettime(CLOCK_MONOTONIC, &start);

    long long report_interval = 100000;
    long long next_report = report_interval;

    fprintf(stderr, "Injecting %lld packets on %s (delay=%ldus)\n",
            count, iface, delay_us);

    while (running && sent < count) {
        ssize_t n = sendto(sock, pkt, sizeof(pkt), 0,
                           (struct sockaddr *)&dst, sizeof(dst));
        if (n < 0) {
            if (errno == ENOBUFS || errno == EAGAIN) {
                // Socket buffer full — retry immediately
                continue;
            }
            errors++;
            if (errors > 100) {
                perror("sendto");
                break;
            }
            continue;
        }
        sent++;

        if (delay_us > 0) {
            nanosleep(&delay_ts, NULL);
        }

        if (sent >= next_report) {
            clock_gettime(CLOCK_MONOTONIC, &now);
            double elapsed = (now.tv_sec - start.tv_sec) +
                             (now.tv_nsec - start.tv_nsec) / 1e9;
            double pps = sent / elapsed;
            fprintf(stderr, "\r  %lld pkts sent (%.0f pkt/s, %.1fM insn/s)  ",
                    sent, pps, pps * 256.0 / 1e6);
            next_report = sent + report_interval;
        }
    }

    clock_gettime(CLOCK_MONOTONIC, &now);
    double elapsed = (now.tv_sec - start.tv_sec) +
                     (now.tv_nsec - start.tv_nsec) / 1e9;
    double pps = (elapsed > 0) ? sent / elapsed : 0;

    fprintf(stderr, "\nDone: %lld packets in %.1fs (%.0f pkt/s, %.1fM insn/s, %lld errors)\n",
            sent, elapsed, pps, pps * 256.0 / 1e6, errors);

    close(sock);
    return 0;
}
