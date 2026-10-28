// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"unheaded/pkg/wotanerr"
	chatpb "unheaded/services/wotan/proto"
)

// trailerStream records the trailer a unary handler sets.
type trailerStream struct{ trailer metadata.MD }

func (s *trailerStream) Method() string               { return "/chat.TopicService/PublishTopic" }
func (s *trailerStream) SetHeader(metadata.MD) error  { return nil }
func (s *trailerStream) SendHeader(metadata.MD) error { return nil }
func (s *trailerStream) SetTrailer(md metadata.MD) error {
	s.trailer = metadata.Join(s.trailer, md)
	return nil
}

// S-TRUE-WIRE Step 145: a refused gRPC publish carries the wotan-memory-03
// code in the x-wotan-error trailer.
func TestPublishTopicTrailerCarriesWotanErrorCode(t *testing.T) {
	cases := []struct {
		name string
		req  *chatpb.TopicPublishRequest
		want wotanerr.Code
	}{
		{"unauthorized publish", &chatpb.TopicPublishRequest{Topic: "alerts.critical", Payload: []byte("x"),
			Metadata: map[string]string{"display_name": "stranger"}}, wotanerr.UnauthorizedPublish},
		{"signature required", &chatpb.TopicPublishRequest{Topic: "config.kingdom", Payload: []byte("x"),
			Metadata: map[string]string{"display_name": "service"}}, wotanerr.SignatureRequired},
		{"signature invalid", &chatpb.TopicPublishRequest{Topic: "config.kingdom", Payload: []byte("x"),
			Signature: []byte("not-a-signature"), PublicKey: []byte("not-a-key"), Algorithm: "ML-DSA-65",
			Metadata: map[string]string{"display_name": "service"}}, wotanerr.SignatureInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := productionTopicService()
			svc.AutoApprove = allowList{"service": true}
			st := &trailerStream{}
			ctx := grpc.NewContextWithServerTransportStream(context.Background(), st)
			if _, err := svc.PublishTopic(ctx, tc.req); err == nil {
				t.Fatal("publish accepted")
			}
			got, ok := wotanerr.FromValues(st.trailer.Get(wotanerr.TrailerKey))
			if !ok || *got != tc.want {
				t.Fatalf("trailer %v, want %s", st.trailer, tc.want.Hex())
			}
		})
	}
}
