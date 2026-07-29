// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package proxy

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

const soOriginalDst = 80 // SO_ORIGINAL_DST (netfilter extension)

// getOriginalDst retrieves the original destination via SO_ORIGINAL_DST (Linux).
func getOriginalDst(conn net.Conn) (string, error) {
	sc, ok := conn.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	if !ok {
		return "", fmt.Errorf("connection does not support SyscallConn: %T", conn)
	}

	rawConn, err := sc.SyscallConn()
	if err != nil {
		return "", fmt.Errorf("get raw connection: %w", err)
	}

	var addr string
	var sockErr error

	err = rawConn.Control(func(fd uintptr) {
		// Try IPv4 first
		var sa4 syscall.RawSockaddrInet4
		sz := uint32(unsafe.Sizeof(sa4))

		_, _, errno := syscall.Syscall6(
			syscall.SYS_GETSOCKOPT,
			fd,
			syscall.SOL_IP,
			soOriginalDst,
			uintptr(unsafe.Pointer(&sa4)), // #nosec G103 -- Pointer->uintptr inside a syscall argument list, the pattern unsafe.Pointer rule (4) permits
			uintptr(unsafe.Pointer(&sz)),  // #nosec G103 -- Pointer->uintptr inside a syscall argument list, the pattern unsafe.Pointer rule (4) permits
			0,
		)
		if errno != 0 {
			sockErr = fmt.Errorf("getsockopt SO_ORIGINAL_DST: %w", errno)
			return
		}

		// Parse address: Port is big-endian uint16, swap to host byte order
		port := int(sa4.Port&0xff)<<8 | int(sa4.Port>>8)
		ip := net.IPv4(sa4.Addr[0], sa4.Addr[1], sa4.Addr[2], sa4.Addr[3])
		addr = fmt.Sprintf("%s:%d", ip.String(), port)
	})
	if err != nil {
		return "", fmt.Errorf("rawconn control: %w", err)
	}
	if sockErr != nil {
		return "", sockErr
	}

	return addr, nil
}
