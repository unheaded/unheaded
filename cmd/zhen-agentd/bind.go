// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"net"
	"strconv"

	"unheaded/pkg/modules"
)

// checkListenAddrs applies the module bind rule (ADR-149) to the API
// listener (-host, -port) and, when set, the metrics listener
// (-metrics-addr). allowEnv is UNHEADED_MODULE_BIND_ALLOW's value.
func checkListenAddrs(host string, port int, metricsAddr, allowEnv string) error {
	allow, err := modules.ParseBindAllow(allowEnv)
	if err != nil {
		return err
	}
	if err := modules.CheckBind(net.JoinHostPort(host, strconv.Itoa(port)), allow); err != nil {
		return err
	}
	if metricsAddr != "" {
		if err := modules.CheckBind(metricsAddr, allow); err != nil {
			return err
		}
	}
	return nil
}
