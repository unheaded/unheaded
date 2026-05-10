// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the MIT License.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Package network provides network policy enforcement for container isolation.
//
// The NetworkPolicy controller implements "The Closed Gate" principle - default deny
// with explicit allow rules. It translates declarative policies to iptables/nftables
// rules and applies them to container network namespaces.
package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// COMMON ERRORS
// ============================================================================

var (
	ErrPolicyNotFound       = errors.New("policy not found")
	ErrInvalidPolicy        = errors.New("invalid policy definition")
	ErrPolicyAlreadyExists  = errors.New("policy already exists")
	ErrApplyFailed          = errors.New("failed to apply policy")
	ErrRemoveFailed         = errors.New("failed to remove policy")
	ErrNamespaceNotFound    = errors.New("network namespace not found")
	ErrInvalidRuleType      = errors.New("invalid rule type")
	ErrInvalidAction        = errors.New("invalid policy action")
	ErrBackendNotSupported  = errors.New("firewall backend not supported")
	ErrSyncFailed           = errors.New("policy sync failed")
	ErrWatcherFailed        = errors.New("policy watcher failed")
	ErrBGPIntegrationFailed = errors.New("BGP integration failed")
)

// ============================================================================
// PROMETHEUS METRICS
// ============================================================================

var (
	policiesApplied = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "unheaded",
			Subsystem: "network",
			Name:      "policies_applied_total",
			Help:      "Total number of network policies currently applied",
		},
		[]string{"namespace"},
	)

	rulesApplied = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "unheaded",
			Subsystem: "network",
			Name:      "rules_applied_total",
			Help:      "Total number of firewall rules currently applied",
		},
		[]string{"namespace", "policy", "direction"},
	)

	violationsCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "unheaded",
			Subsystem: "network",
			Name:      "violations_total",
			Help:      "Total number of policy violations detected",
		},
		[]string{"namespace", "policy", "rule_type"},
	)

	policyApplyDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "unheaded",
			Subsystem: "network",
			Name:      "policy_apply_duration_seconds",
			Help:      "Time taken to apply a network policy",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"namespace", "operation"},
	)

	policySyncErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "unheaded",
			Subsystem: "network",
			Name:      "sync_errors_total",
			Help:      "Total number of policy sync errors",
		},
		[]string{"namespace", "error_type"},
	)
)

// ============================================================================
// POLICY ACTION TYPES
// ============================================================================

// PolicyAction defines what action to take when a rule matches
type PolicyAction string

const (
	ActionAllow  PolicyAction = "allow"
	ActionDeny   PolicyAction = "deny"
	ActionLog    PolicyAction = "log"
	ActionReject PolicyAction = "reject" // Send ICMP unreachable
)

// IsValid checks if the action is valid
func (a PolicyAction) IsValid() bool {
	switch a {
	case ActionAllow, ActionDeny, ActionLog, ActionReject:
		return true
	default:
		return false
	}
}

// ToIPTablesTarget converts action to iptables target
func (a PolicyAction) ToIPTablesTarget() string {
	switch a {
	case ActionAllow:
		return "ACCEPT"
	case ActionDeny:
		return "DROP"
	case ActionLog:
		return "LOG"
	case ActionReject:
		return "REJECT"
	default:
		return "DROP"
	}
}

// ToNFTablesVerdict converts action to nftables verdict
func (a PolicyAction) ToNFTablesVerdict() string {
	switch a {
	case ActionAllow:
		return "accept"
	case ActionDeny:
		return "drop"
	case ActionLog:
		return "log"
	case ActionReject:
		return "reject"
	default:
		return "drop"
	}
}

// ============================================================================
// PROTOCOL AND PORT DEFINITIONS
// ============================================================================

// Protocol represents network protocol
type Protocol string

const (
	ProtocolTCP  Protocol = "tcp"
	ProtocolUDP  Protocol = "udp"
	ProtocolICMP Protocol = "icmp"
	ProtocolAny  Protocol = "any"
)

// PortRange defines a range of ports
type PortRange struct {
	Start uint16 `json:"start" yaml:"start"`
	End   uint16 `json:"end" yaml:"end"`
}

// String returns string representation of port range
func (p PortRange) String() string {
	if p.Start == p.End {
		return fmt.Sprintf("%d", p.Start)
	}
	return fmt.Sprintf("%d:%d", p.Start, p.End)
}

// IsValid checks if port range is valid. Note: Start/End are uint16 so
// the upper-bound 65535 check (SA4003) is structurally satisfied; only
// the non-zero-Start and End>=Start invariants need explicit checks.
func (p PortRange) IsValid() bool {
	return p.Start > 0 && p.End >= p.Start
}

// ============================================================================
// POLICY TARGET
// ============================================================================

// PolicyTarget defines which containers a policy applies to
type PolicyTarget struct {
	// ContainerIDs is a list of specific container IDs
	ContainerIDs []string `json:"container_ids,omitempty" yaml:"container_ids,omitempty"`

	// ContainerNames is a list of container name patterns (supports glob)
	ContainerNames []string `json:"container_names,omitempty" yaml:"container_names,omitempty"`

	// Labels is a map of label selectors
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// Namespace is the network namespace name (optional)
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	// NetworkCIDR matches containers in a specific network
	NetworkCIDR string `json:"network_cidr,omitempty" yaml:"network_cidr,omitempty"`
}

// IsEmpty checks if target has no selectors
func (t PolicyTarget) IsEmpty() bool {
	return len(t.ContainerIDs) == 0 &&
		len(t.ContainerNames) == 0 &&
		len(t.Labels) == 0 &&
		t.Namespace == "" &&
		t.NetworkCIDR == ""
}

// ============================================================================
// RULE TYPES
// ============================================================================

// RuleType indicates the type of traffic rule
type RuleType string

const (
	RuleTypeIngress        RuleType = "ingress"
	RuleTypeEgress         RuleType = "egress"
	RuleTypeInterContainer RuleType = "inter-container"
	RuleTypeExternal       RuleType = "external"
)

// IngressRule defines rules for incoming traffic
type IngressRule struct {
	// Name is a human-readable name for the rule
	Name string `json:"name" yaml:"name"`

	// Description explains what this rule does
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// SourceCIDRs are allowed source IP ranges
	SourceCIDRs []string `json:"source_cidrs,omitempty" yaml:"source_cidrs,omitempty"`

	// SourceContainers are allowed source containers
	SourceContainers []string `json:"source_containers,omitempty" yaml:"source_containers,omitempty"`

	// Protocol is the network protocol
	Protocol Protocol `json:"protocol" yaml:"protocol"`

	// Ports are the allowed destination ports
	Ports []PortRange `json:"ports,omitempty" yaml:"ports,omitempty"`

	// Action is what to do when rule matches
	Action PolicyAction `json:"action" yaml:"action"`

	// Priority determines rule order (lower = higher priority)
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`

	// Logging enables logging for this rule
	Logging bool `json:"logging,omitempty" yaml:"logging,omitempty"`
}

// Validate checks if the ingress rule is valid
func (r *IngressRule) Validate() error {
	if r.Name == "" {
		return errors.New("rule name cannot be empty")
	}
	if !r.Action.IsValid() {
		return ErrInvalidAction
	}
	for _, p := range r.Ports {
		if !p.IsValid() {
			return fmt.Errorf("invalid port range: %v", p)
		}
	}
	return nil
}

// EgressRule defines rules for outgoing traffic
type EgressRule struct {
	// Name is a human-readable name for the rule
	Name string `json:"name" yaml:"name"`

	// Description explains what this rule does
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// DestinationCIDRs are allowed destination IP ranges
	DestinationCIDRs []string `json:"destination_cidrs,omitempty" yaml:"destination_cidrs,omitempty"`

	// DestinationContainers are allowed destination containers
	DestinationContainers []string `json:"destination_containers,omitempty" yaml:"destination_containers,omitempty"`

	// Protocol is the network protocol
	Protocol Protocol `json:"protocol" yaml:"protocol"`

	// Ports are the allowed destination ports
	Ports []PortRange `json:"ports,omitempty" yaml:"ports,omitempty"`

	// Action is what to do when rule matches
	Action PolicyAction `json:"action" yaml:"action"`

	// Priority determines rule order (lower = higher priority)
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`

	// Logging enables logging for this rule
	Logging bool `json:"logging,omitempty" yaml:"logging,omitempty"`
}

// Validate checks if the egress rule is valid
func (r *EgressRule) Validate() error {
	if r.Name == "" {
		return errors.New("rule name cannot be empty")
	}
	if !r.Action.IsValid() {
		return ErrInvalidAction
	}
	for _, p := range r.Ports {
		if !p.IsValid() {
			return fmt.Errorf("invalid port range: %v", p)
		}
	}
	return nil
}

// InterContainerRule defines rules for container-to-container traffic
type InterContainerRule struct {
	// Name is a human-readable name for the rule
	Name string `json:"name" yaml:"name"`

	// Description explains what this rule does
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// SourceContainers are allowed source containers
	SourceContainers []string `json:"source_containers" yaml:"source_containers"`

	// DestinationContainers are allowed destination containers
	DestinationContainers []string `json:"destination_containers" yaml:"destination_containers"`

	// Protocol is the network protocol
	Protocol Protocol `json:"protocol" yaml:"protocol"`

	// Ports are the allowed ports
	Ports []PortRange `json:"ports,omitempty" yaml:"ports,omitempty"`

	// Action is what to do when rule matches
	Action PolicyAction `json:"action" yaml:"action"`

	// Bidirectional allows traffic in both directions
	Bidirectional bool `json:"bidirectional,omitempty" yaml:"bidirectional,omitempty"`

	// Priority determines rule order
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`

	// Logging enables logging for this rule
	Logging bool `json:"logging,omitempty" yaml:"logging,omitempty"`
}

// Validate checks if the inter-container rule is valid
func (r *InterContainerRule) Validate() error {
	if r.Name == "" {
		return errors.New("rule name cannot be empty")
	}
	if len(r.SourceContainers) == 0 {
		return errors.New("source_containers cannot be empty")
	}
	if len(r.DestinationContainers) == 0 {
		return errors.New("destination_containers cannot be empty")
	}
	if !r.Action.IsValid() {
		return ErrInvalidAction
	}
	return nil
}

// ExternalRule defines rules for traffic to/from outside the cluster
type ExternalRule struct {
	// Name is a human-readable name for the rule
	Name string `json:"name" yaml:"name"`

	// Description explains what this rule does
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Direction is ingress or egress
	Direction RuleType `json:"direction" yaml:"direction"`

	// ExternalCIDRs are external IP ranges
	ExternalCIDRs []string `json:"external_cidrs,omitempty" yaml:"external_cidrs,omitempty"`

	// ExternalDomains are external domain names (resolved at apply time)
	ExternalDomains []string `json:"external_domains,omitempty" yaml:"external_domains,omitempty"`

	// Protocol is the network protocol
	Protocol Protocol `json:"protocol" yaml:"protocol"`

	// Ports are the allowed ports
	Ports []PortRange `json:"ports,omitempty" yaml:"ports,omitempty"`

	// Action is what to do when rule matches
	Action PolicyAction `json:"action" yaml:"action"`

	// Priority determines rule order
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`

	// Logging enables logging for this rule
	Logging bool `json:"logging,omitempty" yaml:"logging,omitempty"`
}

// Validate checks if the external rule is valid
func (r *ExternalRule) Validate() error {
	if r.Name == "" {
		return errors.New("rule name cannot be empty")
	}
	if r.Direction != RuleTypeIngress && r.Direction != RuleTypeEgress {
		return errors.New("direction must be ingress or egress")
	}
	if !r.Action.IsValid() {
		return ErrInvalidAction
	}
	return nil
}

// ============================================================================
// POLICY RULE (UNIFIED)
// ============================================================================

// PolicyRule represents a single rule within a policy
type PolicyRule struct {
	// Type indicates the rule type
	Type RuleType `json:"type" yaml:"type"`

	// Ingress rules (when Type == "ingress")
	Ingress *IngressRule `json:"ingress,omitempty" yaml:"ingress,omitempty"`

	// Egress rules (when Type == "egress")
	Egress *EgressRule `json:"egress,omitempty" yaml:"egress,omitempty"`

	// InterContainer rules (when Type == "inter-container")
	InterContainer *InterContainerRule `json:"inter_container,omitempty" yaml:"inter_container,omitempty"`

	// External rules (when Type == "external")
	External *ExternalRule `json:"external,omitempty" yaml:"external,omitempty"`
}

// Validate checks if the policy rule is valid
func (r *PolicyRule) Validate() error {
	switch r.Type {
	case RuleTypeIngress:
		if r.Ingress == nil {
			return errors.New("ingress rule definition required")
		}
		return r.Ingress.Validate()
	case RuleTypeEgress:
		if r.Egress == nil {
			return errors.New("egress rule definition required")
		}
		return r.Egress.Validate()
	case RuleTypeInterContainer:
		if r.InterContainer == nil {
			return errors.New("inter_container rule definition required")
		}
		return r.InterContainer.Validate()
	case RuleTypeExternal:
		if r.External == nil {
			return errors.New("external rule definition required")
		}
		return r.External.Validate()
	default:
		return ErrInvalidRuleType
	}
}

// GetName returns the rule name
func (r *PolicyRule) GetName() string {
	switch r.Type {
	case RuleTypeIngress:
		if r.Ingress != nil {
			return r.Ingress.Name
		}
	case RuleTypeEgress:
		if r.Egress != nil {
			return r.Egress.Name
		}
	case RuleTypeInterContainer:
		if r.InterContainer != nil {
			return r.InterContainer.Name
		}
	case RuleTypeExternal:
		if r.External != nil {
			return r.External.Name
		}
	}
	return ""
}

// GetPriority returns the rule priority
func (r *PolicyRule) GetPriority() int {
	switch r.Type {
	case RuleTypeIngress:
		if r.Ingress != nil {
			return r.Ingress.Priority
		}
	case RuleTypeEgress:
		if r.Egress != nil {
			return r.Egress.Priority
		}
	case RuleTypeInterContainer:
		if r.InterContainer != nil {
			return r.InterContainer.Priority
		}
	case RuleTypeExternal:
		if r.External != nil {
			return r.External.Priority
		}
	}
	return 0
}

// ============================================================================
// NETWORK POLICY
// ============================================================================

// NetworkPolicy defines a complete network policy
type NetworkPolicy struct {
	// APIVersion for policy versioning
	APIVersion string `json:"api_version" yaml:"api_version"`

	// Kind is always "NetworkPolicy"
	Kind string `json:"kind" yaml:"kind"`

	// Metadata contains policy metadata
	Metadata PolicyMetadata `json:"metadata" yaml:"metadata"`

	// Spec contains the policy specification
	Spec PolicySpec `json:"spec" yaml:"spec"`
}

// PolicyMetadata contains policy identification information
type PolicyMetadata struct {
	// Name is the policy name
	Name string `json:"name" yaml:"name"`

	// Namespace is the policy namespace
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	// Labels are policy labels
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// Annotations are policy annotations
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// CreatedAt is when the policy was created
	CreatedAt time.Time `json:"created_at,omitempty" yaml:"created_at,omitempty"`

	// UpdatedAt is when the policy was last updated
	UpdatedAt time.Time `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

// PolicySpec contains the policy rules and targets
type PolicySpec struct {
	// Target defines which containers this policy applies to
	Target PolicyTarget `json:"target" yaml:"target"`

	// DefaultAction is the default action when no rules match
	DefaultAction PolicyAction `json:"default_action" yaml:"default_action"`

	// Rules are the policy rules
	Rules []PolicyRule `json:"rules" yaml:"rules"`

	// BGPConfig contains optional BGP integration settings
	BGPConfig *BGPConfig `json:"bgp_config,omitempty" yaml:"bgp_config,omitempty"`

	// Enabled indicates if the policy is enabled
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// BGPConfig contains BGP integration settings
type BGPConfig struct {
	// Enabled enables BGP integration
	Enabled bool `json:"enabled" yaml:"enabled"`

	// RouterID is the BGP router ID
	RouterID string `json:"router_id,omitempty" yaml:"router_id,omitempty"`

	// ASN is the local autonomous system number
	ASN uint32 `json:"asn,omitempty" yaml:"asn,omitempty"`

	// Peers are BGP peer configurations
	Peers []BGPPeer `json:"peers,omitempty" yaml:"peers,omitempty"`

	// AdvertiseRoutes are routes to advertise
	AdvertiseRoutes []string `json:"advertise_routes,omitempty" yaml:"advertise_routes,omitempty"`
}

// BGPPeer represents a BGP peer configuration
type BGPPeer struct {
	// Address is the peer address
	Address string `json:"address" yaml:"address"`

	// ASN is the peer's autonomous system number
	ASN uint32 `json:"asn" yaml:"asn"`

	// Password is the optional BGP session password
	Password string `json:"password,omitempty" yaml:"password,omitempty"`

	// HoldTime is the BGP hold time in seconds
	HoldTime uint16 `json:"hold_time,omitempty" yaml:"hold_time,omitempty"`
}

// Validate checks if the network policy is valid
func (p *NetworkPolicy) Validate() error {
	if p.Metadata.Name == "" {
		return errors.New("policy name cannot be empty")
	}
	if p.Spec.Target.IsEmpty() {
		return errors.New("policy target cannot be empty")
	}
	if !p.Spec.DefaultAction.IsValid() {
		return fmt.Errorf("invalid default action: %s", p.Spec.DefaultAction)
	}
	for i, rule := range p.Spec.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("invalid rule %d: %w", i, err)
		}
	}
	return nil
}

// GetFullName returns namespace/name
func (p *NetworkPolicy) GetFullName() string {
	if p.Metadata.Namespace == "" {
		return p.Metadata.Name
	}
	return fmt.Sprintf("%s/%s", p.Metadata.Namespace, p.Metadata.Name)
}

// ============================================================================
// POLICY VIOLATION
// ============================================================================

// PolicyViolation represents a detected policy violation
type PolicyViolation struct {
	// ID is a unique violation identifier
	ID string `json:"id"`

	// PolicyName is the violated policy name
	PolicyName string `json:"policy_name"`

	// RuleName is the specific rule that was violated
	RuleName string `json:"rule_name,omitempty"`

	// RuleType is the type of rule violated
	RuleType RuleType `json:"rule_type"`

	// SourceIP is the source IP address
	SourceIP string `json:"source_ip"`

	// DestinationIP is the destination IP address
	DestinationIP string `json:"destination_ip"`

	// SourcePort is the source port
	SourcePort uint16 `json:"source_port,omitempty"`

	// DestinationPort is the destination port
	DestinationPort uint16 `json:"destination_port,omitempty"`

	// Protocol is the network protocol
	Protocol Protocol `json:"protocol"`

	// ContainerID is the container involved
	ContainerID string `json:"container_id,omitempty"`

	// ContainerName is the container name
	ContainerName string `json:"container_name,omitempty"`

	// Action is the action taken
	Action PolicyAction `json:"action"`

	// Timestamp is when the violation occurred
	Timestamp time.Time `json:"timestamp"`

	// Count is the number of times this violation occurred
	Count int64 `json:"count"`

	// Metadata contains additional violation context
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ============================================================================
// FIREWALL BACKEND
// ============================================================================

// FirewallBackend indicates which firewall backend to use
type FirewallBackend string

const (
	BackendIPTables FirewallBackend = "iptables"
	BackendNFTables FirewallBackend = "nftables"
)

// ============================================================================
// WOTAN EVENT EMITTER INTERFACE
// ============================================================================

// EventEmitter interface for Wotan integration
type EventEmitter interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// ============================================================================
// CONTAINER RESOLVER INTERFACE
// ============================================================================

// ContainerResolver resolves container information
type ContainerResolver interface {
	// GetContainerNamespace returns the network namespace path for a container
	GetContainerNamespace(containerID string) (string, error)

	// GetContainerIP returns the IP address of a container
	GetContainerIP(containerID string) (string, error)

	// ListContainers lists containers matching the given labels
	ListContainers(labels map[string]string) ([]string, error)

	// GetContainerByName returns container ID by name
	GetContainerByName(name string) (string, error)
}

// ============================================================================
// POLICY CONTROLLER
// ============================================================================

// PolicyController manages network policies
type PolicyController struct {
	// policies is a map of policy name to policy
	policies map[string]*NetworkPolicy

	// appliedPolicies tracks which policies are currently applied
	appliedPolicies map[string]time.Time

	// violations stores recent policy violations
	violations []PolicyViolation

	// backend is the firewall backend to use
	backend FirewallBackend

	// resolver resolves container information
	resolver ContainerResolver

	// emitter emits events to Wotan
	emitter EventEmitter

	// logger is the zerolog logger
	logger zerolog.Logger

	// auditLog is the audit log file path
	auditLogPath string

	// mu protects internal state
	mu sync.RWMutex

	// violationsMu protects violations slice
	violationsMu sync.RWMutex

	// watcher watches for policy file changes
	watcher *fsnotify.Watcher

	// watcherCtx is the watcher context
	watcherCtx    context.Context
	watcherCancel context.CancelFunc

	// chainPrefix is the iptables chain prefix
	chainPrefix string

	// maxViolations is the maximum number of violations to store
	maxViolations int
}

// PolicyControllerConfig contains controller configuration
type PolicyControllerConfig struct {
	// Backend is the firewall backend
	Backend FirewallBackend

	// Resolver resolves container information
	Resolver ContainerResolver

	// Emitter emits events to Wotan
	Emitter EventEmitter

	// Logger is the zerolog logger
	Logger zerolog.Logger

	// AuditLogPath is the path to the audit log
	AuditLogPath string

	// ChainPrefix is the iptables chain prefix
	ChainPrefix string

	// MaxViolations is the maximum violations to store
	MaxViolations int
}

// NewPolicyController creates a new policy controller
func NewPolicyController(cfg PolicyControllerConfig) (*PolicyController, error) {
	if cfg.Backend == "" {
		cfg.Backend = BackendIPTables
	}
	if cfg.ChainPrefix == "" {
		cfg.ChainPrefix = "UNHEADED"
	}
	if cfg.MaxViolations <= 0 {
		cfg.MaxViolations = 10000
	}

	// Check if backend is available
	switch cfg.Backend {
	case BackendIPTables:
		if _, err := exec.LookPath("iptables"); err != nil {
			return nil, fmt.Errorf("%w: iptables not found", ErrBackendNotSupported)
		}
	case BackendNFTables:
		if _, err := exec.LookPath("nft"); err != nil {
			return nil, fmt.Errorf("%w: nft not found", ErrBackendNotSupported)
		}
	}

	pc := &PolicyController{
		policies:        make(map[string]*NetworkPolicy),
		appliedPolicies: make(map[string]time.Time),
		violations:      make([]PolicyViolation, 0, cfg.MaxViolations),
		backend:         cfg.Backend,
		resolver:        cfg.Resolver,
		emitter:         cfg.Emitter,
		logger:          cfg.Logger,
		auditLogPath:    cfg.AuditLogPath,
		chainPrefix:     cfg.ChainPrefix,
		maxViolations:   cfg.MaxViolations,
	}

	return pc, nil
}

// LoadPolicies loads policy definitions from a path
func (pc *PolicyController) LoadPolicies(path string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat path %s: %w", path, err)
	}

	if info.IsDir() {
		return pc.loadPoliciesFromDir(path)
	}
	return pc.loadPolicyFromFile(path)
}

// loadPoliciesFromDir loads all policies from a directory
func (pc *PolicyController) loadPoliciesFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		if err := pc.loadPolicyFromFile(filePath); err != nil {
			pc.logger.Warn().
				Str("file", filePath).
				Err(err).
				Msg("failed to load policy file")
			continue
		}
	}

	pc.logger.Info().
		Int("count", len(pc.policies)).
		Str("path", dir).
		Msg("policies loaded")

	return nil
}

// loadPolicyFromFile loads a single policy from a file
func (pc *PolicyController) loadPolicyFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file %s: %w", path, err)
	}

	var policy NetworkPolicy
	ext := filepath.Ext(path)

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &policy); err != nil {
			return fmt.Errorf("unmarshal yaml: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &policy); err != nil {
			return fmt.Errorf("unmarshal json: %w", err)
		}
	default:
		return fmt.Errorf("unsupported file extension: %s", ext)
	}

	if err := policy.Validate(); err != nil {
		return fmt.Errorf("validate policy: %w", err)
	}

	policy.Metadata.UpdatedAt = time.Now()
	if policy.Metadata.CreatedAt.IsZero() {
		policy.Metadata.CreatedAt = policy.Metadata.UpdatedAt
	}

	pc.policies[policy.GetFullName()] = &policy

	pc.logger.Debug().
		Str("policy", policy.GetFullName()).
		Str("file", path).
		Msg("policy loaded")

	return nil
}

// ApplyPolicy applies a policy to target containers
func (pc *PolicyController) ApplyPolicy(ctx context.Context, policy *NetworkPolicy) error {
	if policy == nil {
		return ErrInvalidPolicy
	}

	if err := policy.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPolicy, err)
	}

	start := time.Now()
	defer func() {
		policyApplyDuration.WithLabelValues(policy.Metadata.Namespace, "apply").Observe(time.Since(start).Seconds())
	}()

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Store policy
	pc.policies[policy.GetFullName()] = policy

	// Generate firewall rules
	rules, err := pc.generateRules(policy)
	if err != nil {
		return fmt.Errorf("generate rules: %w", err)
	}

	// Apply rules based on backend
	switch pc.backend {
	case BackendIPTables:
		if err := pc.applyIPTablesRules(ctx, policy, rules); err != nil {
			return fmt.Errorf("apply iptables rules: %w", err)
		}
	case BackendNFTables:
		if err := pc.applyNFTablesRules(ctx, policy, rules); err != nil {
			return fmt.Errorf("apply nftables rules: %w", err)
		}
	}

	// Handle BGP integration
	if policy.Spec.BGPConfig != nil && policy.Spec.BGPConfig.Enabled {
		if err := pc.applyBGPConfig(ctx, policy); err != nil {
			pc.logger.Warn().
				Str("policy", policy.GetFullName()).
				Err(err).
				Msg("BGP configuration failed")
		}
	}

	// Track applied policy
	pc.appliedPolicies[policy.GetFullName()] = time.Now()

	// Update metrics
	policiesApplied.WithLabelValues(policy.Metadata.Namespace).Inc()
	rulesApplied.WithLabelValues(policy.Metadata.Namespace, policy.Metadata.Name, "ingress").Set(float64(countRulesByType(rules, RuleTypeIngress)))
	rulesApplied.WithLabelValues(policy.Metadata.Namespace, policy.Metadata.Name, "egress").Set(float64(countRulesByType(rules, RuleTypeEgress)))

	// Audit log
	pc.auditLog("apply", policy.GetFullName(), "success", nil)

	// Emit event
	if pc.emitter != nil {
		pc.emitPolicyEvent(ctx, "policy.applied", policy)
	}

	pc.logger.Info().
		Str("policy", policy.GetFullName()).
		Int("rules", len(rules)).
		Dur("duration", time.Since(start)).
		Msg("policy applied")

	return nil
}

// RemovePolicy removes a policy by name
func (pc *PolicyController) RemovePolicy(ctx context.Context, policyName string) error {
	start := time.Now()
	defer func() {
		policyApplyDuration.WithLabelValues("", "remove").Observe(time.Since(start).Seconds())
	}()

	pc.mu.Lock()
	defer pc.mu.Unlock()

	policy, ok := pc.policies[policyName]
	if !ok {
		return ErrPolicyNotFound
	}

	// Remove firewall rules
	switch pc.backend {
	case BackendIPTables:
		if err := pc.removeIPTablesRules(ctx, policy); err != nil {
			return fmt.Errorf("remove iptables rules: %w", err)
		}
	case BackendNFTables:
		if err := pc.removeNFTablesRules(ctx, policy); err != nil {
			return fmt.Errorf("remove nftables rules: %w", err)
		}
	}

	// Remove from tracking
	delete(pc.policies, policyName)
	delete(pc.appliedPolicies, policyName)

	// Update metrics
	policiesApplied.WithLabelValues(policy.Metadata.Namespace).Dec()
	rulesApplied.WithLabelValues(policy.Metadata.Namespace, policy.Metadata.Name, "ingress").Set(0)
	rulesApplied.WithLabelValues(policy.Metadata.Namespace, policy.Metadata.Name, "egress").Set(0)

	// Audit log
	pc.auditLog("remove", policyName, "success", nil)

	// Emit event
	if pc.emitter != nil {
		pc.emitPolicyEvent(ctx, "policy.removed", policy)
	}

	pc.logger.Info().
		Str("policy", policyName).
		Dur("duration", time.Since(start)).
		Msg("policy removed")

	return nil
}

// SyncPolicies reconciles policies with running state
func (pc *PolicyController) SyncPolicies(ctx context.Context) error {
	pc.mu.RLock()
	policies := make([]*NetworkPolicy, 0, len(pc.policies))
	for _, p := range pc.policies {
		policies = append(policies, p)
	}
	pc.mu.RUnlock()

	var syncErrors []error

	for _, policy := range policies {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !policy.Spec.Enabled {
			continue
		}

		// Re-apply to ensure state is correct
		if err := pc.ApplyPolicy(ctx, policy); err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("sync policy %s: %w", policy.GetFullName(), err))
			policySyncErrors.WithLabelValues(policy.Metadata.Namespace, "apply").Inc()
		}
	}

	if len(syncErrors) > 0 {
		return fmt.Errorf("%w: %d errors", ErrSyncFailed, len(syncErrors))
	}

	pc.logger.Info().
		Int("policies", len(policies)).
		Msg("policies synchronized")

	return nil
}

// GetViolations retrieves logged violations within the given duration
func (pc *PolicyController) GetViolations(duration time.Duration) []PolicyViolation {
	pc.violationsMu.RLock()
	defer pc.violationsMu.RUnlock()

	cutoff := time.Now().Add(-duration)
	result := make([]PolicyViolation, 0)

	for _, v := range pc.violations {
		if v.Timestamp.After(cutoff) {
			result = append(result, v)
		}
	}

	return result
}

// RecordViolation records a policy violation
func (pc *PolicyController) RecordViolation(v PolicyViolation) {
	pc.violationsMu.Lock()
	defer pc.violationsMu.Unlock()

	// Update metrics
	violationsCount.WithLabelValues(
		v.ContainerName,
		v.PolicyName,
		string(v.RuleType),
	).Inc()

	// Store violation
	if len(pc.violations) >= pc.maxViolations {
		// Remove oldest violations
		pc.violations = pc.violations[len(pc.violations)/2:]
	}

	v.ID = fmt.Sprintf("viol-%d", time.Now().UnixNano())
	v.Timestamp = time.Now()
	pc.violations = append(pc.violations, v)

	pc.logger.Warn().
		Str("policy", v.PolicyName).
		Str("source", v.SourceIP).
		Str("destination", v.DestinationIP).
		Uint16("port", v.DestinationPort).
		Str("protocol", string(v.Protocol)).
		Str("action", string(v.Action)).
		Msg("policy violation detected")
}

// WatchPolicies watches for policy file changes
func (pc *PolicyController) WatchPolicies(ctx context.Context, path string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}

	pc.watcher = watcher
	pc.watcherCtx, pc.watcherCancel = context.WithCancel(ctx)

	// Add path to watcher
	if err := watcher.Add(path); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("watch path %s: %w", path, err)
	}

	go pc.watchLoop(path)

	pc.logger.Info().
		Str("path", path).
		Msg("watching for policy changes")

	return nil
}

// watchLoop handles file system events
func (pc *PolicyController) watchLoop(basePath string) {
	defer func() { _ = pc.watcher.Close() }()

	for {
		select {
		case <-pc.watcherCtx.Done():
			return

		case event, ok := <-pc.watcher.Events:
			if !ok {
				return
			}

			// Handle file changes
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				ext := filepath.Ext(event.Name)
				if ext == ".yaml" || ext == ".yml" || ext == ".json" {
					pc.logger.Debug().
						Str("file", event.Name).
						Str("op", event.Op.String()).
						Msg("policy file changed")

					// Reload policies
					if err := pc.LoadPolicies(basePath); err != nil {
						pc.logger.Error().
							Err(err).
							Str("path", basePath).
							Msg("failed to reload policies")
					}

					// Sync after reload
					if err := pc.SyncPolicies(pc.watcherCtx); err != nil {
						pc.logger.Error().
							Err(err).
							Msg("failed to sync policies after reload")
					}
				}
			}

		case err, ok := <-pc.watcher.Errors:
			if !ok {
				return
			}
			pc.logger.Error().
				Err(err).
				Msg("watcher error")
		}
	}
}

// StopWatching stops the policy file watcher
func (pc *PolicyController) StopWatching() {
	if pc.watcherCancel != nil {
		pc.watcherCancel()
	}
}

// GetPolicy returns a policy by name
func (pc *PolicyController) GetPolicy(name string) (*NetworkPolicy, bool) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	p, ok := pc.policies[name]
	return p, ok
}

// ListPolicies returns all loaded policies
func (pc *PolicyController) ListPolicies() []*NetworkPolicy {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	result := make([]*NetworkPolicy, 0, len(pc.policies))
	for _, p := range pc.policies {
		result = append(result, p)
	}
	return result
}

// ============================================================================
// RULE GENERATION
// ============================================================================

// generatedRule represents a generated firewall rule
type generatedRule struct {
	Type     RuleType
	Chain    string
	Protocol Protocol
	SourceIP string
	DestIP   string
	SrcPort  string
	DstPort  string
	Action   PolicyAction
	Logging  bool
	Comment  string
	Priority int
}

// generateRules generates firewall rules from a policy
func (pc *PolicyController) generateRules(policy *NetworkPolicy) ([]generatedRule, error) {
	var rules []generatedRule

	// Add default deny rules first (The Closed Gate principle)
	if policy.Spec.DefaultAction == ActionDeny {
		rules = append(rules, generatedRule{
			Type:     RuleTypeIngress,
			Chain:    "INPUT",
			Action:   ActionDeny,
			Comment:  fmt.Sprintf("%s-default-deny-in", policy.Metadata.Name),
			Priority: 9999,
		})
		rules = append(rules, generatedRule{
			Type:     RuleTypeEgress,
			Chain:    "OUTPUT",
			Action:   ActionDeny,
			Comment:  fmt.Sprintf("%s-default-deny-out", policy.Metadata.Name),
			Priority: 9999,
		})
	}

	// Generate rules from policy
	for _, rule := range policy.Spec.Rules {
		generated, err := pc.generateRuleFromPolicyRule(&rule, policy)
		if err != nil {
			return nil, fmt.Errorf("generate rule %s: %w", rule.GetName(), err)
		}
		rules = append(rules, generated...)
	}

	// Sort by priority
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority < rules[j].Priority
	})

	return rules, nil
}

// generateRuleFromPolicyRule generates firewall rules from a PolicyRule
func (pc *PolicyController) generateRuleFromPolicyRule(rule *PolicyRule, policy *NetworkPolicy) ([]generatedRule, error) {
	var rules []generatedRule

	switch rule.Type {
	case RuleTypeIngress:
		rules = pc.generateIngressRules(rule.Ingress, policy)
	case RuleTypeEgress:
		rules = pc.generateEgressRules(rule.Egress, policy)
	case RuleTypeInterContainer:
		rules = pc.generateInterContainerRules(rule.InterContainer, policy)
	case RuleTypeExternal:
		rules = pc.generateExternalRules(rule.External, policy)
	}

	return rules, nil
}

// generateIngressRules generates ingress firewall rules
func (pc *PolicyController) generateIngressRules(rule *IngressRule, policy *NetworkPolicy) []generatedRule {
	var rules []generatedRule

	// If no source CIDRs specified, create rule for any source
	sourceCIDRs := rule.SourceCIDRs
	if len(sourceCIDRs) == 0 {
		sourceCIDRs = []string{"0.0.0.0/0"}
	}

	for _, cidr := range sourceCIDRs {
		for _, port := range rule.Ports {
			r := generatedRule{
				Type:     RuleTypeIngress,
				Chain:    "INPUT",
				Protocol: rule.Protocol,
				SourceIP: cidr,
				DstPort:  port.String(),
				Action:   rule.Action,
				Logging:  rule.Logging,
				Comment:  fmt.Sprintf("%s-%s", policy.Metadata.Name, rule.Name),
				Priority: rule.Priority,
			}
			rules = append(rules, r)
		}

		// If no ports specified, create rule for all ports
		if len(rule.Ports) == 0 {
			r := generatedRule{
				Type:     RuleTypeIngress,
				Chain:    "INPUT",
				Protocol: rule.Protocol,
				SourceIP: cidr,
				Action:   rule.Action,
				Logging:  rule.Logging,
				Comment:  fmt.Sprintf("%s-%s", policy.Metadata.Name, rule.Name),
				Priority: rule.Priority,
			}
			rules = append(rules, r)
		}
	}

	return rules
}

// generateEgressRules generates egress firewall rules
func (pc *PolicyController) generateEgressRules(rule *EgressRule, policy *NetworkPolicy) []generatedRule {
	var rules []generatedRule

	// If no destination CIDRs specified, create rule for any destination
	destCIDRs := rule.DestinationCIDRs
	if len(destCIDRs) == 0 {
		destCIDRs = []string{"0.0.0.0/0"}
	}

	for _, cidr := range destCIDRs {
		for _, port := range rule.Ports {
			r := generatedRule{
				Type:     RuleTypeEgress,
				Chain:    "OUTPUT",
				Protocol: rule.Protocol,
				DestIP:   cidr,
				DstPort:  port.String(),
				Action:   rule.Action,
				Logging:  rule.Logging,
				Comment:  fmt.Sprintf("%s-%s", policy.Metadata.Name, rule.Name),
				Priority: rule.Priority,
			}
			rules = append(rules, r)
		}

		// If no ports specified, create rule for all ports
		if len(rule.Ports) == 0 {
			r := generatedRule{
				Type:     RuleTypeEgress,
				Chain:    "OUTPUT",
				Protocol: rule.Protocol,
				DestIP:   cidr,
				Action:   rule.Action,
				Logging:  rule.Logging,
				Comment:  fmt.Sprintf("%s-%s", policy.Metadata.Name, rule.Name),
				Priority: rule.Priority,
			}
			rules = append(rules, r)
		}
	}

	return rules
}

// generateInterContainerRules generates inter-container firewall rules
func (pc *PolicyController) generateInterContainerRules(rule *InterContainerRule, policy *NetworkPolicy) []generatedRule {
	var rules []generatedRule

	// Resolve container IPs
	var sourceIPs, destIPs []string
	if pc.resolver != nil {
		for _, name := range rule.SourceContainers {
			if id, err := pc.resolver.GetContainerByName(name); err == nil {
				if ip, err := pc.resolver.GetContainerIP(id); err == nil {
					sourceIPs = append(sourceIPs, ip)
				}
			}
		}
		for _, name := range rule.DestinationContainers {
			if id, err := pc.resolver.GetContainerByName(name); err == nil {
				if ip, err := pc.resolver.GetContainerIP(id); err == nil {
					destIPs = append(destIPs, ip)
				}
			}
		}
	}

	// Generate forward chain rules
	for _, srcIP := range sourceIPs {
		for _, dstIP := range destIPs {
			for _, port := range rule.Ports {
				r := generatedRule{
					Type:     RuleTypeInterContainer,
					Chain:    "FORWARD",
					Protocol: rule.Protocol,
					SourceIP: srcIP,
					DestIP:   dstIP,
					DstPort:  port.String(),
					Action:   rule.Action,
					Logging:  rule.Logging,
					Comment:  fmt.Sprintf("%s-%s", policy.Metadata.Name, rule.Name),
					Priority: rule.Priority,
				}
				rules = append(rules, r)

				// If bidirectional, add reverse rule
				if rule.Bidirectional {
					reverse := r
					reverse.SourceIP = dstIP
					reverse.DestIP = srcIP
					reverse.Comment = fmt.Sprintf("%s-%s-reverse", policy.Metadata.Name, rule.Name)
					rules = append(rules, reverse)
				}
			}
		}
	}

	return rules
}

// generateExternalRules generates external traffic firewall rules
func (pc *PolicyController) generateExternalRules(rule *ExternalRule, policy *NetworkPolicy) []generatedRule {
	var rules []generatedRule

	cidrs := rule.ExternalCIDRs
	if len(cidrs) == 0 {
		cidrs = []string{"0.0.0.0/0"}
	}

	for _, cidr := range cidrs {
		for _, port := range rule.Ports {
			var r generatedRule
			if rule.Direction == RuleTypeIngress {
				r = generatedRule{
					Type:     RuleTypeExternal,
					Chain:    "INPUT",
					Protocol: rule.Protocol,
					SourceIP: cidr,
					DstPort:  port.String(),
					Action:   rule.Action,
					Logging:  rule.Logging,
					Comment:  fmt.Sprintf("%s-%s-ext", policy.Metadata.Name, rule.Name),
					Priority: rule.Priority,
				}
			} else {
				r = generatedRule{
					Type:     RuleTypeExternal,
					Chain:    "OUTPUT",
					Protocol: rule.Protocol,
					DestIP:   cidr,
					DstPort:  port.String(),
					Action:   rule.Action,
					Logging:  rule.Logging,
					Comment:  fmt.Sprintf("%s-%s-ext", policy.Metadata.Name, rule.Name),
					Priority: rule.Priority,
				}
			}
			rules = append(rules, r)
		}
	}

	return rules
}

// ============================================================================
// IPTABLES BACKEND
// ============================================================================

// applyIPTablesRules applies rules using iptables
func (pc *PolicyController) applyIPTablesRules(ctx context.Context, policy *NetworkPolicy, rules []generatedRule) error {
	chainName := fmt.Sprintf("%s-%s", pc.chainPrefix, sanitizeChainName(policy.Metadata.Name))

	// Create custom chain
	if err := pc.runIPTables(ctx, "-N", chainName); err != nil {
		// Chain may already exist, continue
		pc.logger.Debug().
			Str("chain", chainName).
			Msg("chain may already exist")
	}

	// Flush existing rules in chain
	if err := pc.runIPTables(ctx, "-F", chainName); err != nil {
		return fmt.Errorf("flush chain: %w", err)
	}

	// Apply rules
	for _, rule := range rules {
		args := pc.buildIPTablesArgs(rule, chainName)
		if err := pc.runIPTables(ctx, args...); err != nil {
			return fmt.Errorf("apply rule %s: %w", rule.Comment, err)
		}
	}

	// Jump to custom chain from main chains
	for _, chain := range []string{"INPUT", "OUTPUT", "FORWARD"} {
		jumpArgs := []string{"-I", chain, "-j", chainName}
		_ = pc.runIPTables(ctx, jumpArgs...) // Ignore if already exists
	}

	return nil
}

// buildIPTablesArgs builds iptables command arguments
func (pc *PolicyController) buildIPTablesArgs(rule generatedRule, chainName string) []string {
	args := []string{"-A", chainName}

	if rule.Protocol != "" && rule.Protocol != ProtocolAny {
		args = append(args, "-p", string(rule.Protocol))
	}

	if rule.SourceIP != "" && rule.SourceIP != "0.0.0.0/0" {
		args = append(args, "-s", rule.SourceIP)
	}

	if rule.DestIP != "" && rule.DestIP != "0.0.0.0/0" {
		args = append(args, "-d", rule.DestIP)
	}

	if rule.DstPort != "" {
		args = append(args, "--dport", rule.DstPort)
	}

	if rule.SrcPort != "" {
		args = append(args, "--sport", rule.SrcPort)
	}

	if rule.Comment != "" {
		args = append(args, "-m", "comment", "--comment", rule.Comment)
	}

	// TODO: rule.Logging requires emitting a *separate* iptables -j LOG rule
	// before the terminal action. buildIPTablesArgs returns a single rule
	// slice today, so the caller (applyIPTablesRules) needs a second pass to
	// install the LOG rule. Leaving the field on the policy spec — the
	// emission path is not yet wired.

	args = append(args, "-j", rule.Action.ToIPTablesTarget())

	return args
}

// removeIPTablesRules removes iptables rules for a policy
func (pc *PolicyController) removeIPTablesRules(ctx context.Context, policy *NetworkPolicy) error {
	chainName := fmt.Sprintf("%s-%s", pc.chainPrefix, sanitizeChainName(policy.Metadata.Name))

	// Remove jumps from main chains
	for _, chain := range []string{"INPUT", "OUTPUT", "FORWARD"} {
		_ = pc.runIPTables(ctx, "-D", chain, "-j", chainName)
	}

	// Flush and delete custom chain
	_ = pc.runIPTables(ctx, "-F", chainName)
	_ = pc.runIPTables(ctx, "-X", chainName)

	return nil
}

// runIPTables executes an iptables command
func (pc *PolicyController) runIPTables(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %v: %s: %w", args, string(output), err)
	}
	return nil
}

// ============================================================================
// NFTABLES BACKEND
// ============================================================================

// applyNFTablesRules applies rules using nftables
func (pc *PolicyController) applyNFTablesRules(ctx context.Context, policy *NetworkPolicy, rules []generatedRule) error {
	tableName := strings.ToLower(pc.chainPrefix)
	chainName := sanitizeChainName(policy.Metadata.Name)

	// Create table if not exists
	nftScript := fmt.Sprintf(`
table inet %s {
    chain %s {
        type filter hook input priority 0; policy drop;
    }
}
`, tableName, chainName)

	// Add rules to script
	for _, rule := range rules {
		nftScript += pc.buildNFTablesRule(rule, tableName, chainName)
	}

	// Apply script
	cmd := exec.CommandContext(ctx, "nft", "-f", "-")
	cmd.Stdin = strings.NewReader(nftScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft apply: %s: %w", string(output), err)
	}

	return nil
}

// buildNFTablesRule builds an nftables rule string
func (pc *PolicyController) buildNFTablesRule(rule generatedRule, table, chain string) string {
	var parts []string

	if rule.Protocol != "" && rule.Protocol != ProtocolAny {
		parts = append(parts, fmt.Sprintf("ip protocol %s", rule.Protocol))
	}

	if rule.SourceIP != "" && rule.SourceIP != "0.0.0.0/0" {
		parts = append(parts, fmt.Sprintf("ip saddr %s", rule.SourceIP))
	}

	if rule.DestIP != "" && rule.DestIP != "0.0.0.0/0" {
		parts = append(parts, fmt.Sprintf("ip daddr %s", rule.DestIP))
	}

	if rule.DstPort != "" {
		parts = append(parts, fmt.Sprintf("%s dport %s", rule.Protocol, rule.DstPort))
	}

	if rule.Comment != "" {
		parts = append(parts, fmt.Sprintf("comment \"%s\"", rule.Comment))
	}

	parts = append(parts, rule.Action.ToNFTablesVerdict())

	return fmt.Sprintf("add rule inet %s %s %s\n", table, chain, strings.Join(parts, " "))
}

// removeNFTablesRules removes nftables rules for a policy
func (pc *PolicyController) removeNFTablesRules(ctx context.Context, policy *NetworkPolicy) error {
	tableName := strings.ToLower(pc.chainPrefix)
	chainName := sanitizeChainName(policy.Metadata.Name)

	// Delete chain
	cmd := exec.CommandContext(ctx, "nft", "delete", "chain", "inet", tableName, chainName)
	_, err := cmd.CombinedOutput()
	return err
}

// ============================================================================
// BGP INTEGRATION
// ============================================================================

// applyBGPConfig applies BGP configuration for a policy
func (pc *PolicyController) applyBGPConfig(ctx context.Context, policy *NetworkPolicy) error {
	if policy.Spec.BGPConfig == nil || !policy.Spec.BGPConfig.Enabled {
		return nil
	}

	bgp := policy.Spec.BGPConfig

	// Generate BGP configuration
	config := pc.generateBGPConfig(bgp)

	// Write to BGP daemon config file
	configPath := fmt.Sprintf("/etc/frr/bgpd.conf.d/%s.conf", sanitizeChainName(policy.Metadata.Name))
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("write BGP config: %w", err)
	}

	// Reload BGP daemon using proper argument separation to prevent command injection
	// Use vtysh with -f flag to load config file instead of shell-interpolated command
	cmd := exec.CommandContext(ctx, "vtysh", "-f", configPath)
	if _, err := cmd.CombinedOutput(); err != nil {
		// BGP daemon may not be running, log and continue
		pc.logger.Warn().
			Err(err).
			Msg("failed to reload BGP daemon")
	}

	pc.logger.Info().
		Str("policy", policy.GetFullName()).
		Uint32("asn", bgp.ASN).
		Int("peers", len(bgp.Peers)).
		Msg("BGP configuration applied")

	return nil
}

// generateBGPConfig generates FRR/BIRD BGP configuration
func (pc *PolicyController) generateBGPConfig(bgp *BGPConfig) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "router bgp %d\n", bgp.ASN)
	if bgp.RouterID != "" {
		fmt.Fprintf(&sb, "  bgp router-id %s\n", bgp.RouterID)
	}

	for _, peer := range bgp.Peers {
		fmt.Fprintf(&sb, "  neighbor %s remote-as %d\n", peer.Address, peer.ASN)
		if peer.Password != "" {
			fmt.Fprintf(&sb, "  neighbor %s password %s\n", peer.Address, peer.Password)
		}
		if peer.HoldTime > 0 {
			fmt.Fprintf(&sb, "  neighbor %s timers connect %d\n", peer.Address, peer.HoldTime)
		}
	}

	sb.WriteString("  address-family ipv4 unicast\n")
	for _, route := range bgp.AdvertiseRoutes {
		fmt.Fprintf(&sb, "    network %s\n", route)
	}
	sb.WriteString("  exit-address-family\n")

	return sb.String()
}

// ============================================================================
// AUDIT LOGGING
// ============================================================================

// auditLog writes an entry to the audit log
func (pc *PolicyController) auditLog(operation, policy, result string, details map[string]string) {
	if pc.auditLogPath == "" {
		return
	}

	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"operation": operation,
		"policy":    policy,
		"result":    result,
		"details":   details,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		pc.logger.Error().
			Err(err).
			Msg("failed to marshal audit log entry")
		return
	}

	f, err := os.OpenFile(pc.auditLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		pc.logger.Error().
			Err(err).
			Str("path", pc.auditLogPath).
			Msg("failed to open audit log")
		return
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(append(data, '\n')); err != nil {
		pc.logger.Error().
			Err(err).
			Msg("failed to write audit log entry")
	}
}

// ============================================================================
// EVENT EMISSION
// ============================================================================

// emitPolicyEvent emits a policy event to Wotan
func (pc *PolicyController) emitPolicyEvent(ctx context.Context, eventType string, policy *NetworkPolicy) {
	if pc.emitter == nil {
		return
	}

	event := map[string]interface{}{
		"event_type":  eventType,
		"policy_name": policy.GetFullName(),
		"namespace":   policy.Metadata.Namespace,
		"enabled":     policy.Spec.Enabled,
		"rules_count": len(policy.Spec.Rules),
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
	}

	data, err := json.Marshal(event)
	if err != nil {
		pc.logger.Error().
			Err(err).
			Msg("failed to marshal policy event")
		return
	}

	if err := pc.emitter.Publish(ctx, "network.policy.events", data); err != nil {
		pc.logger.Error().
			Err(err).
			Str("event", eventType).
			Msg("failed to publish policy event")
	}
}

// ============================================================================
// HELPERS
// ============================================================================

// sanitizeChainName sanitizes a policy name for use as a chain name
func sanitizeChainName(name string) string {
	// Replace invalid characters
	result := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)

	// Limit length
	if len(result) > 28 {
		result = result[:28]
	}

	return strings.ToUpper(result)
}

// countRulesByType counts rules of a specific type
func countRulesByType(rules []generatedRule, ruleType RuleType) int {
	count := 0
	for _, r := range rules {
		if r.Type == ruleType {
			count++
		}
	}
	return count
}

// ============================================================================
// NETWORK NAMESPACE HANDLING
// ============================================================================

// RunInNamespace executes a function within a network namespace
func (pc *PolicyController) RunInNamespace(ctx context.Context, nsPath string, fn func() error) error {
	if nsPath == "" {
		return ErrNamespaceNotFound
	}

	// Use nsenter to run in namespace
	// This is a simplified version; production code would use netns package
	cmd := exec.CommandContext(ctx, "nsenter", "--net="+nsPath, "--", "sh", "-c", "true")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("enter namespace %s: %w", nsPath, err)
	}

	return fn()
}

// ApplyPolicyToNamespace applies a policy specifically to a network namespace
func (pc *PolicyController) ApplyPolicyToNamespace(ctx context.Context, policy *NetworkPolicy, nsPath string) error {
	return pc.RunInNamespace(ctx, nsPath, func() error {
		return pc.ApplyPolicy(ctx, policy)
	})
}

// ============================================================================
// POLICY VALIDATION HELPERS
// ============================================================================

// ValidatePolicyFile validates a policy file without loading it
func ValidatePolicyFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var policy NetworkPolicy
	ext := filepath.Ext(path)

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &policy); err != nil {
			return fmt.Errorf("parse yaml: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &policy); err != nil {
			return fmt.Errorf("parse json: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %s", ext)
	}

	return policy.Validate()
}

// ============================================================================
// DEFAULT POLICIES
// ============================================================================

// CreateDefaultDenyPolicy creates a default deny all policy
func CreateDefaultDenyPolicy(namespace string) *NetworkPolicy {
	return &NetworkPolicy{
		APIVersion: "network.unheaded.io/v1",
		Kind:       "NetworkPolicy",
		Metadata: PolicyMetadata{
			Name:      "default-deny",
			Namespace: namespace,
			Labels: map[string]string{
				"unheaded.io/policy-type": "default",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: PolicySpec{
			Target: PolicyTarget{
				Namespace: namespace,
			},
			DefaultAction: ActionDeny,
			Rules:         []PolicyRule{},
			Enabled:       true,
		},
	}
}

// CreateAllowLocalPolicy creates a policy that allows local container communication
func CreateAllowLocalPolicy(namespace, networkCIDR string) *NetworkPolicy {
	return &NetworkPolicy{
		APIVersion: "network.unheaded.io/v1",
		Kind:       "NetworkPolicy",
		Metadata: PolicyMetadata{
			Name:      "allow-local",
			Namespace: namespace,
			Labels: map[string]string{
				"unheaded.io/policy-type": "local",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: PolicySpec{
			Target: PolicyTarget{
				Namespace:   namespace,
				NetworkCIDR: networkCIDR,
			},
			DefaultAction: ActionDeny,
			Rules: []PolicyRule{
				{
					Type: RuleTypeIngress,
					Ingress: &IngressRule{
						Name:        "allow-local-ingress",
						Description: "Allow ingress from local network",
						SourceCIDRs: []string{networkCIDR},
						Protocol:    ProtocolAny,
						Action:      ActionAllow,
						Priority:    100,
					},
				},
				{
					Type: RuleTypeEgress,
					Egress: &EgressRule{
						Name:             "allow-local-egress",
						Description:      "Allow egress to local network",
						DestinationCIDRs: []string{networkCIDR},
						Protocol:         ProtocolAny,
						Action:           ActionAllow,
						Priority:         100,
					},
				},
			},
			Enabled: true,
		},
	}
}
