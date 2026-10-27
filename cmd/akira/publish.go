// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
	"unheaded/pkg/wotansig"
)

// signingPublisher is the part of the Wotan client Akira publishes with
// (wotanClient.Client.PublishWithSigner).
type signingPublisher interface {
	PublishWithSigner(ctx context.Context, topic string, payload []byte, signer *wotansig.TopicSigner) error
}

// wotanPublisher publishes Akira's messages. system.health.consensus and
// system.health.remediate are signed topics (S-IRON-GATE Phase 9, M-053):
// with the key HEALTH_SIGNING_KEY_FILE names they go out ML-DSA-65 signed
// over gRPC; without one they are refused here, logged once per topic and
// counted, never sent unsigned. Reports (system.health.reports) are not
// signed and always go out.
type wotanPublisher struct {
	pub     signingPublisher
	signer  *wotansig.TopicSigner
	refused *metrics.CounterVec

	mu     sync.Mutex
	warned map[string]bool
}

func newWotanPublisher(pub signingPublisher, signer *wotansig.TopicSigner, reg *metrics.Registry) *wotanPublisher {
	cv := metrics.NewCounterVec("akira_signed_publish_refused_total",
		"Messages Akira did not publish because the topic is signed and HEALTH_SIGNING_KEY_FILE is not set.",
		nil, []string{"topic"})
	if err := reg.Register(cv); err != nil {
		log.Error().Err(err).Msg("akira_signed_publish_refused_total not registered")
	}
	return &wotanPublisher{pub: pub, signer: signer, refused: cv, warned: map[string]bool{}}
}

func (p *wotanPublisher) publish(topic string, v interface{}) {
	payload, err := json.Marshal(v)
	if err != nil {
		return
	}
	if p.signer == nil && wotansig.RequiresSignature(topic) {
		p.refused.WithLabelValues(topic).Inc()
		p.mu.Lock()
		first := !p.warned[topic]
		p.warned[topic] = true
		p.mu.Unlock()
		if first {
			log.Error().Str("topic", topic).Str("env", wotansig.HealthKeyEnv).
				Msg("not publishing a signed topic: no health signing key (set HEALTH_SIGNING_KEY_FILE and enrol its public key in Wotan's signed_prefixes); counted in akira_signed_publish_refused_total")
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.pub.PublishWithSigner(ctx, topic, payload, p.signer); err != nil {
		log.Warn().Err(err).Str("topic", topic).Msg("publish failed")
	}
}

// loadHealthSigner loads the key HEALTH_SIGNING_KEY_FILE names: nil when
// unset (Akira then refuses its signed topics), an error when it names a
// key that cannot be used (Akira refuses to start).
func loadHealthSigner() (*wotansig.TopicSigner, error) {
	return wotansig.SignerFromEnv(wotansig.HealthKeyEnv)
}

// wotanGRPCAddr is --wotan-grpc, or the --wotan host on Wotan's gRPC port.
func wotanGRPCAddr(httpAddr, grpcAddr string) string {
	if grpcAddr != "" {
		return grpcAddr
	}
	hostport := httpAddr
	if i := strings.Index(hostport, "://"); i >= 0 {
		hostport = hostport[i+3:]
	}
	hostport = strings.TrimSuffix(hostport, "/")
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	return net.JoinHostPort(host, strconv.Itoa(ports.WotanGRPC))
}
