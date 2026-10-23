// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// (f) gRPC server credentials from files: Go's default floor (TLS 1.2).
func F(cert, key string) (grpc.ServerOption, error) {
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		return nil, err
	}
	return grpc.Creds(creds), nil
}
