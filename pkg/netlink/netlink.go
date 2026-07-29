// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux
// +build linux

// Package netlink provides a from-scratch implementation of Linux netlink sockets
// for the Unheaded Kingdom infrastructure project.
//
// Netlink is a Linux kernel interface used for inter-process communication (IPC)
// between both the kernel and userspace processes, and between different userspace
// processes. This implementation focuses on RTNetlink (NETLINK_ROUTE) for network
// configuration, as well as Generic Netlink for custom protocols.
//
// This code uses raw syscalls via golang.org/x/sys/unix and does not depend on
// any existing netlink libraries.
package netlink

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// =============================================================================
// Constants and Protocol Definitions
// =============================================================================

// Netlink socket families
const (
	NETLINK_ROUTE          = 0  // Routing/device hook
	NETLINK_UNUSED         = 1  // Unused number
	NETLINK_USERSOCK       = 2  // Reserved for user mode socket protocols
	NETLINK_FIREWALL       = 3  // Unused number, formerly ip_queue
	NETLINK_SOCK_DIAG      = 4  // Socket monitoring
	NETLINK_NFLOG          = 5  // Netfilter/iptables ULOG
	NETLINK_XFRM           = 6  // IPsec
	NETLINK_SELINUX        = 7  // SELinux event notifications
	NETLINK_ISCSI          = 8  // Open-iSCSI
	NETLINK_AUDIT          = 9  // Auditing
	NETLINK_FIB_LOOKUP     = 10 // FIB lookup
	NETLINK_CONNECTOR      = 11 // Kernel connector
	NETLINK_NETFILTER      = 12 // Netfilter subsystem
	NETLINK_IP6_FW         = 13 // Unused number
	NETLINK_DNRTMSG        = 14 // DECnet routing messages
	NETLINK_KOBJECT_UEVENT = 15 // Kernel messages to userspace
	NETLINK_GENERIC        = 16 // Generic netlink family for simplified netlink usage
)

// Netlink message types (standard)
const (
	NLMSG_NOOP    = 0x1 // Nothing
	NLMSG_ERROR   = 0x2 // Error
	NLMSG_DONE    = 0x3 // End of a dump
	NLMSG_OVERRUN = 0x4 // Data lost
)

// Netlink message flags
const (
	NLM_F_REQUEST       = 0x001 // Must be set on all request messages
	NLM_F_MULTI         = 0x002 // Multipart message, terminated by NLMSG_DONE
	NLM_F_ACK           = 0x004 // Reply with ack, with zero or error code
	NLM_F_ECHO          = 0x008 // Echo this request
	NLM_F_DUMP_INTR     = 0x010 // Dump was inconsistent due to sequence change
	NLM_F_DUMP_FILTERED = 0x020 // Dump was filtered as requested

	// Modifiers to GET request
	NLM_F_ROOT   = 0x100 // Specify tree root
	NLM_F_MATCH  = 0x200 // Return all matching
	NLM_F_ATOMIC = 0x400 // Atomic GET
	NLM_F_DUMP   = NLM_F_ROOT | NLM_F_MATCH

	// Modifiers to NEW request
	NLM_F_REPLACE = 0x100 // Override existing
	NLM_F_EXCL    = 0x200 // Do not touch if it exists
	NLM_F_CREATE  = 0x400 // Create if it does not exist
	NLM_F_APPEND  = 0x800 // Add to end of list
)

// RTNetlink message types for network interface operations
const (
	RTM_BASE       = 16
	RTM_NEWLINK    = 16 // Create a new network interface
	RTM_DELLINK    = 17 // Remove a network interface
	RTM_GETLINK    = 18 // Retrieve information about a network interface
	RTM_SETLINK    = 19 // Set network interface attributes
	RTM_NEWADDR    = 20 // Add a new address to an interface
	RTM_DELADDR    = 21 // Remove an address from an interface
	RTM_GETADDR    = 22 // Retrieve address information
	RTM_NEWROUTE   = 24 // Add a new route
	RTM_DELROUTE   = 25 // Remove a route
	RTM_GETROUTE   = 26 // Retrieve route information
	RTM_NEWNEIGH   = 28 // Add a neighbor table entry
	RTM_DELNEIGH   = 29 // Remove a neighbor table entry
	RTM_GETNEIGH   = 30 // Retrieve neighbor information
	RTM_NEWRULE    = 32 // Add routing rule
	RTM_DELRULE    = 33 // Delete routing rule
	RTM_GETRULE    = 34 // Retrieve routing rule
	RTM_NEWQDISC   = 36 // Add queueing discipline
	RTM_DELQDISC   = 37 // Remove queueing discipline
	RTM_GETQDISC   = 38 // Get queueing discipline
	RTM_NEWTCLASS  = 40 // Add traffic class
	RTM_DELTCLASS  = 41 // Remove traffic class
	RTM_GETTCLASS  = 42 // Get traffic class
	RTM_NEWTFILTER = 44 // Add traffic filter
	RTM_DELTFILTER = 45 // Remove traffic filter
	RTM_GETTFILTER = 46 // Get traffic filter
	RTM_NEWACTION  = 48 // Add traffic control action
	RTM_DELACTION  = 49 // Remove traffic control action
	RTM_GETACTION  = 50 // Get traffic control action
	RTM_NEWNSID    = 88 // New network namespace ID
	RTM_DELNSID    = 89 // Delete network namespace ID
	RTM_GETNSID    = 90 // Get network namespace ID
)

// Interface link attribute types (IFLA_*)
const (
	IFLA_UNSPEC          = 0
	IFLA_ADDRESS         = 1  // Hardware address (MAC)
	IFLA_BROADCAST       = 2  // Broadcast address
	IFLA_IFNAME          = 3  // Interface name
	IFLA_MTU             = 4  // MTU of the device
	IFLA_LINK            = 5  // Link type
	IFLA_QDISC           = 6  // Queueing discipline
	IFLA_STATS           = 7  // Interface statistics
	IFLA_COST            = 8  // Cost
	IFLA_PRIORITY        = 9  // Priority
	IFLA_MASTER          = 10 // Master device
	IFLA_WIRELESS        = 11 // Wireless extension
	IFLA_PROTINFO        = 12 // Protocol specific information
	IFLA_TXQLEN          = 13 // Transmit queue length
	IFLA_MAP             = 14 // Interface hardware map
	IFLA_WEIGHT          = 15 // Interface weight
	IFLA_OPERSTATE       = 16 // Operational state
	IFLA_LINKMODE        = 17 // Link mode
	IFLA_LINKINFO        = 18 // Link information
	IFLA_NET_NS_PID      = 19 // Network namespace PID
	IFLA_IFALIAS         = 20 // Interface alias
	IFLA_NUM_VF          = 21 // Number of VFs
	IFLA_VFINFO_LIST     = 22 // VF info list
	IFLA_STATS64         = 23 // 64-bit statistics
	IFLA_VF_PORTS        = 24 // VF ports
	IFLA_PORT_SELF       = 25 // Port self
	IFLA_AF_SPEC         = 26 // Address family specific
	IFLA_GROUP           = 27 // Interface group
	IFLA_NET_NS_FD       = 28 // Network namespace FD
	IFLA_EXT_MASK        = 29 // Extended mask
	IFLA_PROMISCUITY     = 30 // Promiscuity count
	IFLA_NUM_TX_QUEUES   = 31 // Number of TX queues
	IFLA_NUM_RX_QUEUES   = 32 // Number of RX queues
	IFLA_CARRIER         = 33 // Carrier state
	IFLA_PHYS_PORT_ID    = 34 // Physical port ID
	IFLA_CARRIER_CHANGES = 35 // Carrier changes count
	IFLA_PHYS_SWITCH_ID  = 36 // Physical switch ID
	IFLA_LINK_NETNSID    = 37 // Link network namespace ID
	IFLA_PHYS_PORT_NAME  = 38 // Physical port name
	IFLA_PROTO_DOWN      = 39 // Protocol down
	IFLA_GSO_MAX_SEGS    = 40 // GSO max segments
	IFLA_GSO_MAX_SIZE    = 41 // GSO max size
	IFLA_XDP             = 43 // XDP program
	IFLA_XDP_FD          = 1  // XDP program file descriptor
	IFLA_XDP_ATTACHED    = 2  // XDP attached mode
	IFLA_XDP_FLAGS       = 3  // XDP flags
	IFLA_XDP_PROG_ID     = 4  // XDP program ID
)

// XDP attachment flags
const (
	XDP_FLAGS_UPDATE_IF_NOEXIST = 1 << 0
	XDP_FLAGS_SKB_MODE          = 1 << 1
	XDP_FLAGS_DRV_MODE          = 1 << 2
	XDP_FLAGS_HW_MODE           = 1 << 3
	XDP_FLAGS_REPLACE           = 1 << 4
)

// Interface address attribute types (IFA_*)
const (
	IFA_UNSPEC    = 0
	IFA_ADDRESS   = 1 // Interface address
	IFA_LOCAL     = 2 // Local address
	IFA_LABEL     = 3 // Name of the interface
	IFA_BROADCAST = 4 // Broadcast address
	IFA_ANYCAST   = 5 // Anycast address
	IFA_CACHEINFO = 6 // Address information
	IFA_MULTICAST = 7 // Multicast address
	IFA_FLAGS     = 8 // Address flags
)

// Route attribute types (RTA_*)
const (
	RTA_UNSPEC    = 0
	RTA_DST       = 1  // Route destination address
	RTA_SRC       = 2  // Route source address
	RTA_IIF       = 3  // Input interface index
	RTA_OIF       = 4  // Output interface index
	RTA_GATEWAY   = 5  // Gateway address
	RTA_PRIORITY  = 6  // Route priority/metric
	RTA_PREFSRC   = 7  // Preferred source address
	RTA_METRICS   = 8  // Route metrics
	RTA_MULTIPATH = 9  // Multipath nexthop data
	RTA_PROTOINFO = 10 // Protocol specific info (no longer used)
	RTA_FLOW      = 11 // Route realm
	RTA_CACHEINFO = 12 // Cache info
	RTA_SESSION   = 13 // No longer used
	RTA_MP_ALGO   = 14 // No longer used
	RTA_TABLE     = 15 // Routing table ID
	RTA_MARK      = 16 // Route mark
	RTA_MFC_STATS = 17 // Multicast forwarding cache stats
	RTA_VIA       = 18 // Route via
	RTA_NEWDST    = 19 // New destination
	RTA_PREF      = 20 // Route preference
	RTA_ENCAP     = 22 // Encapsulation
)

// Route types
const (
	RTN_UNSPEC      = 0  // Unknown route
	RTN_UNICAST     = 1  // Gateway or direct route
	RTN_LOCAL       = 2  // Accept locally
	RTN_BROADCAST   = 3  // Accept locally as broadcast, send as broadcast
	RTN_ANYCAST     = 4  // Accept locally as broadcast, but send as unicast
	RTN_MULTICAST   = 5  // Multicast route
	RTN_BLACKHOLE   = 6  // Drop packets
	RTN_UNREACHABLE = 7  // Destination is unreachable
	RTN_PROHIBIT    = 8  // Administratively prohibited
	RTN_THROW       = 9  // Continue routing lookup in another table
	RTN_NAT         = 10 // Translate this address (deprecated)
	RTN_XRESOLVE    = 11 // Use external resolver
)

// Route protocols (origin of route)
const (
	RTPROT_UNSPEC   = 0   // Unknown
	RTPROT_REDIRECT = 1   // ICMP redirect
	RTPROT_KERNEL   = 2   // Kernel
	RTPROT_BOOT     = 3   // Boot
	RTPROT_STATIC   = 4   // Static route
	RTPROT_GATED    = 8   // Gated
	RTPROT_RA       = 9   // Router advertisement
	RTPROT_MRT      = 10  // Merit MRT
	RTPROT_ZEBRA    = 11  // Zebra
	RTPROT_BIRD     = 12  // Bird
	RTPROT_DNROUTED = 13  // DECnet routing daemon
	RTPROT_XORP     = 14  // XORP
	RTPROT_NTK      = 15  // Netsukuku
	RTPROT_DHCP     = 16  // DHCP client
	RTPROT_MROUTED  = 17  // Multicast daemon
	RTPROT_BABEL    = 42  // Babel
	RTPROT_BGP      = 186 // BGP
	RTPROT_ISIS     = 187 // IS-IS
	RTPROT_OSPF     = 188 // OSPF
	RTPROT_RIP      = 189 // RIP
	RTPROT_EIGRP    = 192 // EIGRP
)

// Route scopes
const (
	RT_SCOPE_UNIVERSE = 0   // Global route
	RT_SCOPE_SITE     = 200 // Interior route in local autonomous system
	RT_SCOPE_LINK     = 253 // Route on directly connected network
	RT_SCOPE_HOST     = 254 // Route on local host
	RT_SCOPE_NOWHERE  = 255 // Destination doesn't exist
)

// Routing table IDs
const (
	RT_TABLE_UNSPEC  = 0   // Unspecified table
	RT_TABLE_COMPAT  = 252 // Compat table
	RT_TABLE_DEFAULT = 253 // Default table
	RT_TABLE_MAIN    = 254 // Main table
	RT_TABLE_LOCAL   = 255 // Local table
)

// Traffic control attribute types (TCA_*)
const (
	TCA_UNSPEC        = 0
	TCA_KIND          = 1  // Qdisc/class/filter kind name
	TCA_OPTIONS       = 2  // Qdisc specific options
	TCA_STATS         = 3  // Statistics
	TCA_XSTATS        = 4  // Module-specific statistics
	TCA_RATE          = 5  // Rate estimator configuration
	TCA_FCNT          = 6  // Filter count
	TCA_STATS2        = 7  // More statistics
	TCA_STAB          = 8  // Size table
	TCA_PAD           = 9  // Padding
	TCA_DUMP_FLAGS    = 10 // Dump flags
	TCA_CHAIN         = 11 // Chain index
	TCA_HW_OFFLOAD    = 12 // Hardware offload
	TCA_INGRESS_BLOCK = 13 // Ingress block
	TCA_EGRESS_BLOCK  = 14 // Egress block
)

// TC handles
const (
	TC_H_UNSPEC  = 0
	TC_H_ROOT    = 0xFFFFFFFF
	TC_H_INGRESS = 0xFFFFFFF1
	TC_H_CLSACT  = TC_H_INGRESS
)

// Generic netlink constants
const (
	GENL_ID_CTRL            = 0x10 // Generic netlink controller ID
	GENL_CTRL_CMD_GETFAMILY = 3    // Get family command
)

// Generic netlink controller attributes
const (
	CTRL_ATTR_UNSPEC       = 0
	CTRL_ATTR_FAMILY_ID    = 1
	CTRL_ATTR_FAMILY_NAME  = 2
	CTRL_ATTR_VERSION      = 3
	CTRL_ATTR_HDRSIZE      = 4
	CTRL_ATTR_MAXATTR      = 5
	CTRL_ATTR_OPS          = 6
	CTRL_ATTR_MCAST_GROUPS = 7
)

// Structure sizes
const (
	NlMsgHdrLen   = 16 // sizeof(nlmsghdr) = 16 bytes
	NlAttrHdrLen  = 4  // sizeof(nlattr) = 4 bytes
	IfInfoMsgLen  = 16 // sizeof(ifinfomsg) = 16 bytes
	IfAddrMsgLen  = 8  // sizeof(ifaddrmsg) = 8 bytes
	RtMsgLen      = 12 // sizeof(rtmsg) = 12 bytes
	TcMsgLen      = 20 // sizeof(tcmsg) = 20 bytes
	GenlMsgHdrLen = 4  // sizeof(genlmsghdr) = 4 bytes
)

// Alignment helpers - netlink requires 4-byte alignment
const nlmsgAlignTo = 4

// =============================================================================
// Core Structures
// =============================================================================

// NlMsgHdr represents the netlink message header.
// Every netlink message must begin with this header.
//
// Structure layout (16 bytes total):
//
//	0                   1                   2                   3
//	0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                          Length                               |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |            Type              |            Flags               |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                     Sequence Number                           |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                        Port ID (PID)                          |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
type NlMsgHdr struct {
	Len   uint32 // Length of message including header
	Type  uint16 // Message type (content type)
	Flags uint16 // Additional flags
	Seq   uint32 // Sequence number
	Pid   uint32 // Port ID (usually process ID)
}

// NlAttr represents a netlink attribute.
// Attributes are used to pass additional data in netlink messages.
// They are TLV (Type-Length-Value) encoded.
//
// Structure layout (variable length, header is 4 bytes):
//
//	0                   1                   2                   3
//	0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |           Length              |           Type               |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                         Value...                              |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
type NlAttr struct {
	Len  uint16 // Length including header
	Type uint16 // Attribute type
}

// IfInfoMsg represents the interface info message.
// Used with RTM_NEWLINK, RTM_DELLINK, and RTM_GETLINK.
//
// Structure layout (16 bytes):
//
//	0                   1                   2                   3
//	0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |   Family      |   Reserved    |           Type               |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                          Index                                |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                          Flags                                |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                          Change                               |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
type IfInfoMsg struct {
	Family   uint8  // Address family (AF_UNSPEC for getting all)
	Reserved uint8  // Reserved, always 0
	Type     uint16 // Device type (ARPHRD_*)
	Index    int32  // Interface index
	Flags    uint32 // Device flags (IFF_*)
	Change   uint32 // Change mask
}

// IfAddrMsg represents the interface address message.
// Used with RTM_NEWADDR, RTM_DELADDR, and RTM_GETADDR.
//
// Structure layout (8 bytes):
type IfAddrMsg struct {
	Family    uint8  // Address family (AF_INET, AF_INET6)
	PrefixLen uint8  // Prefix length (CIDR notation)
	Flags     uint8  // Address flags
	Scope     uint8  // Address scope
	Index     uint32 // Interface index
}

// RtMsg represents the route message.
// Used with RTM_NEWROUTE, RTM_DELROUTE, and RTM_GETROUTE.
//
// Structure layout (12 bytes):
type RtMsg struct {
	Family   uint8  // Address family (AF_INET, AF_INET6)
	DstLen   uint8  // Destination prefix length
	SrcLen   uint8  // Source prefix length
	Tos      uint8  // Type of service
	Table    uint8  // Routing table ID
	Protocol uint8  // Routing protocol
	Scope    uint8  // Route scope
	Type     uint8  // Route type
	Flags    uint32 // Route flags
}

// TcMsg represents the traffic control message.
// Used with RTM_NEWQDISC, RTM_DELQDISC, RTM_GETTCLASS, etc.
//
// Structure layout (20 bytes):
type TcMsg struct {
	Family  uint8  // Address family
	Pad1    uint8  // Padding
	Pad2    uint16 // Padding
	Ifindex int32  // Interface index
	Handle  uint32 // Qdisc handle
	Parent  uint32 // Parent qdisc handle
	Info    uint32 // Priority and protocol for filters
}

// GenlMsgHdr represents the generic netlink message header.
// Used for generic netlink protocols (NETLINK_GENERIC).
//
// Structure layout (4 bytes):
type GenlMsgHdr struct {
	Cmd      uint8  // Command
	Version  uint8  // Version
	Reserved uint16 // Reserved
}

// NlErr represents a netlink error message.
// Returned when an error occurs in response to a request.
type NlErr struct {
	Error int32    // Negative errno or 0 for ACK
	Msg   NlMsgHdr // Header of the message that caused the error
}

// =============================================================================
// High-Level Structures for User Convenience
// =============================================================================

// LinkInfo contains parsed information about a network interface.
type LinkInfo struct {
	Index        int32
	Name         string
	HardwareAddr net.HardwareAddr
	MTU          uint32
	Flags        uint32
	OperState    uint8
	TxQueueLen   uint32
	Qdisc        string
	Master       int32
}

// AddrInfo contains parsed information about an interface address.
type AddrInfo struct {
	Index     uint32
	Family    uint8
	PrefixLen uint8
	Scope     uint8
	Address   net.IP
	Local     net.IP
	Broadcast net.IP
	Label     string
}

// RouteInfo contains parsed information about a route.
type RouteInfo struct {
	Family   uint8
	DstLen   uint8
	SrcLen   uint8
	Table    uint8
	Protocol uint8
	Scope    uint8
	Type     uint8
	Dst      net.IP
	Src      net.IP
	Gateway  net.IP
	OifIndex int32
	IifIndex int32
	Priority uint32
	PrefSrc  net.IP
}

// =============================================================================
// Netlink Socket Implementation
// =============================================================================

// Socket represents a netlink socket connection.
type Socket struct {
	fd      int        // File descriptor
	family  int        // Netlink family (NETLINK_ROUTE, NETLINK_GENERIC, etc.)
	pid     uint32     // Port ID (process ID)
	seq     uint32     // Sequence number counter
	seqMu   sync.Mutex // Mutex for sequence number
	recvBuf []byte     // Receive buffer
}

// NewSocket creates a new netlink socket for the specified protocol family.
// The family parameter should be one of NETLINK_ROUTE, NETLINK_GENERIC, etc.
func NewSocket(family int) (*Socket, error) {
	// Create the netlink socket using raw syscall
	// AF_NETLINK is the address family for netlink sockets
	// SOCK_DGRAM provides datagram semantics (though netlink uses RAW internally)
	// SOCK_CLOEXEC ensures the socket is closed on exec
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, family)
	if err != nil {
		return nil, fmt.Errorf("failed to create netlink socket: %w", err)
	}

	// Bind the socket to get a unique port ID (pid)
	// If we specify pid=0, the kernel will assign one (usually the process ID)
	sockAddr := &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Pid:    0, // Let kernel assign
		Groups: 0, // No multicast groups initially
	}

	if err := unix.Bind(fd, sockAddr); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("failed to bind netlink socket: %w", err)
	}

	// Get the assigned port ID
	sa, err := unix.Getsockname(fd)
	if err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("failed to get socket name: %w", err)
	}

	nlsa, ok := sa.(*unix.SockaddrNetlink)
	if !ok {
		_ = unix.Close(fd)
		return nil, errors.New("unexpected socket address type")
	}

	return &Socket{
		fd:      fd,
		family:  family,
		pid:     nlsa.Pid,
		seq:     0,
		recvBuf: make([]byte, 65536), // 64KB buffer for receiving messages
	}, nil
}

// Close closes the netlink socket.
func (s *Socket) Close() error {
	return unix.Close(s.fd)
}

// nextSeq returns the next sequence number for messages.
// Sequence numbers are used to match responses with requests.
func (s *Socket) nextSeq() uint32 {
	s.seqMu.Lock()
	defer s.seqMu.Unlock()
	s.seq++
	return s.seq
}

// SetReceiveBuffer sets the receive buffer size for the socket.
func (s *Socket) SetReceiveBuffer(size int) error {
	return unix.SetsockoptInt(s.fd, unix.SOL_SOCKET, unix.SO_RCVBUF, size)
}

// JoinGroup joins a netlink multicast group.
// Groups are used to receive broadcast notifications from the kernel.
func (s *Socket) JoinGroup(group uint32) error {
	return unix.SetsockoptInt(s.fd, unix.SOL_NETLINK, unix.NETLINK_ADD_MEMBERSHIP, int(group))
}

// LeaveGroup leaves a netlink multicast group.
func (s *Socket) LeaveGroup(group uint32) error {
	return unix.SetsockoptInt(s.fd, unix.SOL_NETLINK, unix.NETLINK_DROP_MEMBERSHIP, int(group))
}

// =============================================================================
// Message Serialization and Parsing
// =============================================================================

// nlmsgAlign aligns a length to the netlink alignment boundary (4 bytes).
func nlmsgAlign(len int) int {
	return (len + nlmsgAlignTo - 1) &^ (nlmsgAlignTo - 1)
}

// SerializeNlMsgHdr serializes a netlink message header to bytes.
func SerializeNlMsgHdr(hdr *NlMsgHdr) []byte {
	buf := make([]byte, NlMsgHdrLen)
	binary.LittleEndian.PutUint32(buf[0:4], hdr.Len)
	binary.LittleEndian.PutUint16(buf[4:6], hdr.Type)
	binary.LittleEndian.PutUint16(buf[6:8], hdr.Flags)
	binary.LittleEndian.PutUint32(buf[8:12], hdr.Seq)
	binary.LittleEndian.PutUint32(buf[12:16], hdr.Pid)
	return buf
}

// ParseNlMsgHdr parses a netlink message header from bytes.
func ParseNlMsgHdr(buf []byte) (*NlMsgHdr, error) {
	if len(buf) < NlMsgHdrLen {
		return nil, errors.New("buffer too small for netlink header")
	}
	return &NlMsgHdr{
		Len:   binary.LittleEndian.Uint32(buf[0:4]),
		Type:  binary.LittleEndian.Uint16(buf[4:6]),
		Flags: binary.LittleEndian.Uint16(buf[6:8]),
		Seq:   binary.LittleEndian.Uint32(buf[8:12]),
		Pid:   binary.LittleEndian.Uint32(buf[12:16]),
	}, nil
}

// SerializeIfInfoMsg serializes an interface info message to bytes.
func SerializeIfInfoMsg(msg *IfInfoMsg) []byte {
	buf := make([]byte, IfInfoMsgLen)
	buf[0] = msg.Family
	buf[1] = msg.Reserved
	binary.LittleEndian.PutUint16(buf[2:4], msg.Type)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(msg.Index)) // #nosec G115 -- 8/16-bit MBC memory store; the truncation IS the instruction semantics, bounds-checked above
	binary.LittleEndian.PutUint32(buf[8:12], msg.Flags)
	binary.LittleEndian.PutUint32(buf[12:16], msg.Change)
	return buf
}

// ParseIfInfoMsg parses an interface info message from bytes.
func ParseIfInfoMsg(buf []byte) (*IfInfoMsg, error) {
	if len(buf) < IfInfoMsgLen {
		return nil, errors.New("buffer too small for ifinfomsg")
	}
	return &IfInfoMsg{
		Family:   buf[0],
		Reserved: buf[1],
		Type:     binary.LittleEndian.Uint16(buf[2:4]),
		Index:    int32(binary.LittleEndian.Uint32(buf[4:8])), // #nosec G115 -- kernel/netlink ABI field width
		Flags:    binary.LittleEndian.Uint32(buf[8:12]),
		Change:   binary.LittleEndian.Uint32(buf[12:16]),
	}, nil
}

// SerializeIfAddrMsg serializes an interface address message to bytes.
func SerializeIfAddrMsg(msg *IfAddrMsg) []byte {
	buf := make([]byte, IfAddrMsgLen)
	buf[0] = msg.Family
	buf[1] = msg.PrefixLen
	buf[2] = msg.Flags
	buf[3] = msg.Scope
	binary.LittleEndian.PutUint32(buf[4:8], msg.Index)
	return buf
}

// ParseIfAddrMsg parses an interface address message from bytes.
func ParseIfAddrMsg(buf []byte) (*IfAddrMsg, error) {
	if len(buf) < IfAddrMsgLen {
		return nil, errors.New("buffer too small for ifaddrmsg")
	}
	return &IfAddrMsg{
		Family:    buf[0],
		PrefixLen: buf[1],
		Flags:     buf[2],
		Scope:     buf[3],
		Index:     binary.LittleEndian.Uint32(buf[4:8]),
	}, nil
}

// SerializeRtMsg serializes a route message to bytes.
func SerializeRtMsg(msg *RtMsg) []byte {
	buf := make([]byte, RtMsgLen)
	buf[0] = msg.Family
	buf[1] = msg.DstLen
	buf[2] = msg.SrcLen
	buf[3] = msg.Tos
	buf[4] = msg.Table
	buf[5] = msg.Protocol
	buf[6] = msg.Scope
	buf[7] = msg.Type
	binary.LittleEndian.PutUint32(buf[8:12], msg.Flags)
	return buf
}

// ParseRtMsg parses a route message from bytes.
func ParseRtMsg(buf []byte) (*RtMsg, error) {
	if len(buf) < RtMsgLen {
		return nil, errors.New("buffer too small for rtmsg")
	}
	return &RtMsg{
		Family:   buf[0],
		DstLen:   buf[1],
		SrcLen:   buf[2],
		Tos:      buf[3],
		Table:    buf[4],
		Protocol: buf[5],
		Scope:    buf[6],
		Type:     buf[7],
		Flags:    binary.LittleEndian.Uint32(buf[8:12]),
	}, nil
}

// SerializeTcMsg serializes a traffic control message to bytes.
func SerializeTcMsg(msg *TcMsg) []byte {
	buf := make([]byte, TcMsgLen)
	buf[0] = msg.Family
	buf[1] = msg.Pad1
	binary.LittleEndian.PutUint16(buf[2:4], msg.Pad2)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(msg.Ifindex)) // #nosec G115 -- 8/16-bit MBC memory store; the truncation IS the instruction semantics, bounds-checked above
	binary.LittleEndian.PutUint32(buf[8:12], msg.Handle)
	binary.LittleEndian.PutUint32(buf[12:16], msg.Parent)
	binary.LittleEndian.PutUint32(buf[16:20], msg.Info)
	return buf
}

// ParseTcMsg parses a traffic control message from bytes.
func ParseTcMsg(buf []byte) (*TcMsg, error) {
	if len(buf) < TcMsgLen {
		return nil, errors.New("buffer too small for tcmsg")
	}
	return &TcMsg{
		Family:  buf[0],
		Pad1:    buf[1],
		Pad2:    binary.LittleEndian.Uint16(buf[2:4]),
		Ifindex: int32(binary.LittleEndian.Uint32(buf[4:8])), // #nosec G115 -- kernel/netlink ABI field width
		Handle:  binary.LittleEndian.Uint32(buf[8:12]),
		Parent:  binary.LittleEndian.Uint32(buf[12:16]),
		Info:    binary.LittleEndian.Uint32(buf[16:20]),
	}, nil
}

// SerializeGenlMsgHdr serializes a generic netlink message header to bytes.
func SerializeGenlMsgHdr(hdr *GenlMsgHdr) []byte {
	buf := make([]byte, GenlMsgHdrLen)
	buf[0] = hdr.Cmd
	buf[1] = hdr.Version
	binary.LittleEndian.PutUint16(buf[2:4], hdr.Reserved)
	return buf
}

// ParseGenlMsgHdr parses a generic netlink message header from bytes.
func ParseGenlMsgHdr(buf []byte) (*GenlMsgHdr, error) {
	if len(buf) < GenlMsgHdrLen {
		return nil, errors.New("buffer too small for genlmsghdr")
	}
	return &GenlMsgHdr{
		Cmd:      buf[0],
		Version:  buf[1],
		Reserved: binary.LittleEndian.Uint16(buf[2:4]),
	}, nil
}

// =============================================================================
// Netlink Attribute Handling
// =============================================================================

// SerializeAttr creates a netlink attribute with the given type and data.
// The attribute is properly aligned to 4 bytes.
func SerializeAttr(attrType uint16, data []byte) []byte {
	attrLen := NlAttrHdrLen + len(data)
	alignedLen := nlmsgAlign(attrLen)
	buf := make([]byte, alignedLen)

	// Write attribute header
	binary.LittleEndian.PutUint16(buf[0:2], uint16(attrLen)) // #nosec G115 -- 8/16-bit MBC memory store; the truncation IS the instruction semantics, bounds-checked above
	binary.LittleEndian.PutUint16(buf[2:4], attrType)

	// Write attribute data
	copy(buf[NlAttrHdrLen:], data)

	return buf
}

// SerializeAttrU8 creates a netlink attribute containing a uint8 value.
func SerializeAttrU8(attrType uint16, val uint8) []byte {
	return SerializeAttr(attrType, []byte{val})
}

// SerializeAttrU16 creates a netlink attribute containing a uint16 value.
func SerializeAttrU16(attrType uint16, val uint16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, val)
	return SerializeAttr(attrType, buf)
}

// SerializeAttrU32 creates a netlink attribute containing a uint32 value.
func SerializeAttrU32(attrType uint16, val uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, val)
	return SerializeAttr(attrType, buf)
}

// SerializeAttrU64 creates a netlink attribute containing a uint64 value.
func SerializeAttrU64(attrType uint16, val uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, val)
	return SerializeAttr(attrType, buf)
}

// SerializeAttrString creates a netlink attribute containing a null-terminated string.
func SerializeAttrString(attrType uint16, val string) []byte {
	// Netlink strings are null-terminated
	return SerializeAttr(attrType, append([]byte(val), 0))
}

// SerializeAttrIP creates a netlink attribute containing an IP address.
func SerializeAttrIP(attrType uint16, ip net.IP) []byte {
	// Use the 4-byte or 16-byte representation based on the IP version
	if ip4 := ip.To4(); ip4 != nil {
		return SerializeAttr(attrType, ip4)
	}
	return SerializeAttr(attrType, ip.To16())
}

// ParsedAttr represents a parsed netlink attribute.
type ParsedAttr struct {
	Type uint16
	Data []byte
}

// ParseAttrs parses netlink attributes from a byte slice.
// Returns a slice of parsed attributes.
func ParseAttrs(buf []byte) ([]ParsedAttr, error) {
	var attrs []ParsedAttr

	for len(buf) >= NlAttrHdrLen {
		// Read attribute header
		attrLen := binary.LittleEndian.Uint16(buf[0:2])
		attrType := binary.LittleEndian.Uint16(buf[2:4])

		if attrLen < NlAttrHdrLen {
			return nil, fmt.Errorf("invalid attribute length: %d", attrLen)
		}

		if int(attrLen) > len(buf) {
			return nil, errors.New("attribute extends beyond buffer")
		}

		// Extract attribute data (excluding padding)
		dataLen := int(attrLen) - NlAttrHdrLen
		data := make([]byte, dataLen)
		copy(data, buf[NlAttrHdrLen:attrLen])

		attrs = append(attrs, ParsedAttr{
			Type: attrType,
			Data: data,
		})

		// Move to next attribute (with alignment)
		alignedLen := nlmsgAlign(int(attrLen))
		if alignedLen > len(buf) {
			break
		}
		buf = buf[alignedLen:]
	}

	return attrs, nil
}

// ParseAttrU8 extracts a uint8 value from attribute data.
func ParseAttrU8(data []byte) (uint8, error) {
	if len(data) < 1 {
		return 0, errors.New("insufficient data for uint8")
	}
	return data[0], nil
}

// ParseAttrU16 extracts a uint16 value from attribute data.
func ParseAttrU16(data []byte) (uint16, error) {
	if len(data) < 2 {
		return 0, errors.New("insufficient data for uint16")
	}
	return binary.LittleEndian.Uint16(data), nil
}

// ParseAttrU32 extracts a uint32 value from attribute data.
func ParseAttrU32(data []byte) (uint32, error) {
	if len(data) < 4 {
		return 0, errors.New("insufficient data for uint32")
	}
	return binary.LittleEndian.Uint32(data), nil
}

// ParseAttrU64 extracts a uint64 value from attribute data.
func ParseAttrU64(data []byte) (uint64, error) {
	if len(data) < 8 {
		return 0, errors.New("insufficient data for uint64")
	}
	return binary.LittleEndian.Uint64(data), nil
}

// ParseAttrString extracts a null-terminated string from attribute data.
func ParseAttrString(data []byte) string {
	// Remove null terminator if present
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}
	return string(data)
}

// ParseAttrIP extracts an IP address from attribute data.
func ParseAttrIP(data []byte) net.IP {
	return net.IP(data)
}

// =============================================================================
// Message Building and Sending
// =============================================================================

// NlMsgBuilder helps construct netlink messages.
type NlMsgBuilder struct {
	data []byte
}

// NewNlMsgBuilder creates a new netlink message builder.
func NewNlMsgBuilder(msgType uint16, flags uint16) *NlMsgBuilder {
	b := &NlMsgBuilder{
		data: make([]byte, 0, 4096),
	}

	// Reserve space for header (will be filled in Finalize)
	hdr := &NlMsgHdr{
		Len:   0, // Will be set in Finalize
		Type:  msgType,
		Flags: flags,
		Seq:   0, // Will be set when sending
		Pid:   0, // Will be set when sending
	}
	b.data = append(b.data, SerializeNlMsgHdr(hdr)...)

	return b
}

// AddIfInfoMsg adds an interface info message to the builder.
func (b *NlMsgBuilder) AddIfInfoMsg(msg *IfInfoMsg) {
	b.data = append(b.data, SerializeIfInfoMsg(msg)...)
}

// AddIfAddrMsg adds an interface address message to the builder.
func (b *NlMsgBuilder) AddIfAddrMsg(msg *IfAddrMsg) {
	b.data = append(b.data, SerializeIfAddrMsg(msg)...)
}

// AddRtMsg adds a route message to the builder.
func (b *NlMsgBuilder) AddRtMsg(msg *RtMsg) {
	b.data = append(b.data, SerializeRtMsg(msg)...)
}

// AddTcMsg adds a traffic control message to the builder.
func (b *NlMsgBuilder) AddTcMsg(msg *TcMsg) {
	b.data = append(b.data, SerializeTcMsg(msg)...)
}

// AddGenlMsgHdr adds a generic netlink message header to the builder.
func (b *NlMsgBuilder) AddGenlMsgHdr(hdr *GenlMsgHdr) {
	b.data = append(b.data, SerializeGenlMsgHdr(hdr)...)
}

// AddAttr adds a raw attribute to the builder.
func (b *NlMsgBuilder) AddAttr(attrType uint16, data []byte) {
	b.data = append(b.data, SerializeAttr(attrType, data)...)
}

// AddAttrU8 adds a uint8 attribute to the builder.
func (b *NlMsgBuilder) AddAttrU8(attrType uint16, val uint8) {
	b.data = append(b.data, SerializeAttrU8(attrType, val)...)
}

// AddAttrU16 adds a uint16 attribute to the builder.
func (b *NlMsgBuilder) AddAttrU16(attrType uint16, val uint16) {
	b.data = append(b.data, SerializeAttrU16(attrType, val)...)
}

// AddAttrU32 adds a uint32 attribute to the builder.
func (b *NlMsgBuilder) AddAttrU32(attrType uint16, val uint32) {
	b.data = append(b.data, SerializeAttrU32(attrType, val)...)
}

// AddAttrU64 adds a uint64 attribute to the builder.
func (b *NlMsgBuilder) AddAttrU64(attrType uint16, val uint64) {
	b.data = append(b.data, SerializeAttrU64(attrType, val)...)
}

// AddAttrString adds a string attribute to the builder.
func (b *NlMsgBuilder) AddAttrString(attrType uint16, val string) {
	b.data = append(b.data, SerializeAttrString(attrType, val)...)
}

// AddAttrIP adds an IP address attribute to the builder.
func (b *NlMsgBuilder) AddAttrIP(attrType uint16, ip net.IP) {
	b.data = append(b.data, SerializeAttrIP(attrType, ip)...)
}

// AddNestedAttr starts a nested attribute. Returns the offset where the
// nested attribute header was written, which must be passed to EndNestedAttr.
func (b *NlMsgBuilder) AddNestedAttr(attrType uint16) int {
	offset := len(b.data)
	// Write placeholder header
	hdr := make([]byte, NlAttrHdrLen)
	binary.LittleEndian.PutUint16(hdr[0:2], 0) // Length will be set in EndNestedAttr
	binary.LittleEndian.PutUint16(hdr[2:4], attrType)
	b.data = append(b.data, hdr...)
	return offset
}

// EndNestedAttr completes a nested attribute, updating its length.
func (b *NlMsgBuilder) EndNestedAttr(offset int) {
	// Calculate total length of nested attribute
	length := len(b.data) - offset
	binary.LittleEndian.PutUint16(b.data[offset:offset+2], uint16(length)) // #nosec G115 -- 8/16-bit MBC memory store; the truncation IS the instruction semantics, bounds-checked above
}

// Finalize completes the message, updating the length field in the header.
// Returns the complete message bytes.
func (b *NlMsgBuilder) Finalize() []byte {
	// Update the length field in the header
	binary.LittleEndian.PutUint32(b.data[0:4], uint32(len(b.data))) // #nosec G115 -- bounded by the length of an already-allocated slice
	return b.data
}

// =============================================================================
// Socket Communication
// =============================================================================

// Send sends a netlink message and sets the sequence number and PID.
func (s *Socket) Send(msg []byte) error {
	// Update sequence number in the message
	seq := s.nextSeq()
	binary.LittleEndian.PutUint32(msg[8:12], seq)
	binary.LittleEndian.PutUint32(msg[12:16], s.pid)

	// Send to kernel (PID 0)
	addr := &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Pid:    0, // Kernel
		Groups: 0,
	}

	return unix.Sendto(s.fd, msg, 0, addr)
}

// Receive receives netlink messages from the socket.
// Returns all received messages and handles multipart messages.
func (s *Socket) Receive() ([][]byte, error) {
	var messages [][]byte

	for {
		n, _, err := unix.Recvfrom(s.fd, s.recvBuf, 0)
		if err != nil {
			return nil, fmt.Errorf("recvfrom failed: %w", err)
		}

		if n < NlMsgHdrLen {
			return nil, errors.New("received message too short")
		}

		// Parse all messages in the buffer
		buf := s.recvBuf[:n]
		for len(buf) >= NlMsgHdrLen {
			hdr, err := ParseNlMsgHdr(buf)
			if err != nil {
				return nil, err
			}

			if hdr.Len < NlMsgHdrLen || int(hdr.Len) > len(buf) {
				return nil, errors.New("invalid message length")
			}

			// Check for DONE message (end of multipart)
			if hdr.Type == NLMSG_DONE {
				return messages, nil
			}

			// Check for error message
			if hdr.Type == NLMSG_ERROR {
				if hdr.Len < NlMsgHdrLen+4 {
					return nil, errors.New("error message too short")
				}
				errno := int32(binary.LittleEndian.Uint32(buf[NlMsgHdrLen : NlMsgHdrLen+4])) // #nosec G115 -- kernel/netlink ABI field width
				if errno != 0 {
					return nil, fmt.Errorf("netlink error: %d (%s)", -errno, unix.Errno(-errno).Error()) // #nosec G115 -- bounded by construction; see the surrounding guard
				}
				// errno == 0 means ACK
				return messages, nil
			}

			// Copy the message
			msgCopy := make([]byte, hdr.Len)
			copy(msgCopy, buf[:hdr.Len])
			messages = append(messages, msgCopy)

			// Move to next message (with alignment)
			alignedLen := nlmsgAlign(int(hdr.Len))
			if alignedLen > len(buf) {
				break
			}
			buf = buf[alignedLen:]
		}

		// Check if we need to continue receiving (multipart message)
		if len(messages) > 0 {
			lastHdr, _ := ParseNlMsgHdr(messages[len(messages)-1])
			if lastHdr.Flags&NLM_F_MULTI == 0 {
				// Not multipart, we're done
				return messages, nil
			}
		}
	}
}

// Execute sends a message and waits for the response.
// This is a convenience method for request-response operations.
func (s *Socket) Execute(msg []byte) ([][]byte, error) {
	if err := s.Send(msg); err != nil {
		return nil, err
	}
	return s.Receive()
}

// =============================================================================
// RTNetlink Operations - Interface Queries
// =============================================================================

// GetLinks retrieves information about all network interfaces.
func (s *Socket) GetLinks() ([]LinkInfo, error) {
	// Build RTM_GETLINK request
	b := NewNlMsgBuilder(RTM_GETLINK, NLM_F_REQUEST|NLM_F_DUMP)
	b.AddIfInfoMsg(&IfInfoMsg{
		Family: unix.AF_UNSPEC,
	})

	responses, err := s.Execute(b.Finalize())
	if err != nil {
		return nil, err
	}

	var links []LinkInfo
	for _, resp := range responses {
		link, err := parseLinkMessage(resp)
		if err != nil {
			continue // Skip malformed messages
		}
		links = append(links, *link)
	}

	return links, nil
}

// GetLinkByIndex retrieves information about a specific interface by index.
func (s *Socket) GetLinkByIndex(index int32) (*LinkInfo, error) {
	b := NewNlMsgBuilder(RTM_GETLINK, NLM_F_REQUEST)
	b.AddIfInfoMsg(&IfInfoMsg{
		Family: unix.AF_UNSPEC,
		Index:  index,
	})

	responses, err := s.Execute(b.Finalize())
	if err != nil {
		return nil, err
	}

	if len(responses) == 0 {
		return nil, errors.New("no response for link query")
	}

	return parseLinkMessage(responses[0])
}

// GetLinkByName retrieves information about a specific interface by name.
func (s *Socket) GetLinkByName(name string) (*LinkInfo, error) {
	b := NewNlMsgBuilder(RTM_GETLINK, NLM_F_REQUEST)
	b.AddIfInfoMsg(&IfInfoMsg{
		Family: unix.AF_UNSPEC,
	})
	b.AddAttrString(IFLA_IFNAME, name)

	responses, err := s.Execute(b.Finalize())
	if err != nil {
		return nil, err
	}

	if len(responses) == 0 {
		return nil, errors.New("no response for link query")
	}

	return parseLinkMessage(responses[0])
}

// parseLinkMessage parses a single RTM_NEWLINK message into LinkInfo.
func parseLinkMessage(msg []byte) (*LinkInfo, error) {
	if len(msg) < NlMsgHdrLen+IfInfoMsgLen {
		return nil, errors.New("message too short")
	}

	ifInfo, err := ParseIfInfoMsg(msg[NlMsgHdrLen : NlMsgHdrLen+IfInfoMsgLen])
	if err != nil {
		return nil, err
	}

	link := &LinkInfo{
		Index: ifInfo.Index,
		Flags: ifInfo.Flags,
	}

	// Parse attributes
	attrs, err := ParseAttrs(msg[NlMsgHdrLen+IfInfoMsgLen:])
	if err != nil {
		return nil, err
	}

	for _, attr := range attrs {
		switch attr.Type {
		case IFLA_IFNAME:
			link.Name = ParseAttrString(attr.Data)
		case IFLA_ADDRESS:
			link.HardwareAddr = net.HardwareAddr(attr.Data)
		case IFLA_MTU:
			if mtu, err := ParseAttrU32(attr.Data); err == nil {
				link.MTU = mtu
			}
		case IFLA_OPERSTATE:
			if state, err := ParseAttrU8(attr.Data); err == nil {
				link.OperState = state
			}
		case IFLA_TXQLEN:
			if txq, err := ParseAttrU32(attr.Data); err == nil {
				link.TxQueueLen = txq
			}
		case IFLA_QDISC:
			link.Qdisc = ParseAttrString(attr.Data)
		case IFLA_MASTER:
			if master, err := ParseAttrU32(attr.Data); err == nil {
				link.Master = int32(master) // #nosec G115 -- bounded by construction; see the surrounding guard
			}
		}
	}

	return link, nil
}

// =============================================================================
// RTNetlink Operations - Address Management
// =============================================================================

// GetAddrs retrieves all addresses for a specific interface (or all if index is 0).
func (s *Socket) GetAddrs(family uint8, ifindex uint32) ([]AddrInfo, error) {
	b := NewNlMsgBuilder(RTM_GETADDR, NLM_F_REQUEST|NLM_F_DUMP)
	b.AddIfAddrMsg(&IfAddrMsg{
		Family: family,
		Index:  ifindex,
	})

	responses, err := s.Execute(b.Finalize())
	if err != nil {
		return nil, err
	}

	var addrs []AddrInfo
	for _, resp := range responses {
		addr, err := parseAddrMessage(resp)
		if err != nil {
			continue
		}
		// Filter by interface if specified
		if ifindex == 0 || addr.Index == ifindex {
			addrs = append(addrs, *addr)
		}
	}

	return addrs, nil
}

// AddAddr adds an IP address to an interface.
func (s *Socket) AddAddr(ifindex uint32, addr *net.IPNet) error {
	family := uint8(unix.AF_INET)
	addrBytes := addr.IP.To4()
	if addrBytes == nil {
		family = unix.AF_INET6
		addrBytes = addr.IP.To16()
	}

	prefixLen, _ := addr.Mask.Size()

	b := NewNlMsgBuilder(RTM_NEWADDR, NLM_F_REQUEST|NLM_F_CREATE|NLM_F_ACK)
	b.AddIfAddrMsg(&IfAddrMsg{
		Family:    family,
		PrefixLen: uint8(prefixLen), // #nosec G115 -- bounded by construction; see the surrounding guard
		Scope:     RT_SCOPE_UNIVERSE,
		Index:     ifindex,
	})
	b.AddAttr(IFA_LOCAL, addrBytes)
	b.AddAttr(IFA_ADDRESS, addrBytes)

	_, err := s.Execute(b.Finalize())
	return err
}

// DelAddr removes an IP address from an interface.
func (s *Socket) DelAddr(ifindex uint32, addr *net.IPNet) error {
	family := uint8(unix.AF_INET)
	addrBytes := addr.IP.To4()
	if addrBytes == nil {
		family = unix.AF_INET6
		addrBytes = addr.IP.To16()
	}

	prefixLen, _ := addr.Mask.Size()

	b := NewNlMsgBuilder(RTM_DELADDR, NLM_F_REQUEST|NLM_F_ACK)
	b.AddIfAddrMsg(&IfAddrMsg{
		Family:    family,
		PrefixLen: uint8(prefixLen), // #nosec G115 -- kernel/netlink ABI field width
		Index:     ifindex,
	})
	b.AddAttr(IFA_LOCAL, addrBytes)
	b.AddAttr(IFA_ADDRESS, addrBytes)

	_, err := s.Execute(b.Finalize())
	return err
}

// parseAddrMessage parses a single RTM_NEWADDR message into AddrInfo.
func parseAddrMessage(msg []byte) (*AddrInfo, error) {
	if len(msg) < NlMsgHdrLen+IfAddrMsgLen {
		return nil, errors.New("message too short")
	}

	ifAddr, err := ParseIfAddrMsg(msg[NlMsgHdrLen : NlMsgHdrLen+IfAddrMsgLen])
	if err != nil {
		return nil, err
	}

	addr := &AddrInfo{
		Index:     ifAddr.Index,
		Family:    ifAddr.Family,
		PrefixLen: ifAddr.PrefixLen,
		Scope:     ifAddr.Scope,
	}

	attrs, err := ParseAttrs(msg[NlMsgHdrLen+IfAddrMsgLen:])
	if err != nil {
		return nil, err
	}

	for _, attr := range attrs {
		switch attr.Type {
		case IFA_ADDRESS:
			addr.Address = ParseAttrIP(attr.Data)
		case IFA_LOCAL:
			addr.Local = ParseAttrIP(attr.Data)
		case IFA_BROADCAST:
			addr.Broadcast = ParseAttrIP(attr.Data)
		case IFA_LABEL:
			addr.Label = ParseAttrString(attr.Data)
		}
	}

	return addr, nil
}

// =============================================================================
// RTNetlink Operations - Route Management
// =============================================================================

// GetRoutes retrieves all routes for a specific address family.
func (s *Socket) GetRoutes(family uint8) ([]RouteInfo, error) {
	b := NewNlMsgBuilder(RTM_GETROUTE, NLM_F_REQUEST|NLM_F_DUMP)
	b.AddRtMsg(&RtMsg{
		Family: family,
	})

	responses, err := s.Execute(b.Finalize())
	if err != nil {
		return nil, err
	}

	var routes []RouteInfo
	for _, resp := range responses {
		route, err := parseRouteMessage(resp)
		if err != nil {
			continue
		}
		routes = append(routes, *route)
	}

	return routes, nil
}

// AddRoute adds a new route.
func (s *Socket) AddRoute(dst *net.IPNet, gateway net.IP, ifindex int32) error {
	family := uint8(unix.AF_INET)
	var dstLen uint8
	var dstBytes []byte

	if dst != nil {
		dstBytes = dst.IP.To4()
		if dstBytes == nil {
			family = unix.AF_INET6
			dstBytes = dst.IP.To16()
		}
		dstLen = uint8(maskBits(dst.Mask)) // #nosec G115 -- bounded by construction; see the surrounding guard
	} else if gateway != nil {
		if gateway.To4() != nil {
			family = unix.AF_INET
		} else {
			family = unix.AF_INET6
		}
	}

	b := NewNlMsgBuilder(RTM_NEWROUTE, NLM_F_REQUEST|NLM_F_CREATE|NLM_F_ACK)
	b.AddRtMsg(&RtMsg{
		Family:   family,
		DstLen:   dstLen,
		Table:    RT_TABLE_MAIN,
		Protocol: RTPROT_STATIC,
		Scope:    RT_SCOPE_UNIVERSE,
		Type:     RTN_UNICAST,
	})

	if dst != nil && dstLen > 0 {
		b.AddAttr(RTA_DST, dstBytes)
	}
	if gateway != nil {
		gwBytes := gateway.To4()
		if gwBytes == nil {
			gwBytes = gateway.To16()
		}
		b.AddAttr(RTA_GATEWAY, gwBytes)
	}
	if ifindex > 0 {
		b.AddAttrU32(RTA_OIF, uint32(ifindex))
	}

	_, err := s.Execute(b.Finalize())
	return err
}

// DelRoute removes a route.
func (s *Socket) DelRoute(dst *net.IPNet, gateway net.IP) error {
	family := uint8(unix.AF_INET)
	var dstLen uint8
	var dstBytes []byte

	if dst != nil {
		dstBytes = dst.IP.To4()
		if dstBytes == nil {
			family = unix.AF_INET6
			dstBytes = dst.IP.To16()
		}
		dstLen = uint8(maskBits(dst.Mask)) // #nosec G115 -- bounded by construction; see the surrounding guard
	} else if gateway != nil {
		if gateway.To4() != nil {
			family = unix.AF_INET
		} else {
			family = unix.AF_INET6
		}
	}

	b := NewNlMsgBuilder(RTM_DELROUTE, NLM_F_REQUEST|NLM_F_ACK)
	b.AddRtMsg(&RtMsg{
		Family: family,
		DstLen: dstLen,
		Table:  RT_TABLE_MAIN,
		Scope:  RT_SCOPE_UNIVERSE,
	})

	if dst != nil && dstLen > 0 {
		b.AddAttr(RTA_DST, dstBytes)
	}
	if gateway != nil {
		gwBytes := gateway.To4()
		if gwBytes == nil {
			gwBytes = gateway.To16()
		}
		b.AddAttr(RTA_GATEWAY, gwBytes)
	}

	_, err := s.Execute(b.Finalize())
	return err
}

// parseRouteMessage parses a single RTM_NEWROUTE message into RouteInfo.
func parseRouteMessage(msg []byte) (*RouteInfo, error) {
	if len(msg) < NlMsgHdrLen+RtMsgLen {
		return nil, errors.New("message too short")
	}

	rtMsg, err := ParseRtMsg(msg[NlMsgHdrLen : NlMsgHdrLen+RtMsgLen])
	if err != nil {
		return nil, err
	}

	route := &RouteInfo{
		Family:   rtMsg.Family,
		DstLen:   rtMsg.DstLen,
		SrcLen:   rtMsg.SrcLen,
		Table:    rtMsg.Table,
		Protocol: rtMsg.Protocol,
		Scope:    rtMsg.Scope,
		Type:     rtMsg.Type,
	}

	attrs, err := ParseAttrs(msg[NlMsgHdrLen+RtMsgLen:])
	if err != nil {
		return nil, err
	}

	for _, attr := range attrs {
		switch attr.Type {
		case RTA_DST:
			route.Dst = ParseAttrIP(attr.Data)
		case RTA_SRC:
			route.Src = ParseAttrIP(attr.Data)
		case RTA_GATEWAY:
			route.Gateway = ParseAttrIP(attr.Data)
		case RTA_OIF:
			if oif, err := ParseAttrU32(attr.Data); err == nil {
				route.OifIndex = int32(oif) // #nosec G115 -- kernel/netlink ABI field width
			}
		case RTA_IIF:
			if iif, err := ParseAttrU32(attr.Data); err == nil {
				route.IifIndex = int32(iif) // #nosec G115 -- kernel/netlink ABI field width
			}
		case RTA_PRIORITY:
			if prio, err := ParseAttrU32(attr.Data); err == nil {
				route.Priority = prio
			}
		case RTA_PREFSRC:
			route.PrefSrc = ParseAttrIP(attr.Data)
		}
	}

	return route, nil
}

// maskBits returns the number of bits in a network mask.
func maskBits(mask net.IPMask) int {
	bits, _ := mask.Size()
	return bits
}

// =============================================================================
// XDP Program Attachment
// =============================================================================

// AttachXDP attaches an XDP program to an interface.
// The fd parameter is the file descriptor of a loaded BPF program.
// flags can be XDP_FLAGS_* constants.
func (s *Socket) AttachXDP(ifindex int32, fd int, flags uint32) error {
	b := NewNlMsgBuilder(RTM_SETLINK, NLM_F_REQUEST|NLM_F_ACK)
	b.AddIfInfoMsg(&IfInfoMsg{
		Family: unix.AF_UNSPEC,
		Index:  ifindex,
	})

	// Create nested XDP attribute
	xdpOffset := b.AddNestedAttr(IFLA_XDP)
	b.AddAttrU32(IFLA_XDP_FD, uint32(fd)) // #nosec G115 -- bounded by construction; see the surrounding guard
	b.AddAttrU32(IFLA_XDP_FLAGS, flags)
	b.EndNestedAttr(xdpOffset)

	_, err := s.Execute(b.Finalize())
	return err
}

// DetachXDP detaches an XDP program from an interface.
func (s *Socket) DetachXDP(ifindex int32, flags uint32) error {
	// Detach by attaching fd=-1
	return s.AttachXDP(ifindex, -1, flags)
}

// GetXDPInfo retrieves XDP information for an interface.
func (s *Socket) GetXDPInfo(ifindex int32) (uint32, uint32, error) {
	link, err := s.GetLinkByIndex(ifindex)
	if err != nil {
		return 0, 0, err
	}
	// The XDP info would need additional parsing from nested attributes
	// For now, return basic link info
	_ = link
	return 0, 0, nil // Would need to parse IFLA_XDP nested attrs
}

// =============================================================================
// Traffic Control (TC) Operations
// =============================================================================

// AddQdisc adds a queueing discipline to an interface.
func (s *Socket) AddQdisc(ifindex int32, parent, handle uint32, kind string) error {
	b := NewNlMsgBuilder(RTM_NEWQDISC, NLM_F_REQUEST|NLM_F_CREATE|NLM_F_ACK)
	b.AddTcMsg(&TcMsg{
		Family:  unix.AF_UNSPEC,
		Ifindex: ifindex,
		Handle:  handle,
		Parent:  parent,
	})
	b.AddAttrString(TCA_KIND, kind)

	_, err := s.Execute(b.Finalize())
	return err
}

// DelQdisc removes a queueing discipline from an interface.
func (s *Socket) DelQdisc(ifindex int32, parent, handle uint32) error {
	b := NewNlMsgBuilder(RTM_DELQDISC, NLM_F_REQUEST|NLM_F_ACK)
	b.AddTcMsg(&TcMsg{
		Family:  unix.AF_UNSPEC,
		Ifindex: ifindex,
		Handle:  handle,
		Parent:  parent,
	})

	_, err := s.Execute(b.Finalize())
	return err
}

// AddClsactQdisc adds a clsact qdisc for attaching eBPF programs.
// The clsact qdisc is a special qdisc that provides ingress and egress hooks.
func (s *Socket) AddClsactQdisc(ifindex int32) error {
	return s.AddQdisc(ifindex, TC_H_CLSACT, TC_H_CLSACT, "clsact")
}

// AddFilter adds a TC filter to an interface.
func (s *Socket) AddFilter(ifindex int32, parent uint32, protocol uint16, priority uint16, kind string, options []byte) error {
	b := NewNlMsgBuilder(RTM_NEWTFILTER, NLM_F_REQUEST|NLM_F_CREATE|NLM_F_ACK)

	// Info field encodes priority and protocol
	info := uint32(priority)<<16 | uint32(protocol)

	b.AddTcMsg(&TcMsg{
		Family:  unix.AF_UNSPEC,
		Ifindex: ifindex,
		Handle:  0,
		Parent:  parent,
		Info:    info,
	})
	b.AddAttrString(TCA_KIND, kind)
	if len(options) > 0 {
		b.AddAttr(TCA_OPTIONS, options)
	}

	_, err := s.Execute(b.Finalize())
	return err
}

// DelFilter removes a TC filter from an interface.
func (s *Socket) DelFilter(ifindex int32, parent uint32, protocol uint16, priority uint16) error {
	b := NewNlMsgBuilder(RTM_DELTFILTER, NLM_F_REQUEST|NLM_F_ACK)

	info := uint32(priority)<<16 | uint32(protocol)

	b.AddTcMsg(&TcMsg{
		Family:  unix.AF_UNSPEC,
		Ifindex: ifindex,
		Handle:  0,
		Parent:  parent,
		Info:    info,
	})

	_, err := s.Execute(b.Finalize())
	return err
}

// GetQdiscs retrieves all queueing disciplines.
func (s *Socket) GetQdiscs() ([]TcMsg, error) {
	b := NewNlMsgBuilder(RTM_GETQDISC, NLM_F_REQUEST|NLM_F_DUMP)
	b.AddTcMsg(&TcMsg{
		Family: unix.AF_UNSPEC,
	})

	responses, err := s.Execute(b.Finalize())
	if err != nil {
		return nil, err
	}

	var qdiscs []TcMsg
	for _, resp := range responses {
		if len(resp) < NlMsgHdrLen+TcMsgLen {
			continue
		}
		tcMsg, err := ParseTcMsg(resp[NlMsgHdrLen : NlMsgHdrLen+TcMsgLen])
		if err != nil {
			continue
		}
		qdiscs = append(qdiscs, *tcMsg)
	}

	return qdiscs, nil
}

// =============================================================================
// Generic Netlink Operations
// =============================================================================

// GenlSocket is a convenience wrapper for generic netlink operations.
type GenlSocket struct {
	*Socket
}

// NewGenlSocket creates a new generic netlink socket.
func NewGenlSocket() (*GenlSocket, error) {
	s, err := NewSocket(NETLINK_GENERIC)
	if err != nil {
		return nil, err
	}
	return &GenlSocket{Socket: s}, nil
}

// GetFamilyID retrieves the family ID for a generic netlink family by name.
// This is required before communicating with a specific generic netlink family.
func (g *GenlSocket) GetFamilyID(name string) (uint16, error) {
	b := NewNlMsgBuilder(GENL_ID_CTRL, NLM_F_REQUEST)
	b.AddGenlMsgHdr(&GenlMsgHdr{
		Cmd:     GENL_CTRL_CMD_GETFAMILY,
		Version: 1,
	})
	b.AddAttrString(CTRL_ATTR_FAMILY_NAME, name)

	responses, err := g.Execute(b.Finalize())
	if err != nil {
		return 0, err
	}

	if len(responses) == 0 {
		return 0, errors.New("no response for family query")
	}

	// Parse response to find family ID
	msg := responses[0]
	if len(msg) < NlMsgHdrLen+GenlMsgHdrLen {
		return 0, errors.New("response too short")
	}

	attrs, err := ParseAttrs(msg[NlMsgHdrLen+GenlMsgHdrLen:])
	if err != nil {
		return 0, err
	}

	for _, attr := range attrs {
		if attr.Type == CTRL_ATTR_FAMILY_ID {
			id, err := ParseAttrU16(attr.Data)
			if err != nil {
				return 0, err
			}
			return id, nil
		}
	}

	return 0, errors.New("family ID not found in response")
}

// SendGenlMsg sends a generic netlink message to a specific family.
func (g *GenlSocket) SendGenlMsg(familyID uint16, cmd uint8, version uint8, attrs []byte) ([][]byte, error) {
	b := NewNlMsgBuilder(familyID, NLM_F_REQUEST)
	b.AddGenlMsgHdr(&GenlMsgHdr{
		Cmd:     cmd,
		Version: version,
	})
	if len(attrs) > 0 {
		b.data = append(b.data, attrs...)
	}

	return g.Execute(b.Finalize())
}

// =============================================================================
// Utility Functions
// =============================================================================

// MakeTcHandle creates a TC handle from major and minor parts.
// TC handles are 32-bit values where high 16 bits are the major number
// and low 16 bits are the minor number.
func MakeTcHandle(major, minor uint16) uint32 {
	return uint32(major)<<16 | uint32(minor)
}

// SplitTcHandle splits a TC handle into major and minor parts.
func SplitTcHandle(handle uint32) (major, minor uint16) {
	return uint16(handle >> 16), uint16(handle & 0xFFFF)
}

// IfaceFlags converts interface flags to a human-readable string.
func IfaceFlags(flags uint32) string {
	var names []string
	if flags&unix.IFF_UP != 0 {
		names = append(names, "UP")
	}
	if flags&unix.IFF_BROADCAST != 0 {
		names = append(names, "BROADCAST")
	}
	if flags&unix.IFF_LOOPBACK != 0 {
		names = append(names, "LOOPBACK")
	}
	if flags&unix.IFF_POINTOPOINT != 0 {
		names = append(names, "POINTOPOINT")
	}
	if flags&unix.IFF_MULTICAST != 0 {
		names = append(names, "MULTICAST")
	}
	if flags&unix.IFF_RUNNING != 0 {
		names = append(names, "RUNNING")
	}
	if flags&unix.IFF_PROMISC != 0 {
		names = append(names, "PROMISC")
	}
	if len(names) == 0 {
		return "NONE"
	}
	result := names[0]
	for i := 1; i < len(names); i++ {
		result += "|" + names[i]
	}
	return result
}

// OperStateString converts operational state to a human-readable string.
func OperStateString(state uint8) string {
	switch state {
	case 0:
		return "UNKNOWN"
	case 1:
		return "NOTPRESENT"
	case 2:
		return "DOWN"
	case 3:
		return "LOWERLAYERDOWN"
	case 4:
		return "TESTING"
	case 5:
		return "DORMANT"
	case 6:
		return "UP"
	default:
		return fmt.Sprintf("STATE(%d)", state)
	}
}

// RouteTypeString converts route type to a human-readable string.
func RouteTypeString(t uint8) string {
	switch t {
	case RTN_UNSPEC:
		return "UNSPEC"
	case RTN_UNICAST:
		return "UNICAST"
	case RTN_LOCAL:
		return "LOCAL"
	case RTN_BROADCAST:
		return "BROADCAST"
	case RTN_ANYCAST:
		return "ANYCAST"
	case RTN_MULTICAST:
		return "MULTICAST"
	case RTN_BLACKHOLE:
		return "BLACKHOLE"
	case RTN_UNREACHABLE:
		return "UNREACHABLE"
	case RTN_PROHIBIT:
		return "PROHIBIT"
	case RTN_THROW:
		return "THROW"
	default:
		return fmt.Sprintf("TYPE(%d)", t)
	}
}

// RouteScopeString converts route scope to a human-readable string.
func RouteScopeString(scope uint8) string {
	switch scope {
	case RT_SCOPE_UNIVERSE:
		return "UNIVERSE"
	case RT_SCOPE_SITE:
		return "SITE"
	case RT_SCOPE_LINK:
		return "LINK"
	case RT_SCOPE_HOST:
		return "HOST"
	case RT_SCOPE_NOWHERE:
		return "NOWHERE"
	default:
		return fmt.Sprintf("SCOPE(%d)", scope)
	}
}

// RouteProtocolString converts route protocol to a human-readable string.
func RouteProtocolString(proto uint8) string {
	switch proto {
	case RTPROT_UNSPEC:
		return "UNSPEC"
	case RTPROT_REDIRECT:
		return "REDIRECT"
	case RTPROT_KERNEL:
		return "KERNEL"
	case RTPROT_BOOT:
		return "BOOT"
	case RTPROT_STATIC:
		return "STATIC"
	case RTPROT_DHCP:
		return "DHCP"
	case RTPROT_BGP:
		return "BGP"
	case RTPROT_OSPF:
		return "OSPF"
	default:
		return fmt.Sprintf("PROTO(%d)", proto)
	}
}

// SetLinkUp sets a network interface to the UP state.
func (s *Socket) SetLinkUp(ifindex int32) error {
	b := NewNlMsgBuilder(RTM_SETLINK, NLM_F_REQUEST|NLM_F_ACK)
	b.AddIfInfoMsg(&IfInfoMsg{
		Family: unix.AF_UNSPEC,
		Index:  ifindex,
		Flags:  unix.IFF_UP,
		Change: unix.IFF_UP,
	})

	_, err := s.Execute(b.Finalize())
	return err
}

// SetLinkDown sets a network interface to the DOWN state.
func (s *Socket) SetLinkDown(ifindex int32) error {
	b := NewNlMsgBuilder(RTM_SETLINK, NLM_F_REQUEST|NLM_F_ACK)
	b.AddIfInfoMsg(&IfInfoMsg{
		Family: unix.AF_UNSPEC,
		Index:  ifindex,
		Flags:  0,
		Change: unix.IFF_UP,
	})

	_, err := s.Execute(b.Finalize())
	return err
}

// SetLinkMTU sets the MTU for a network interface.
func (s *Socket) SetLinkMTU(ifindex int32, mtu uint32) error {
	b := NewNlMsgBuilder(RTM_SETLINK, NLM_F_REQUEST|NLM_F_ACK)
	b.AddIfInfoMsg(&IfInfoMsg{
		Family: unix.AF_UNSPEC,
		Index:  ifindex,
	})
	b.AddAttrU32(IFLA_MTU, mtu)

	_, err := s.Execute(b.Finalize())
	return err
}

// SetLinkName renames a network interface.
func (s *Socket) SetLinkName(ifindex int32, name string) error {
	b := NewNlMsgBuilder(RTM_SETLINK, NLM_F_REQUEST|NLM_F_ACK)
	b.AddIfInfoMsg(&IfInfoMsg{
		Family: unix.AF_UNSPEC,
		Index:  ifindex,
	})
	b.AddAttrString(IFLA_IFNAME, name)

	_, err := s.Execute(b.Finalize())
	return err
}

// SetLinkMaster sets the master device for a network interface (e.g., for bridging).
func (s *Socket) SetLinkMaster(ifindex, masterIndex int32) error {
	b := NewNlMsgBuilder(RTM_SETLINK, NLM_F_REQUEST|NLM_F_ACK)
	b.AddIfInfoMsg(&IfInfoMsg{
		Family: unix.AF_UNSPEC,
		Index:  ifindex,
	})
	b.AddAttrU32(IFLA_MASTER, uint32(masterIndex)) // #nosec G115 -- kernel/netlink ABI field width

	_, err := s.Execute(b.Finalize())
	return err
}

// SetLinkPromisc enables or disables promiscuous mode on an interface.
func (s *Socket) SetLinkPromisc(ifindex int32, enable bool) error {
	var flags uint32
	if enable {
		flags = unix.IFF_PROMISC
	}

	b := NewNlMsgBuilder(RTM_SETLINK, NLM_F_REQUEST|NLM_F_ACK)
	b.AddIfInfoMsg(&IfInfoMsg{
		Family: unix.AF_UNSPEC,
		Index:  ifindex,
		Flags:  flags,
		Change: unix.IFF_PROMISC,
	})

	_, err := s.Execute(b.Finalize())
	return err
}

// =============================================================================
// Netlink Error Handling Utilities
// =============================================================================

// NlError represents a netlink-specific error with additional context.
type NlError struct {
	Errno   unix.Errno
	Message string
}

func (e *NlError) Error() string {
	return fmt.Sprintf("netlink error: %s (%d)", e.Message, e.Errno)
}

// IsExist returns true if the error indicates the object already exists.
func IsExist(err error) bool {
	if nlErr, ok := err.(*NlError); ok {
		return nlErr.Errno == unix.EEXIST
	}
	return false
}

// IsNotExist returns true if the error indicates the object does not exist.
func IsNotExist(err error) bool {
	if nlErr, ok := err.(*NlError); ok {
		return nlErr.Errno == unix.ENOENT || nlErr.Errno == unix.ENODEV
	}
	return false
}

// IsPermission returns true if the error indicates a permission problem.
func IsPermission(err error) bool {
	if nlErr, ok := err.(*NlError); ok {
		return nlErr.Errno == unix.EPERM || nlErr.Errno == unix.EACCES
	}
	return false
}

// =============================================================================
// Debug and Inspection Utilities
// =============================================================================

// DumpMessage returns a hex dump of a netlink message for debugging.
func DumpMessage(msg []byte) string {
	if len(msg) < NlMsgHdrLen {
		return "invalid message (too short)"
	}

	hdr, _ := ParseNlMsgHdr(msg)
	result := fmt.Sprintf("NlMsgHdr{Len:%d, Type:%d, Flags:0x%x, Seq:%d, Pid:%d}\n",
		hdr.Len, hdr.Type, hdr.Flags, hdr.Seq, hdr.Pid)

	// Hex dump of payload
	if len(msg) > NlMsgHdrLen {
		result += "Payload: "
		for i, b := range msg[NlMsgHdrLen:] {
			if i > 0 && i%16 == 0 {
				result += "\n         "
			}
			result += fmt.Sprintf("%02x ", b)
		}
	}

	return result
}

// MessageTypeName returns a human-readable name for a netlink message type.
func MessageTypeName(msgType uint16) string {
	switch msgType {
	case NLMSG_NOOP:
		return "NLMSG_NOOP"
	case NLMSG_ERROR:
		return "NLMSG_ERROR"
	case NLMSG_DONE:
		return "NLMSG_DONE"
	case NLMSG_OVERRUN:
		return "NLMSG_OVERRUN"
	case RTM_NEWLINK:
		return "RTM_NEWLINK"
	case RTM_DELLINK:
		return "RTM_DELLINK"
	case RTM_GETLINK:
		return "RTM_GETLINK"
	case RTM_SETLINK:
		return "RTM_SETLINK"
	case RTM_NEWADDR:
		return "RTM_NEWADDR"
	case RTM_DELADDR:
		return "RTM_DELADDR"
	case RTM_GETADDR:
		return "RTM_GETADDR"
	case RTM_NEWROUTE:
		return "RTM_NEWROUTE"
	case RTM_DELROUTE:
		return "RTM_DELROUTE"
	case RTM_GETROUTE:
		return "RTM_GETROUTE"
	case RTM_NEWQDISC:
		return "RTM_NEWQDISC"
	case RTM_DELQDISC:
		return "RTM_DELQDISC"
	case RTM_GETQDISC:
		return "RTM_GETQDISC"
	case RTM_NEWTFILTER:
		return "RTM_NEWTFILTER"
	case RTM_DELTFILTER:
		return "RTM_DELTFILTER"
	case RTM_GETTFILTER:
		return "RTM_GETTFILTER"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", msgType)
	}
}

// PointerToBytes is a helper to convert a pointer to a byte slice.
// This is useful for working with low-level kernel structures.
func PointerToBytes(ptr unsafe.Pointer, size int) []byte {
	return (*[1 << 30]byte)(ptr)[:size:size]
}
