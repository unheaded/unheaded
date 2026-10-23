// SPDX-License-Identifier: GPL-3.0-or-later

package main

import "crypto/tls"

// A third-party SMTP relay decides the group; the 1.2 floor stays.
var relay = &tls.Config{ServerName: "relay.example", MinVersion: tls.VersionTLS12}
