// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package api

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"unheaded/pkg/auth"
	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
	"unheaded/services/wotan/internal/topicpattern"
)

// Permissions is topics.permissions in configs/wotan.yaml: which topics each
// service identity may read and publish (FND-043). Without the section
// Wotan behaves as before it: an approved caller may stream any pattern.
// With it, a verified service identity gets exactly its lists, and an
// identity the section does not name gets what unlisted says.
//
// Only a service identity (an EdDSA service token, ADR-103) is judged. With
// auth off there is no identity, and the section does not apply.
type Permissions struct {
	// Unlisted is what an identity absent from Services may do: "deny"
	// (default) nothing, "allow" everything, as before this section.
	Unlisted string `yaml:"unlisted"`
	// Mode is "enforce" (default): a refusal is an error to the caller; or
	// "audit": the call goes through and the refusal is logged and counted,
	// for finding the lists' gaps before enforcing them.
	Mode string `yaml:"mode"`
	// Services maps a service identity (the sub of its token, the name its
	// key is filed under) to its allowances.
	Services map[string]ServicePermissions `yaml:"services"`
}

// ServicePermissions are one identity's allowances. Entries are topic
// patterns in Wotan's grammar ("tasks.created", "tasks.*", "system.health.#",
// "#"). A service may read what it may publish, so Publish entries count
// for Subscribe too.
type ServicePermissions struct {
	Subscribe []string `yaml:"subscribe"`
	Publish   []string `yaml:"publish"`
}

// Actions Permit judges.
const (
	PermitSubscribe = "subscribe" // open a stream or a subscription, or read a topic's messages
	PermitPublish   = "publish"   // publish one message to a topic
)

const (
	unlistedDeny  = "deny"
	unlistedAllow = "allow"
	modeEnforce   = "enforce"
	modeAudit     = "audit"
)

// UnmarshalYAML refuses unknown fields: a misspelt "services" would leave
// every identity unlisted, and a misspelt "mode" would enforce by accident.
func (p *Permissions) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: topics.permissions is a mapping {unlisted, mode, services}", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		switch k := n.Content[i].Value; k {
		case "unlisted", "mode", "services":
		default:
			return fmt.Errorf("line %d: unknown field %q in topics.permissions", n.Content[i].Line, k)
		}
	}
	type plain Permissions
	return n.Decode((*plain)(p))
}

// UnmarshalYAML refuses unknown fields: a misspelt "subscribe" would leave
// a service with no read allowance and no error.
func (sp *ServicePermissions) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a topics.permissions.services entry is a mapping {subscribe, publish}", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		switch k := n.Content[i].Value; k {
		case "subscribe", "publish":
		default:
			return fmt.Errorf("line %d: unknown field %q in a topics.permissions.services entry", n.Content[i].Line, k)
		}
	}
	type plain ServicePermissions
	return n.Decode((*plain)(sp))
}

// compiledPermissions is Permissions checked and lower-cased, kept under
// TopicConfig.mu. nil means the section is absent.
type compiledPermissions struct {
	unlistedAllow bool
	audit         bool
	services      map[string]compiledService
}

type compiledService struct {
	subscribe []string // Subscribe plus Publish
	publish   []string
}

// compilePermissions validates p. A pattern that is not valid, an empty
// identity, or a value of unlisted or mode that is not one of the two
// words is an error: the section is a security boundary and a typo in it
// must stop Wotan from starting (or a reload from applying).
func compilePermissions(p *Permissions) (*compiledPermissions, error) {
	if p == nil {
		return nil, nil
	}
	cp := &compiledPermissions{services: make(map[string]compiledService, len(p.Services))}
	switch strings.ToLower(strings.TrimSpace(p.Unlisted)) {
	case "", unlistedDeny:
	case unlistedAllow:
		cp.unlistedAllow = true
	default:
		return nil, fmt.Errorf("topics.permissions.unlisted: %q is not deny or allow", p.Unlisted)
	}
	switch strings.ToLower(strings.TrimSpace(p.Mode)) {
	case "", modeEnforce:
	case modeAudit:
		cp.audit = true
	default:
		return nil, fmt.Errorf("topics.permissions.mode: %q is not enforce or audit", p.Mode)
	}
	for name, sp := range p.Services {
		id := normalIdentity(name)
		if id == "" {
			return nil, fmt.Errorf("topics.permissions.services: an identity name is empty")
		}
		if _, dup := cp.services[id]; dup {
			return nil, fmt.Errorf("topics.permissions.services: %q is listed twice (names are matched without case)", id)
		}
		cs := compiledService{}
		for _, list := range [][]string{sp.Subscribe, sp.Publish} {
			for _, pat := range list {
				if err := validAllowance(strings.TrimSpace(pat)); err != nil {
					return nil, fmt.Errorf("topics.permissions.services.%s: %w", id, err)
				}
			}
		}
		for _, pat := range sp.Publish {
			cs.publish = append(cs.publish, strings.TrimSpace(pat))
		}
		cs.subscribe = append(cs.subscribe, cs.publish...)
		for _, pat := range sp.Subscribe {
			cs.subscribe = append(cs.subscribe, strings.TrimSpace(pat))
		}
		cp.services[id] = cs
	}
	cp.startRefusalSeries()
	return cp, nil
}

// startRefusalSeries creates, at zero, the refusal series of every caller
// the section names and of callerOther, for the outcome its mode gives.
// Without it a caller's series would first appear at its first refusal
// with the value 1, and increase() over a series that starts at 1 is 0:
// the first refusal would raise no alert. Adding zero changes no count, so
// a reload is safe.
func (cp *compiledPermissions) startRefusalSeries() {
	outcome := outcomeDenied
	if cp.audit {
		outcome = outcomeAudited
	}
	for _, action := range []string{PermitSubscribe, PermitPublish} {
		topicPermissionRefusals.WithLabelValues(action, outcome, callerOther).Add(0)
		for id := range cp.services {
			topicPermissionRefusals.WithLabelValues(action, outcome, id).Add(0)
		}
	}
}

// validAllowance checks one list entry: Wotan's pattern characters, no
// empty segment, and "#" only as the last segment. Match accepts nothing
// else, so an entry that failed here would allow nothing while looking
// like it allows something.
func validAllowance(pat string) error {
	if !topicpattern.ValidPattern(pat) {
		return fmt.Errorf("%q is not a topic pattern", pat)
	}
	segs := strings.Split(pat, ".")
	for i, seg := range segs {
		switch {
		case seg == "":
			return fmt.Errorf("%q has an empty segment", pat)
		case seg == "#" && i != len(segs)-1:
			return fmt.Errorf("%q: \"#\" is allowed only as the last segment", pat)
		case strings.ContainsAny(seg, "*#") && len(seg) > 1:
			return fmt.Errorf("%q: a wildcard is a whole segment", pat)
		}
	}
	return nil
}

// callerOther is the caller label of every refusal whose identity
// topics.permissions does not name.
const callerOther = "other"

// The refusal counter's outcomes.
const (
	outcomeDenied  = "denied"  // enforce: the caller got an error
	outcomeAudited = "audited" // audit: counted and logged, the call went through
)

// Refusal counters, in the default registry Wotan's /metrics serves.
// outcome is "denied" (enforce) or "audited" (audit: the call went through).
// caller is the refused service's name as topics.permissions spells it, or
// "other": the label's values are the config's identities and that one
// word, never the subject or the pattern a request carried.
var topicPermissionRefusals = auto.NewCounterVec(prom.CounterOpts{
	Name: "unheaded_wotan_topic_permission_refusals_total",
	Help: "Topic reads or publishes outside the caller's topics.permissions, by action, outcome (denied, audited) and caller (an identity listed in topics.permissions, or other; bounded by the config's identities)",
}, []string{"action", "outcome", "caller"})

// Permit reports whether identity may do action (PermitSubscribe or
// PermitPublish) on pattern: a topic, or for PermitSubscribe a pattern. The
// reason is for the caller's log line, never for the client.
//
// A nil config, a config without topics.permissions, or a call with no
// identity (auth off) permits everything, as before FND-043. An identity
// that is not a service's (a user token, an API key) is never permitted:
// its name proves no service. In audit mode a refusal is counted and
// logged, and the call is permitted.
func (c *TopicConfig) Permit(identity *auth.Identity, action, pattern string) (allowed bool, reason string) {
	if c == nil || identity == nil {
		return true, ""
	}
	c.mu.RLock()
	cp := c.perms
	c.mu.RUnlock()
	if cp == nil {
		return true, ""
	}
	allowed, reason = cp.decide(identity, action, pattern)
	if allowed {
		return true, ""
	}
	caller := cp.callerLabel(identity)
	if cp.audit {
		topicPermissionRefusals.WithLabelValues(action, outcomeAudited, caller).Inc()
		log.Warn().
			Str("identity", identity.Subject).
			Str("action", action).
			Str("pattern", pattern).
			Str("reason", reason).
			Msg("topic_permission_refused_audit_only")
		return true, ""
	}
	topicPermissionRefusals.WithLabelValues(action, outcomeDenied, caller).Inc()
	return false, reason
}

// knows reports whether subject is an identity topics.permissions lists.
// It matches as decide does: without case or surrounding space.
func (cp *compiledPermissions) knows(subject string) bool {
	_, listed := cp.services[normalIdentity(subject)]
	return listed
}

// callerLabel is the refusal counter's caller for identity: the listed
// name, in the config's (lower-cased) spelling, for a service identity the
// section names, and callerOther for everything else. A user token or an
// API key that carries a listed name is "other": its subject proves no
// service (auth.Identity.ServiceSubject).
func (cp *compiledPermissions) callerLabel(identity *auth.Identity) string {
	subject := identity.ServiceSubject()
	if subject == "" || !cp.knows(subject) {
		return callerOther
	}
	return normalIdentity(subject)
}

// normalIdentity is how an identity name is keyed in compiledPermissions.
func normalIdentity(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (cp *compiledPermissions) decide(identity *auth.Identity, action, pattern string) (bool, string) {
	subject := identity.ServiceSubject()
	if subject == "" {
		return false, "not a service identity"
	}
	svc, listed := cp.services[normalIdentity(subject)]
	if !listed {
		if cp.unlistedAllow {
			return true, ""
		}
		return false, "identity not in topics.permissions"
	}
	switch action {
	case PermitSubscribe:
		for _, a := range svc.subscribe {
			if topicpattern.Covers(a, pattern) {
				return true, ""
			}
		}
		return false, "pattern outside the identity's subscribe list"
	case PermitPublish:
		if topicpattern.IsPattern(pattern) {
			return false, "cannot publish to a pattern"
		}
		for _, a := range svc.publish {
			if topicpattern.Match(a, pattern) {
				return true, ""
			}
		}
		return false, "topic outside the identity's publish list"
	default:
		return false, "unknown action"
	}
}

// PermissionsConfigured reports whether topics.permissions is present.
func (c *TopicConfig) PermissionsConfigured() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.perms != nil
}
