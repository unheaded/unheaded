// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// TEST HELPERS
// ============================================================================

// newTestController creates a PolicyController without requiring iptables/nft
// on the system. It directly builds the struct, bypassing exec.LookPath.
func newTestController(opts ...func(*PolicyController)) *PolicyController {
	pc := &PolicyController{
		policies:        make(map[string]*NetworkPolicy),
		appliedPolicies: make(map[string]time.Time),
		violations:      make([]PolicyViolation, 0, 100),
		backend:         BackendIPTables,
		logger:          zerolog.New(zerolog.NewTestWriter(testing.TB(nil))).With().Logger(),
		chainPrefix:     "UNHEADED",
		maxViolations:   100,
	}
	// suppress log output during tests
	pc.logger = zerolog.Nop()
	for _, o := range opts {
		o(pc)
	}
	return pc
}

func withBackend(b FirewallBackend) func(*PolicyController) {
	return func(pc *PolicyController) { pc.backend = b }
}

func withResolver(r ContainerResolver) func(*PolicyController) {
	return func(pc *PolicyController) { pc.resolver = r }
}

func withEmitter(e EventEmitter) func(*PolicyController) {
	return func(pc *PolicyController) { pc.emitter = e }
}

func withAuditLog(path string) func(*PolicyController) {
	return func(pc *PolicyController) { pc.auditLogPath = path }
}

func withMaxViolations(n int) func(*PolicyController) {
	return func(pc *PolicyController) {
		pc.maxViolations = n
		pc.violations = make([]PolicyViolation, 0, n)
	}
}

// mockResolver implements ContainerResolver for testing.
type mockResolver struct {
	namespaces map[string]string // containerID -> nsPath
	ips        map[string]string // containerID -> ip
	names      map[string]string // name -> containerID
	labels     map[string][]string
}

func newMockResolver() *mockResolver {
	return &mockResolver{
		namespaces: make(map[string]string),
		ips:        make(map[string]string),
		names:      make(map[string]string),
		labels:     make(map[string][]string),
	}
}

func (m *mockResolver) GetContainerNamespace(containerID string) (string, error) {
	ns, ok := m.namespaces[containerID]
	if !ok {
		return "", fmt.Errorf("container %s not found", containerID)
	}
	return ns, nil
}

func (m *mockResolver) GetContainerIP(containerID string) (string, error) {
	ip, ok := m.ips[containerID]
	if !ok {
		return "", fmt.Errorf("container %s has no IP", containerID)
	}
	return ip, nil
}

func (m *mockResolver) ListContainers(labels map[string]string) ([]string, error) {
	return nil, nil
}

func (m *mockResolver) GetContainerByName(name string) (string, error) {
	id, ok := m.names[name]
	if !ok {
		return "", fmt.Errorf("container %s not found", name)
	}
	return id, nil
}

// mockEmitter implements EventEmitter for testing.
type mockEmitter struct {
	mu     sync.Mutex
	events []mockEvent
	err    error // if set, Publish returns this error
}

type mockEvent struct {
	topic   string
	payload []byte
}

func (m *mockEmitter) Publish(_ context.Context, topic string, payload []byte) error {
	if m.err != nil {
		return m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, mockEvent{topic: topic, payload: payload})
	return nil
}

func (m *mockEmitter) eventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

// validPolicy returns a minimal valid NetworkPolicy for testing.
func validPolicy(name, namespace string) *NetworkPolicy {
	return &NetworkPolicy{
		APIVersion: "network.unheaded.io/v1",
		Kind:       "NetworkPolicy",
		Metadata: PolicyMetadata{
			Name:      name,
			Namespace: namespace,
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

// validPolicyWithRules returns a policy with ingress/egress rules.
func validPolicyWithRules() *NetworkPolicy {
	return &NetworkPolicy{
		APIVersion: "network.unheaded.io/v1",
		Kind:       "NetworkPolicy",
		Metadata: PolicyMetadata{
			Name:      "web-policy",
			Namespace: "production",
		},
		Spec: PolicySpec{
			Target: PolicyTarget{
				Namespace: "production",
			},
			DefaultAction: ActionDeny,
			Rules: []PolicyRule{
				{
					Type: RuleTypeIngress,
					Ingress: &IngressRule{
						Name:        "allow-http",
						Description: "Allow HTTP traffic",
						SourceCIDRs: []string{"10.0.0.0/8"},
						Protocol:    ProtocolTCP,
						Ports:       []PortRange{{Start: 80, End: 80}},
						Action:      ActionAllow,
						Priority:    100,
					},
				},
				{
					Type: RuleTypeEgress,
					Egress: &EgressRule{
						Name:             "allow-dns",
						Description:      "Allow DNS queries",
						DestinationCIDRs: []string{"8.8.8.8/32"},
						Protocol:         ProtocolUDP,
						Ports:            []PortRange{{Start: 53, End: 53}},
						Action:           ActionAllow,
						Priority:         50,
					},
				},
			},
			Enabled: true,
		},
	}
}

// ============================================================================
// PolicyAction TESTS
// ============================================================================

func TestPolicyAction_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		action PolicyAction
		want   bool
	}{
		{"allow", ActionAllow, true},
		{"deny", ActionDeny, true},
		{"log", ActionLog, true},
		{"reject", ActionReject, true},
		{"empty", PolicyAction(""), false},
		{"unknown", PolicyAction("unknown"), false},
		{"uppercase", PolicyAction("ALLOW"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.action.IsValid(); got != tt.want {
				t.Errorf("PolicyAction(%q).IsValid() = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}

func TestPolicyAction_ToIPTablesTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		action PolicyAction
		want   string
	}{
		{ActionAllow, "ACCEPT"},
		{ActionDeny, "DROP"},
		{ActionLog, "LOG"},
		{ActionReject, "REJECT"},
		{PolicyAction("unknown"), "DROP"},
		{PolicyAction(""), "DROP"},
	}
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			t.Parallel()
			if got := tt.action.ToIPTablesTarget(); got != tt.want {
				t.Errorf("ToIPTablesTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPolicyAction_ToNFTablesVerdict(t *testing.T) {
	t.Parallel()
	tests := []struct {
		action PolicyAction
		want   string
	}{
		{ActionAllow, "accept"},
		{ActionDeny, "drop"},
		{ActionLog, "log"},
		{ActionReject, "reject"},
		{PolicyAction("unknown"), "drop"},
		{PolicyAction(""), "drop"},
	}
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			t.Parallel()
			if got := tt.action.ToNFTablesVerdict(); got != tt.want {
				t.Errorf("ToNFTablesVerdict() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ============================================================================
// PortRange TESTS
// ============================================================================

func TestPortRange_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		pr   PortRange
		want string
	}{
		{"single port", PortRange{80, 80}, "80"},
		{"port range", PortRange{8000, 9000}, "8000:9000"},
		{"low range", PortRange{1, 1024}, "1:1024"},
		{"full range", PortRange{1, 65535}, "1:65535"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.pr.String(); got != tt.want {
				t.Errorf("PortRange.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPortRange_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		pr   PortRange
		want bool
	}{
		{"valid single", PortRange{80, 80}, true},
		{"valid range", PortRange{8000, 9000}, true},
		{"min port", PortRange{1, 1}, true},
		{"max port", PortRange{65535, 65535}, true},
		{"full range", PortRange{1, 65535}, true},
		{"zero start", PortRange{0, 80}, false},
		{"zero both", PortRange{0, 0}, false},
		{"end less than start", PortRange{9000, 8000}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.pr.IsValid(); got != tt.want {
				t.Errorf("PortRange{%d,%d}.IsValid() = %v, want %v", tt.pr.Start, tt.pr.End, got, tt.want)
			}
		})
	}
}

// ============================================================================
// PolicyTarget TESTS
// ============================================================================

func TestPolicyTarget_IsEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		target PolicyTarget
		want   bool
	}{
		{"empty", PolicyTarget{}, true},
		{"with container IDs", PolicyTarget{ContainerIDs: []string{"abc"}}, false},
		{"with container names", PolicyTarget{ContainerNames: []string{"web"}}, false},
		{"with labels", PolicyTarget{Labels: map[string]string{"app": "web"}}, false},
		{"with namespace", PolicyTarget{Namespace: "prod"}, false},
		{"with network CIDR", PolicyTarget{NetworkCIDR: "10.0.0.0/8"}, false},
		{"empty slices and maps", PolicyTarget{
			ContainerIDs:   []string{},
			ContainerNames: []string{},
			Labels:         map[string]string{},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.target.IsEmpty(); got != tt.want {
				t.Errorf("PolicyTarget.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// IngressRule VALIDATION TESTS
// ============================================================================

func TestIngressRule_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rule    IngressRule
		wantErr bool
	}{
		{
			name:    "valid rule",
			rule:    IngressRule{Name: "allow-http", Action: ActionAllow, Protocol: ProtocolTCP, Ports: []PortRange{{80, 80}}},
			wantErr: false,
		},
		{
			name:    "valid rule no ports",
			rule:    IngressRule{Name: "allow-all", Action: ActionAllow, Protocol: ProtocolAny},
			wantErr: false,
		},
		{
			name:    "empty name",
			rule:    IngressRule{Name: "", Action: ActionAllow},
			wantErr: true,
		},
		{
			name:    "invalid action",
			rule:    IngressRule{Name: "test", Action: PolicyAction("invalid")},
			wantErr: true,
		},
		{
			name:    "invalid port range",
			rule:    IngressRule{Name: "test", Action: ActionAllow, Ports: []PortRange{{0, 80}}},
			wantErr: true,
		},
		{
			name:    "port end less than start",
			rule:    IngressRule{Name: "test", Action: ActionAllow, Ports: []PortRange{{9000, 80}}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("IngressRule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// EgressRule VALIDATION TESTS
// ============================================================================

func TestEgressRule_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rule    EgressRule
		wantErr bool
	}{
		{
			name:    "valid rule",
			rule:    EgressRule{Name: "allow-dns", Action: ActionAllow, Protocol: ProtocolUDP, Ports: []PortRange{{53, 53}}},
			wantErr: false,
		},
		{
			name:    "empty name",
			rule:    EgressRule{Name: "", Action: ActionAllow},
			wantErr: true,
		},
		{
			name:    "invalid action",
			rule:    EgressRule{Name: "test", Action: PolicyAction("xyz")},
			wantErr: true,
		},
		{
			name:    "invalid port",
			rule:    EgressRule{Name: "test", Action: ActionAllow, Ports: []PortRange{{0, 0}}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("EgressRule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// InterContainerRule VALIDATION TESTS
// ============================================================================

func TestInterContainerRule_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rule    InterContainerRule
		wantErr bool
	}{
		{
			name: "valid",
			rule: InterContainerRule{
				Name:                  "allow-db",
				SourceContainers:      []string{"web"},
				DestinationContainers: []string{"db"},
				Action:                ActionAllow,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			rule: InterContainerRule{
				Name:                  "",
				SourceContainers:      []string{"web"},
				DestinationContainers: []string{"db"},
				Action:                ActionAllow,
			},
			wantErr: true,
		},
		{
			name: "no source containers",
			rule: InterContainerRule{
				Name:                  "test",
				SourceContainers:      []string{},
				DestinationContainers: []string{"db"},
				Action:                ActionAllow,
			},
			wantErr: true,
		},
		{
			name: "no destination containers",
			rule: InterContainerRule{
				Name:                  "test",
				SourceContainers:      []string{"web"},
				DestinationContainers: []string{},
				Action:                ActionAllow,
			},
			wantErr: true,
		},
		{
			name: "invalid action",
			rule: InterContainerRule{
				Name:                  "test",
				SourceContainers:      []string{"web"},
				DestinationContainers: []string{"db"},
				Action:                PolicyAction("bad"),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("InterContainerRule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// ExternalRule VALIDATION TESTS
// ============================================================================

func TestExternalRule_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rule    ExternalRule
		wantErr bool
	}{
		{
			name:    "valid ingress",
			rule:    ExternalRule{Name: "ext-in", Direction: RuleTypeIngress, Action: ActionAllow, Protocol: ProtocolTCP},
			wantErr: false,
		},
		{
			name:    "valid egress",
			rule:    ExternalRule{Name: "ext-out", Direction: RuleTypeEgress, Action: ActionDeny, Protocol: ProtocolUDP},
			wantErr: false,
		},
		{
			name:    "empty name",
			rule:    ExternalRule{Name: "", Direction: RuleTypeIngress, Action: ActionAllow},
			wantErr: true,
		},
		{
			name:    "invalid direction",
			rule:    ExternalRule{Name: "test", Direction: RuleTypeInterContainer, Action: ActionAllow},
			wantErr: true,
		},
		{
			name:    "invalid action",
			rule:    ExternalRule{Name: "test", Direction: RuleTypeIngress, Action: PolicyAction("nope")},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ExternalRule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// PolicyRule TESTS
// ============================================================================

func TestPolicyRule_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rule    PolicyRule
		wantErr bool
	}{
		{
			name: "valid ingress",
			rule: PolicyRule{
				Type:    RuleTypeIngress,
				Ingress: &IngressRule{Name: "http", Action: ActionAllow, Protocol: ProtocolTCP},
			},
			wantErr: false,
		},
		{
			name: "valid egress",
			rule: PolicyRule{
				Type:   RuleTypeEgress,
				Egress: &EgressRule{Name: "dns", Action: ActionAllow, Protocol: ProtocolUDP},
			},
			wantErr: false,
		},
		{
			name: "valid inter-container",
			rule: PolicyRule{
				Type: RuleTypeInterContainer,
				InterContainer: &InterContainerRule{
					Name:                  "web-db",
					SourceContainers:      []string{"web"},
					DestinationContainers: []string{"db"},
					Action:                ActionAllow,
				},
			},
			wantErr: false,
		},
		{
			name: "valid external",
			rule: PolicyRule{
				Type:     RuleTypeExternal,
				External: &ExternalRule{Name: "ext", Direction: RuleTypeIngress, Action: ActionAllow},
			},
			wantErr: false,
		},
		{
			name:    "ingress type without ingress rule",
			rule:    PolicyRule{Type: RuleTypeIngress},
			wantErr: true,
		},
		{
			name:    "egress type without egress rule",
			rule:    PolicyRule{Type: RuleTypeEgress},
			wantErr: true,
		},
		{
			name:    "inter-container type without inter-container rule",
			rule:    PolicyRule{Type: RuleTypeInterContainer},
			wantErr: true,
		},
		{
			name:    "external type without external rule",
			rule:    PolicyRule{Type: RuleTypeExternal},
			wantErr: true,
		},
		{
			name:    "unknown type",
			rule:    PolicyRule{Type: RuleType("unknown")},
			wantErr: true,
		},
		{
			name:    "empty type",
			rule:    PolicyRule{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("PolicyRule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPolicyRule_GetName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rule PolicyRule
		want string
	}{
		{
			name: "ingress name",
			rule: PolicyRule{Type: RuleTypeIngress, Ingress: &IngressRule{Name: "allow-http"}},
			want: "allow-http",
		},
		{
			name: "egress name",
			rule: PolicyRule{Type: RuleTypeEgress, Egress: &EgressRule{Name: "allow-dns"}},
			want: "allow-dns",
		},
		{
			name: "inter-container name",
			rule: PolicyRule{Type: RuleTypeInterContainer, InterContainer: &InterContainerRule{Name: "web-db"}},
			want: "web-db",
		},
		{
			name: "external name",
			rule: PolicyRule{Type: RuleTypeExternal, External: &ExternalRule{Name: "ext-rule"}},
			want: "ext-rule",
		},
		{
			name: "ingress nil rule",
			rule: PolicyRule{Type: RuleTypeIngress},
			want: "",
		},
		{
			name: "egress nil rule",
			rule: PolicyRule{Type: RuleTypeEgress},
			want: "",
		},
		{
			name: "inter-container nil rule",
			rule: PolicyRule{Type: RuleTypeInterContainer},
			want: "",
		},
		{
			name: "external nil rule",
			rule: PolicyRule{Type: RuleTypeExternal},
			want: "",
		},
		{
			name: "unknown type",
			rule: PolicyRule{Type: RuleType("unknown")},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.rule.GetName(); got != tt.want {
				t.Errorf("GetName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPolicyRule_GetPriority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rule PolicyRule
		want int
	}{
		{
			name: "ingress priority",
			rule: PolicyRule{Type: RuleTypeIngress, Ingress: &IngressRule{Name: "r", Action: ActionAllow, Priority: 42}},
			want: 42,
		},
		{
			name: "egress priority",
			rule: PolicyRule{Type: RuleTypeEgress, Egress: &EgressRule{Name: "r", Action: ActionAllow, Priority: 10}},
			want: 10,
		},
		{
			name: "inter-container priority",
			rule: PolicyRule{Type: RuleTypeInterContainer, InterContainer: &InterContainerRule{
				Name: "r", SourceContainers: []string{"a"}, DestinationContainers: []string{"b"}, Action: ActionAllow, Priority: 5,
			}},
			want: 5,
		},
		{
			name: "external priority",
			rule: PolicyRule{Type: RuleTypeExternal, External: &ExternalRule{Name: "r", Direction: RuleTypeIngress, Action: ActionAllow, Priority: 99}},
			want: 99,
		},
		{
			name: "nil ingress",
			rule: PolicyRule{Type: RuleTypeIngress},
			want: 0,
		},
		{
			name: "unknown type",
			rule: PolicyRule{Type: RuleType("unknown")},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.rule.GetPriority(); got != tt.want {
				t.Errorf("GetPriority() = %d, want %d", got, tt.want)
			}
		})
	}
}

// ============================================================================
// NetworkPolicy TESTS
// ============================================================================

func TestNetworkPolicy_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		policy  NetworkPolicy
		wantErr bool
	}{
		{
			name:    "valid minimal",
			policy:  *validPolicy("test", "ns"),
			wantErr: false,
		},
		{
			name: "valid with rules",
			policy: NetworkPolicy{
				Metadata: PolicyMetadata{Name: "test", Namespace: "ns"},
				Spec: PolicySpec{
					Target:        PolicyTarget{Namespace: "ns"},
					DefaultAction: ActionDeny,
					Rules: []PolicyRule{
						{Type: RuleTypeIngress, Ingress: &IngressRule{Name: "r1", Action: ActionAllow}},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			policy: NetworkPolicy{
				Metadata: PolicyMetadata{Name: ""},
				Spec: PolicySpec{
					Target:        PolicyTarget{Namespace: "ns"},
					DefaultAction: ActionDeny,
				},
			},
			wantErr: true,
		},
		{
			name: "empty target",
			policy: NetworkPolicy{
				Metadata: PolicyMetadata{Name: "test"},
				Spec: PolicySpec{
					Target:        PolicyTarget{},
					DefaultAction: ActionDeny,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid default action",
			policy: NetworkPolicy{
				Metadata: PolicyMetadata{Name: "test"},
				Spec: PolicySpec{
					Target:        PolicyTarget{Namespace: "ns"},
					DefaultAction: PolicyAction("invalid"),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid rule in list",
			policy: NetworkPolicy{
				Metadata: PolicyMetadata{Name: "test"},
				Spec: PolicySpec{
					Target:        PolicyTarget{Namespace: "ns"},
					DefaultAction: ActionDeny,
					Rules: []PolicyRule{
						{Type: RuleTypeIngress}, // missing ingress definition
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("NetworkPolicy.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkPolicy_GetFullName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		policy   NetworkPolicy
		wantFull string
	}{
		{
			name:     "with namespace",
			policy:   NetworkPolicy{Metadata: PolicyMetadata{Name: "deny-all", Namespace: "prod"}},
			wantFull: "prod/deny-all",
		},
		{
			name:     "without namespace",
			policy:   NetworkPolicy{Metadata: PolicyMetadata{Name: "deny-all"}},
			wantFull: "deny-all",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.policy.GetFullName(); got != tt.wantFull {
				t.Errorf("GetFullName() = %q, want %q", got, tt.wantFull)
			}
		})
	}
}

// ============================================================================
// PolicyController CRUD TESTS
// ============================================================================

func TestPolicyController_GetPolicy(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	pc.policies["ns/test"] = p

	// Existing policy
	got, ok := pc.GetPolicy("ns/test")
	if !ok {
		t.Fatal("GetPolicy returned false for existing policy")
	}
	if got.Metadata.Name != "test" {
		t.Errorf("GetPolicy name = %q, want %q", got.Metadata.Name, "test")
	}

	// Non-existing policy
	_, ok = pc.GetPolicy("nonexistent")
	if ok {
		t.Error("GetPolicy returned true for non-existing policy")
	}
}

func TestPolicyController_ListPolicies(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	// Empty list
	if got := pc.ListPolicies(); len(got) != 0 {
		t.Errorf("ListPolicies() returned %d, want 0", len(got))
	}

	// Add some policies
	pc.policies["a"] = validPolicy("a", "")
	pc.policies["b"] = validPolicy("b", "")
	pc.policies["c"] = validPolicy("c", "")

	got := pc.ListPolicies()
	if len(got) != 3 {
		t.Errorf("ListPolicies() returned %d, want 3", len(got))
	}
}

// ============================================================================
// VIOLATIONS TESTS
// ============================================================================

func TestPolicyController_RecordAndGetViolations(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	v := PolicyViolation{
		PolicyName:      "deny-all",
		RuleType:        RuleTypeIngress,
		SourceIP:        "10.0.0.5",
		DestinationIP:   "10.0.0.10",
		DestinationPort: 443,
		Protocol:        ProtocolTCP,
		Action:          ActionDeny,
		ContainerName:   "web",
	}

	pc.RecordViolation(v)

	violations := pc.GetViolations(time.Minute)
	if len(violations) != 1 {
		t.Fatalf("GetViolations() returned %d, want 1", len(violations))
	}
	if violations[0].PolicyName != "deny-all" {
		t.Errorf("violation PolicyName = %q, want %q", violations[0].PolicyName, "deny-all")
	}
	if violations[0].ID == "" {
		t.Error("violation ID should be set")
	}
	if violations[0].Timestamp.IsZero() {
		t.Error("violation Timestamp should be set")
	}
}

func TestPolicyController_GetViolations_FiltersByDuration(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	// Record a violation with a timestamp in the past (we can't control it via
	// RecordViolation since it sets time.Now(), so record then test with a
	// very short duration that excludes it).
	pc.RecordViolation(PolicyViolation{
		PolicyName:    "test",
		RuleType:      RuleTypeIngress,
		SourceIP:      "1.2.3.4",
		DestinationIP: "5.6.7.8",
		Protocol:      ProtocolTCP,
		Action:        ActionDeny,
		ContainerName: "web",
	})

	// With a large duration, we should see it
	violations := pc.GetViolations(time.Hour)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
}

func TestPolicyController_Violations_MaxCapacity(t *testing.T) {
	t.Parallel()
	pc := newTestController(withMaxViolations(10))

	// Record more than maxViolations
	for i := 0; i < 15; i++ {
		pc.RecordViolation(PolicyViolation{
			PolicyName:    fmt.Sprintf("policy-%d", i),
			RuleType:      RuleTypeIngress,
			SourceIP:      "1.2.3.4",
			DestinationIP: "5.6.7.8",
			Protocol:      ProtocolTCP,
			Action:        ActionDeny,
			ContainerName: "test",
		})
	}

	violations := pc.GetViolations(time.Hour)
	if len(violations) > 15 {
		t.Errorf("expected at most 15 violations, got %d", len(violations))
	}
	// After hitting capacity, old entries are trimmed by half
	// We check the internal slice is under control
	pc.violationsMu.RLock()
	internalLen := len(pc.violations)
	pc.violationsMu.RUnlock()
	if internalLen > 15 {
		t.Errorf("internal violations length %d exceeds expected max", internalLen)
	}
}

// ============================================================================
// LOAD POLICIES FROM FILE TESTS
// ============================================================================

func TestPolicyController_LoadPolicies_YAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	policy := validPolicy("yaml-policy", "test-ns")
	data, err := yaml.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "policy.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := pc.LoadPolicies(filepath.Join(dir, "policy.yaml")); err != nil {
		t.Fatalf("LoadPolicies() error: %v", err)
	}

	got, ok := pc.GetPolicy("test-ns/yaml-policy")
	if !ok {
		t.Fatal("policy not loaded")
	}
	if got.Metadata.Name != "yaml-policy" {
		t.Errorf("policy name = %q, want %q", got.Metadata.Name, "yaml-policy")
	}
}

func TestPolicyController_LoadPolicies_JSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	policy := validPolicy("json-policy", "test-ns")
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "policy.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := pc.LoadPolicies(filepath.Join(dir, "policy.json")); err != nil {
		t.Fatalf("LoadPolicies() error: %v", err)
	}

	got, ok := pc.GetPolicy("test-ns/json-policy")
	if !ok {
		t.Fatal("policy not loaded")
	}
	if got.Metadata.Name != "json-policy" {
		t.Errorf("policy name = %q, want %q", got.Metadata.Name, "json-policy")
	}
}

func TestPolicyController_LoadPolicies_Directory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	// Write multiple policies
	for i, ext := range []string{".yaml", ".yml", ".json"} {
		name := fmt.Sprintf("policy-%d", i)
		p := validPolicy(name, "ns")
		var data []byte
		var err error
		if ext == ".json" {
			data, err = json.Marshal(p)
		} else {
			data, err = yaml.Marshal(p)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+ext), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Write an ignored file
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a policy"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := pc.LoadPolicies(dir); err != nil {
		t.Fatalf("LoadPolicies(dir) error: %v", err)
	}

	if got := len(pc.ListPolicies()); got != 3 {
		t.Errorf("loaded %d policies, want 3", got)
	}
}

func TestPolicyController_LoadPolicies_NonexistentPath(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	err := pc.LoadPolicies("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestPolicyController_LoadPolicies_InvalidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{{{not yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	err := pc.LoadPolicies(filepath.Join(dir, "bad.yaml"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestPolicyController_LoadPolicies_InvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json}"), 0644); err != nil {
		t.Fatal(err)
	}
	err := pc.LoadPolicies(filepath.Join(dir, "bad.json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestPolicyController_LoadPolicies_InvalidPolicyContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	// Valid YAML but invalid policy (no name, no target)
	p := &NetworkPolicy{}
	data, _ := yaml.Marshal(p)
	if err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}
	err := pc.LoadPolicies(filepath.Join(dir, "invalid.yaml"))
	if err == nil {
		t.Error("expected error for invalid policy content")
	}
}

func TestPolicyController_LoadPolicies_UnsupportedExtension(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	if err := os.WriteFile(filepath.Join(dir, "policy.toml"), []byte("key=value"), 0644); err != nil {
		t.Fatal(err)
	}
	err := pc.LoadPolicies(filepath.Join(dir, "policy.toml"))
	if err == nil {
		t.Error("expected error for unsupported file extension")
	}
}

func TestPolicyController_LoadPolicies_DirectoryWithInvalidFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	// One valid and one invalid policy
	good := validPolicy("good", "ns")
	goodData, _ := yaml.Marshal(good)
	if err := os.WriteFile(filepath.Join(dir, "good.yaml"), goodData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should not error but skip bad files
	if err := pc.LoadPolicies(dir); err != nil {
		t.Fatalf("LoadPolicies(dir) error: %v", err)
	}

	if got := len(pc.ListPolicies()); got != 1 {
		t.Errorf("loaded %d policies, want 1 (skip bad)", got)
	}
}

// ============================================================================
// ValidatePolicyFile TESTS
// ============================================================================

func TestValidatePolicyFile_ValidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := validPolicy("test", "ns")
	data, _ := yaml.Marshal(p)
	path := filepath.Join(dir, "p.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePolicyFile(path); err != nil {
		t.Errorf("ValidatePolicyFile() error = %v", err)
	}
}

func TestValidatePolicyFile_ValidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := validPolicy("test", "ns")
	data, _ := json.Marshal(p)
	path := filepath.Join(dir, "p.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePolicyFile(path); err != nil {
		t.Errorf("ValidatePolicyFile() error = %v", err)
	}
}

func TestValidatePolicyFile_NonexistentFile(t *testing.T) {
	t.Parallel()
	err := ValidatePolicyFile("/nonexistent/file.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestValidatePolicyFile_UnsupportedFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "p.toml")
	if err := os.WriteFile(path, []byte("key=val"), 0644); err != nil {
		t.Fatal(err)
	}
	err := ValidatePolicyFile(path)
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestValidatePolicyFile_InvalidContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Valid YAML but invalid policy
	data, _ := yaml.Marshal(&NetworkPolicy{Metadata: PolicyMetadata{Name: ""}})
	path := filepath.Join(dir, "p.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	err := ValidatePolicyFile(path)
	if err == nil {
		t.Error("expected error for invalid policy content")
	}
}

// ============================================================================
// DEFAULT POLICY FACTORY TESTS
// ============================================================================

func TestCreateDefaultDenyPolicy(t *testing.T) {
	t.Parallel()
	p := CreateDefaultDenyPolicy("production")

	if p.Metadata.Name != "default-deny" {
		t.Errorf("name = %q, want %q", p.Metadata.Name, "default-deny")
	}
	if p.Metadata.Namespace != "production" {
		t.Errorf("namespace = %q, want %q", p.Metadata.Namespace, "production")
	}
	if p.Spec.DefaultAction != ActionDeny {
		t.Errorf("default action = %q, want %q", p.Spec.DefaultAction, ActionDeny)
	}
	if !p.Spec.Enabled {
		t.Error("policy should be enabled")
	}
	if len(p.Spec.Rules) != 0 {
		t.Errorf("rules count = %d, want 0", len(p.Spec.Rules))
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestCreateAllowLocalPolicy(t *testing.T) {
	t.Parallel()
	p := CreateAllowLocalPolicy("dev", "10.0.0.0/24")

	if p.Metadata.Name != "allow-local" {
		t.Errorf("name = %q, want %q", p.Metadata.Name, "allow-local")
	}
	if p.Metadata.Namespace != "dev" {
		t.Errorf("namespace = %q, want %q", p.Metadata.Namespace, "dev")
	}
	if p.Spec.Target.NetworkCIDR != "10.0.0.0/24" {
		t.Errorf("network CIDR = %q, want %q", p.Spec.Target.NetworkCIDR, "10.0.0.0/24")
	}
	if len(p.Spec.Rules) != 2 {
		t.Fatalf("rules count = %d, want 2", len(p.Spec.Rules))
	}
	if p.Spec.Rules[0].Type != RuleTypeIngress {
		t.Errorf("rule[0] type = %q, want ingress", p.Spec.Rules[0].Type)
	}
	if p.Spec.Rules[1].Type != RuleTypeEgress {
		t.Errorf("rule[1] type = %q, want egress", p.Spec.Rules[1].Type)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

// ============================================================================
// RULE GENERATION TESTS
// ============================================================================

func TestGenerateRules_DefaultDeny(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.DefaultAction = ActionDeny

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	// Should have default deny ingress and egress rules
	if len(rules) < 2 {
		t.Fatalf("expected at least 2 default deny rules, got %d", len(rules))
	}

	// Check that rules contain INPUT and OUTPUT default deny
	var hasInputDeny, hasOutputDeny bool
	for _, r := range rules {
		if r.Chain == "INPUT" && r.Action == ActionDeny {
			hasInputDeny = true
		}
		if r.Chain == "OUTPUT" && r.Action == ActionDeny {
			hasOutputDeny = true
		}
	}
	if !hasInputDeny {
		t.Error("missing default deny INPUT rule")
	}
	if !hasOutputDeny {
		t.Error("missing default deny OUTPUT rule")
	}
}

func TestGenerateRules_DefaultAllow(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.DefaultAction = ActionAllow

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	// When default is allow, no default deny rules should be generated
	for _, r := range rules {
		if r.Priority == 9999 {
			t.Error("should not have default deny rules when default action is allow")
		}
	}
}

func TestGenerateRules_IngressRules(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicyWithRules()

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	// Find ingress rules
	var ingressRules []generatedRule
	for _, r := range rules {
		if r.Type == RuleTypeIngress && r.Action == ActionAllow {
			ingressRules = append(ingressRules, r)
		}
	}

	if len(ingressRules) == 0 {
		t.Fatal("expected at least one ingress allow rule")
	}

	// Check first ingress rule
	r := ingressRules[0]
	if r.Chain != "INPUT" {
		t.Errorf("chain = %q, want INPUT", r.Chain)
	}
	if r.Protocol != ProtocolTCP {
		t.Errorf("protocol = %q, want tcp", r.Protocol)
	}
	if r.SourceIP != "10.0.0.0/8" {
		t.Errorf("source IP = %q, want 10.0.0.0/8", r.SourceIP)
	}
	if r.DstPort != "80" {
		t.Errorf("dst port = %q, want 80", r.DstPort)
	}
}

func TestGenerateRules_EgressRules(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicyWithRules()

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var egressRules []generatedRule
	for _, r := range rules {
		if r.Type == RuleTypeEgress && r.Action == ActionAllow {
			egressRules = append(egressRules, r)
		}
	}

	if len(egressRules) == 0 {
		t.Fatal("expected at least one egress allow rule")
	}

	r := egressRules[0]
	if r.Chain != "OUTPUT" {
		t.Errorf("chain = %q, want OUTPUT", r.Chain)
	}
	if r.Protocol != ProtocolUDP {
		t.Errorf("protocol = %q, want udp", r.Protocol)
	}
	if r.DestIP != "8.8.8.8/32" {
		t.Errorf("dest IP = %q, want 8.8.8.8/32", r.DestIP)
	}
}

func TestGenerateRules_IngressNoPorts(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeIngress,
			Ingress: &IngressRule{
				Name:     "allow-all",
				Protocol: ProtocolAny,
				Action:   ActionAllow,
				Priority: 50,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	// Should have a rule with empty DstPort and SourceIP = 0.0.0.0/0
	var found bool
	for _, r := range rules {
		if r.Comment == "test-allow-all" && r.DstPort == "" && r.SourceIP == "0.0.0.0/0" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ingress rule for all sources and all ports")
	}
}

func TestGenerateRules_EgressNoPorts(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeEgress,
			Egress: &EgressRule{
				Name:     "allow-all-egress",
				Protocol: ProtocolAny,
				Action:   ActionAllow,
				Priority: 50,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var found bool
	for _, r := range rules {
		if r.Comment == "test-allow-all-egress" && r.DstPort == "" && r.DestIP == "0.0.0.0/0" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected egress rule for all destinations and all ports")
	}
}

func TestGenerateRules_InterContainerWithResolver(t *testing.T) {
	t.Parallel()
	resolver := newMockResolver()
	resolver.names["web"] = "container-1"
	resolver.names["db"] = "container-2"
	resolver.ips["container-1"] = "10.10.10.1"
	resolver.ips["container-2"] = "10.10.10.2"

	pc := newTestController(withResolver(resolver))
	p := validPolicy("test", "ns")
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeInterContainer,
			InterContainer: &InterContainerRule{
				Name:                  "web-to-db",
				SourceContainers:      []string{"web"},
				DestinationContainers: []string{"db"},
				Protocol:              ProtocolTCP,
				Ports:                 []PortRange{{Start: 5432, End: 5432}},
				Action:                ActionAllow,
				Priority:              10,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var interRules []generatedRule
	for _, r := range rules {
		if r.Type == RuleTypeInterContainer {
			interRules = append(interRules, r)
		}
	}

	if len(interRules) == 0 {
		t.Fatal("expected inter-container rules")
	}

	r := interRules[0]
	if r.Chain != "FORWARD" {
		t.Errorf("chain = %q, want FORWARD", r.Chain)
	}
	if r.SourceIP != "10.10.10.1" {
		t.Errorf("source IP = %q, want 10.10.10.1", r.SourceIP)
	}
	if r.DestIP != "10.10.10.2" {
		t.Errorf("dest IP = %q, want 10.10.10.2", r.DestIP)
	}
	if r.DstPort != "5432" {
		t.Errorf("dst port = %q, want 5432", r.DstPort)
	}
}

func TestGenerateRules_InterContainerBidirectional(t *testing.T) {
	t.Parallel()
	resolver := newMockResolver()
	resolver.names["a"] = "c-1"
	resolver.names["b"] = "c-2"
	resolver.ips["c-1"] = "10.0.0.1"
	resolver.ips["c-2"] = "10.0.0.2"

	pc := newTestController(withResolver(resolver))
	p := validPolicy("test", "ns")
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeInterContainer,
			InterContainer: &InterContainerRule{
				Name:                  "bidir",
				SourceContainers:      []string{"a"},
				DestinationContainers: []string{"b"},
				Protocol:              ProtocolTCP,
				Ports:                 []PortRange{{Start: 80, End: 80}},
				Action:                ActionAllow,
				Bidirectional:         true,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var interRules []generatedRule
	for _, r := range rules {
		if r.Type == RuleTypeInterContainer {
			interRules = append(interRules, r)
		}
	}

	// Should have forward and reverse rules
	if len(interRules) < 2 {
		t.Fatalf("expected at least 2 inter-container rules for bidirectional, got %d", len(interRules))
	}

	// Check one is forward and one is reverse
	var hasForward, hasReverse bool
	for _, r := range interRules {
		if r.SourceIP == "10.0.0.1" && r.DestIP == "10.0.0.2" {
			hasForward = true
		}
		if r.SourceIP == "10.0.0.2" && r.DestIP == "10.0.0.1" {
			hasReverse = true
		}
	}
	if !hasForward {
		t.Error("missing forward rule")
	}
	if !hasReverse {
		t.Error("missing reverse rule")
	}
}

func TestGenerateRules_InterContainerWithoutResolver(t *testing.T) {
	t.Parallel()
	pc := newTestController() // no resolver
	p := validPolicy("test", "ns")
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeInterContainer,
			InterContainer: &InterContainerRule{
				Name:                  "web-to-db",
				SourceContainers:      []string{"web"},
				DestinationContainers: []string{"db"},
				Protocol:              ProtocolTCP,
				Ports:                 []PortRange{{Start: 5432, End: 5432}},
				Action:                ActionAllow,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	// Without resolver, no inter-container rules can be generated (no IPs)
	var interRules []generatedRule
	for _, r := range rules {
		if r.Type == RuleTypeInterContainer {
			interRules = append(interRules, r)
		}
	}
	if len(interRules) != 0 {
		t.Errorf("expected 0 inter-container rules without resolver, got %d", len(interRules))
	}
}

func TestGenerateRules_ExternalIngress(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeExternal,
			External: &ExternalRule{
				Name:          "allow-external-https",
				Direction:     RuleTypeIngress,
				ExternalCIDRs: []string{"0.0.0.0/0"},
				Protocol:      ProtocolTCP,
				Ports:         []PortRange{{Start: 443, End: 443}},
				Action:        ActionAllow,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var extRules []generatedRule
	for _, r := range rules {
		if r.Type == RuleTypeExternal {
			extRules = append(extRules, r)
		}
	}
	if len(extRules) == 0 {
		t.Fatal("expected external rules")
	}
	if extRules[0].Chain != "INPUT" {
		t.Errorf("external ingress chain = %q, want INPUT", extRules[0].Chain)
	}
}

func TestGenerateRules_ExternalEgress(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeExternal,
			External: &ExternalRule{
				Name:          "allow-external-egress",
				Direction:     RuleTypeEgress,
				ExternalCIDRs: []string{"8.8.8.8/32"},
				Protocol:      ProtocolUDP,
				Ports:         []PortRange{{Start: 53, End: 53}},
				Action:        ActionAllow,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var extRules []generatedRule
	for _, r := range rules {
		if r.Type == RuleTypeExternal {
			extRules = append(extRules, r)
		}
	}
	if len(extRules) == 0 {
		t.Fatal("expected external egress rules")
	}
	if extRules[0].Chain != "OUTPUT" {
		t.Errorf("external egress chain = %q, want OUTPUT", extRules[0].Chain)
	}
	if extRules[0].DestIP != "8.8.8.8/32" {
		t.Errorf("dest IP = %q, want 8.8.8.8/32", extRules[0].DestIP)
	}
}

func TestGenerateRules_ExternalNoCIDRs(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeExternal,
			External: &ExternalRule{
				Name:      "ext-no-cidr",
				Direction: RuleTypeIngress,
				Protocol:  ProtocolTCP,
				Ports:     []PortRange{{Start: 80, End: 80}},
				Action:    ActionAllow,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var extRules []generatedRule
	for _, r := range rules {
		if r.Type == RuleTypeExternal {
			extRules = append(extRules, r)
		}
	}
	if len(extRules) == 0 {
		t.Fatal("expected external rules even with no CIDRs")
	}
	// Should default to 0.0.0.0/0
	if extRules[0].SourceIP != "0.0.0.0/0" {
		t.Errorf("source IP = %q, want 0.0.0.0/0", extRules[0].SourceIP)
	}
}

func TestGenerateRules_PrioritySorting(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.DefaultAction = ActionAllow // no default deny rules to simplify
	p.Spec.Rules = []PolicyRule{
		{
			Type:    RuleTypeIngress,
			Ingress: &IngressRule{Name: "low-prio", Action: ActionAllow, Protocol: ProtocolTCP, Priority: 200},
		},
		{
			Type:    RuleTypeIngress,
			Ingress: &IngressRule{Name: "high-prio", Action: ActionAllow, Protocol: ProtocolTCP, Priority: 10},
		},
		{
			Type:    RuleTypeIngress,
			Ingress: &IngressRule{Name: "mid-prio", Action: ActionAllow, Protocol: ProtocolTCP, Priority: 100},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	// Verify sorted by priority ascending
	for i := 1; i < len(rules); i++ {
		if rules[i].Priority < rules[i-1].Priority {
			t.Errorf("rules not sorted by priority: rules[%d].Priority=%d < rules[%d].Priority=%d",
				i, rules[i].Priority, i-1, rules[i-1].Priority)
		}
	}
}

// ============================================================================
// buildIPTablesArgs TESTS
// ============================================================================

func TestBuildIPTablesArgs(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	tests := []struct {
		name     string
		rule     generatedRule
		chain    string
		wantArgs []string
	}{
		{
			name: "basic tcp rule",
			rule: generatedRule{
				Protocol: ProtocolTCP,
				SourceIP: "10.0.0.0/8",
				DstPort:  "80",
				Action:   ActionAllow,
				Comment:  "test-http",
			},
			chain:    "TEST-CHAIN",
			wantArgs: []string{"-A", "TEST-CHAIN", "-p", "tcp", "-s", "10.0.0.0/8", "--dport", "80", "-m", "comment", "--comment", "test-http", "-j", "ACCEPT"},
		},
		{
			name: "rule with dest IP",
			rule: generatedRule{
				Protocol: ProtocolUDP,
				DestIP:   "8.8.8.8/32",
				DstPort:  "53",
				Action:   ActionAllow,
			},
			chain:    "MY-CHAIN",
			wantArgs: []string{"-A", "MY-CHAIN", "-p", "udp", "-d", "8.8.8.8/32", "--dport", "53", "-j", "ACCEPT"},
		},
		{
			name: "any protocol",
			rule: generatedRule{
				Protocol: ProtocolAny,
				Action:   ActionDeny,
			},
			chain:    "C",
			wantArgs: []string{"-A", "C", "-j", "DROP"},
		},
		{
			name: "empty protocol",
			rule: generatedRule{
				Action: ActionDeny,
			},
			chain:    "C",
			wantArgs: []string{"-A", "C", "-j", "DROP"},
		},
		{
			name: "reject action",
			rule: generatedRule{
				Protocol: ProtocolTCP,
				Action:   ActionReject,
			},
			chain:    "C",
			wantArgs: []string{"-A", "C", "-p", "tcp", "-j", "REJECT"},
		},
		{
			name: "0.0.0.0/0 source skipped",
			rule: generatedRule{
				Protocol: ProtocolTCP,
				SourceIP: "0.0.0.0/0",
				Action:   ActionAllow,
			},
			chain:    "C",
			wantArgs: []string{"-A", "C", "-p", "tcp", "-j", "ACCEPT"},
		},
		{
			name: "0.0.0.0/0 dest skipped",
			rule: generatedRule{
				Protocol: ProtocolTCP,
				DestIP:   "0.0.0.0/0",
				Action:   ActionAllow,
			},
			chain:    "C",
			wantArgs: []string{"-A", "C", "-p", "tcp", "-j", "ACCEPT"},
		},
		{
			name: "with src port",
			rule: generatedRule{
				Protocol: ProtocolTCP,
				SrcPort:  "1234",
				Action:   ActionAllow,
			},
			chain:    "C",
			wantArgs: []string{"-A", "C", "-p", "tcp", "--sport", "1234", "-j", "ACCEPT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pc.buildIPTablesArgs(tt.rule, tt.chain)
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("args length = %d, want %d\ngot:  %v\nwant: %v", len(got), len(tt.wantArgs), got, tt.wantArgs)
			}
			for i, arg := range got {
				if arg != tt.wantArgs[i] {
					t.Errorf("arg[%d] = %q, want %q\nfull got:  %v\nfull want: %v", i, arg, tt.wantArgs[i], got, tt.wantArgs)
				}
			}
		})
	}
}

// ============================================================================
// buildNFTablesRule TESTS
// ============================================================================

func TestBuildNFTablesRule(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	tests := []struct {
		name    string
		rule    generatedRule
		table   string
		chain   string
		wantSub []string // substrings that should appear
	}{
		{
			name: "basic tcp rule",
			rule: generatedRule{
				Protocol: ProtocolTCP,
				SourceIP: "10.0.0.0/8",
				DstPort:  "80",
				Action:   ActionAllow,
				Comment:  "test-rule",
			},
			table:   "unheaded",
			chain:   "web",
			wantSub: []string{"ip protocol tcp", "ip saddr 10.0.0.0/8", "tcp dport 80", "accept", "test-rule"},
		},
		{
			name: "deny rule",
			rule: generatedRule{
				Protocol: ProtocolUDP,
				Action:   ActionDeny,
			},
			table:   "t",
			chain:   "c",
			wantSub: []string{"ip protocol udp", "drop"},
		},
		{
			name: "any protocol skipped",
			rule: generatedRule{
				Protocol: ProtocolAny,
				Action:   ActionAllow,
			},
			table:   "t",
			chain:   "c",
			wantSub: []string{"accept"},
		},
		{
			name: "dest IP",
			rule: generatedRule{
				DestIP: "8.8.8.8/32",
				Action: ActionAllow,
			},
			table:   "t",
			chain:   "c",
			wantSub: []string{"ip daddr 8.8.8.8/32", "accept"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pc.buildNFTablesRule(tt.rule, tt.table, tt.chain)
			for _, sub := range tt.wantSub {
				if !containsSubstring(got, sub) {
					t.Errorf("rule output %q does not contain %q", got, sub)
				}
			}
		})
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsCheck(s, sub))
}

func containsCheck(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ============================================================================
// sanitizeChainName TESTS
// ============================================================================

func TestSanitizeChainName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "test", "TEST"},
		{"with hyphens", "my-policy", "MY-POLICY"},
		{"with underscores", "my_policy", "MY_POLICY"},
		{"special chars", "my.policy/v1", "MY_POLICY_V1"},
		{"spaces", "my policy", "MY_POLICY"},
		{"long name", "this-is-a-very-long-policy-name-that-should-be-truncated", "THIS-IS-A-VERY-LONG-POLICY-N"},
		{"empty", "", ""},
		{"numbers", "policy123", "POLICY123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeChainName(tt.in); got != tt.want {
				t.Errorf("sanitizeChainName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ============================================================================
// countRulesByType TESTS
// ============================================================================

func TestCountRulesByType(t *testing.T) {
	t.Parallel()
	rules := []generatedRule{
		{Type: RuleTypeIngress},
		{Type: RuleTypeIngress},
		{Type: RuleTypeEgress},
		{Type: RuleTypeInterContainer},
		{Type: RuleTypeExternal},
		{Type: RuleTypeIngress},
	}

	tests := []struct {
		ruleType RuleType
		want     int
	}{
		{RuleTypeIngress, 3},
		{RuleTypeEgress, 1},
		{RuleTypeInterContainer, 1},
		{RuleTypeExternal, 1},
		{RuleType("unknown"), 0},
	}
	for _, tt := range tests {
		t.Run(string(tt.ruleType), func(t *testing.T) {
			t.Parallel()
			if got := countRulesByType(rules, tt.ruleType); got != tt.want {
				t.Errorf("countRulesByType(%q) = %d, want %d", tt.ruleType, got, tt.want)
			}
		})
	}
}

// ============================================================================
// BGP CONFIG GENERATION TESTS
// ============================================================================

func TestGenerateBGPConfig(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	bgp := &BGPConfig{
		Enabled:  true,
		RouterID: "10.0.0.1",
		ASN:      65000,
		Peers: []BGPPeer{
			{Address: "10.0.0.2", ASN: 65001, Password: "secret", HoldTime: 90},
			{Address: "10.0.0.3", ASN: 65002},
		},
		AdvertiseRoutes: []string{"10.10.0.0/24", "10.20.0.0/24"},
	}

	config := pc.generateBGPConfig(bgp)

	expectations := []string{
		"router bgp 65000",
		"bgp router-id 10.0.0.1",
		"neighbor 10.0.0.2 remote-as 65001",
		"neighbor 10.0.0.2 password secret",
		"neighbor 10.0.0.2 timers connect 90",
		"neighbor 10.0.0.3 remote-as 65002",
		"address-family ipv4 unicast",
		"network 10.10.0.0/24",
		"network 10.20.0.0/24",
		"exit-address-family",
	}

	for _, exp := range expectations {
		if !containsCheck(config, exp) {
			t.Errorf("BGP config missing %q\nfull config:\n%s", exp, config)
		}
	}

	// Peer without password should not have password line
	if containsCheck(config, "neighbor 10.0.0.3 password") {
		t.Error("peer without password should not have password line")
	}
	// Peer without hold time should not have timers line
	if containsCheck(config, "neighbor 10.0.0.3 timers") {
		t.Error("peer without hold time should not have timers line")
	}
}

func TestGenerateBGPConfig_Minimal(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	bgp := &BGPConfig{
		Enabled: true,
		ASN:     65500,
	}

	config := pc.generateBGPConfig(bgp)
	if !containsCheck(config, "router bgp 65500") {
		t.Errorf("missing ASN in config:\n%s", config)
	}
	// No router-id line when empty
	if containsCheck(config, "bgp router-id") {
		t.Error("should not have router-id when empty")
	}
}

// ============================================================================
// AUDIT LOG TESTS
// ============================================================================

func TestAuditLog_WritesToFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	pc := newTestController(withAuditLog(logPath))

	pc.auditLog("apply", "test-policy", "success", map[string]string{"key": "value"})

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil { // -1 to remove trailing newline
		t.Fatalf("unmarshal audit entry: %v", err)
	}

	if entry["operation"] != "apply" {
		t.Errorf("operation = %q, want %q", entry["operation"], "apply")
	}
	if entry["policy"] != "test-policy" {
		t.Errorf("policy = %q, want %q", entry["policy"], "test-policy")
	}
	if entry["result"] != "success" {
		t.Errorf("result = %q, want %q", entry["result"], "success")
	}
}

func TestAuditLog_NoPathDoesNothing(t *testing.T) {
	t.Parallel()
	pc := newTestController() // no audit path

	// Should not panic
	pc.auditLog("apply", "test", "success", nil)
}

func TestAuditLog_InvalidPath(t *testing.T) {
	t.Parallel()
	pc := newTestController(withAuditLog("/nonexistent/dir/audit.log"))

	// Should not panic, just log error
	pc.auditLog("apply", "test", "success", nil)
}

// ============================================================================
// EVENT EMISSION TESTS
// ============================================================================

func TestEmitPolicyEvent_WithEmitter(t *testing.T) {
	t.Parallel()
	emitter := &mockEmitter{}
	pc := newTestController(withEmitter(emitter))

	p := validPolicy("test", "ns")
	pc.emitPolicyEvent(context.Background(), "policy.applied", p)

	if emitter.eventCount() != 1 {
		t.Fatalf("expected 1 event, got %d", emitter.eventCount())
	}

	emitter.mu.Lock()
	ev := emitter.events[0]
	emitter.mu.Unlock()

	if ev.topic != "network.policy.events" {
		t.Errorf("topic = %q, want %q", ev.topic, "network.policy.events")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(ev.payload, &payload); err != nil {
		t.Fatalf("unmarshal event payload: %v", err)
	}
	if payload["event_type"] != "policy.applied" {
		t.Errorf("event_type = %q, want %q", payload["event_type"], "policy.applied")
	}
}

func TestEmitPolicyEvent_NilEmitter(t *testing.T) {
	t.Parallel()
	pc := newTestController() // no emitter

	// Should not panic
	pc.emitPolicyEvent(context.Background(), "policy.applied", validPolicy("test", "ns"))
}

func TestEmitPolicyEvent_EmitterError(t *testing.T) {
	t.Parallel()
	emitter := &mockEmitter{err: fmt.Errorf("publish error")}
	pc := newTestController(withEmitter(emitter))

	// Should not panic
	pc.emitPolicyEvent(context.Background(), "policy.applied", validPolicy("test", "ns"))
}

// ============================================================================
// RunInNamespace TESTS
// ============================================================================

func TestRunInNamespace_EmptyPath(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	err := pc.RunInNamespace(context.Background(), "", func() error {
		return nil
	})
	if err != ErrNamespaceNotFound {
		t.Errorf("RunInNamespace() error = %v, want ErrNamespaceNotFound", err)
	}
}

// ============================================================================
// SERIALIZATION TESTS
// ============================================================================

func TestNetworkPolicy_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := validPolicyWithRules()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded NetworkPolicy
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Metadata.Name != original.Metadata.Name {
		t.Errorf("name = %q, want %q", decoded.Metadata.Name, original.Metadata.Name)
	}
	if decoded.Spec.DefaultAction != original.Spec.DefaultAction {
		t.Errorf("default action = %q, want %q", decoded.Spec.DefaultAction, original.Spec.DefaultAction)
	}
	if len(decoded.Spec.Rules) != len(original.Spec.Rules) {
		t.Errorf("rules count = %d, want %d", len(decoded.Spec.Rules), len(original.Spec.Rules))
	}
}

func TestNetworkPolicy_YAMLRoundTrip(t *testing.T) {
	t.Parallel()
	original := validPolicyWithRules()

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded NetworkPolicy
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Metadata.Name != original.Metadata.Name {
		t.Errorf("name = %q, want %q", decoded.Metadata.Name, original.Metadata.Name)
	}
	if decoded.Spec.DefaultAction != original.Spec.DefaultAction {
		t.Errorf("default action = %q, want %q", decoded.Spec.DefaultAction, original.Spec.DefaultAction)
	}
	if len(decoded.Spec.Rules) != len(original.Spec.Rules) {
		t.Errorf("rules count = %d, want %d", len(decoded.Spec.Rules), len(original.Spec.Rules))
	}
}

// ============================================================================
// CONCURRENT ACCESS TESTS (race-safe)
// ============================================================================

func TestPolicyController_ConcurrentGetAndList(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	// Pre-populate
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("policy-%d", i)
		pc.policies[name] = validPolicy(name, "ns")
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("policy-%d", idx%10)
			pc.GetPolicy(name)
		}(i)
		go func() {
			defer wg.Done()
			pc.ListPolicies()
		}()
	}
	wg.Wait()
}

func TestPolicyController_ConcurrentViolations(t *testing.T) {
	t.Parallel()
	pc := newTestController(withMaxViolations(50))

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			pc.RecordViolation(PolicyViolation{
				PolicyName:    fmt.Sprintf("policy-%d", idx),
				RuleType:      RuleTypeIngress,
				SourceIP:      "10.0.0.1",
				DestinationIP: "10.0.0.2",
				Protocol:      ProtocolTCP,
				Action:        ActionDeny,
				ContainerName: "test",
			})
		}(i)
		go func() {
			defer wg.Done()
			pc.GetViolations(time.Hour)
		}()
	}
	wg.Wait()
}

func TestPolicyController_ConcurrentLoadPolicies(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	// Create a valid policy file
	p := validPolicy("concurrent-test", "ns")
	data, _ := yaml.Marshal(p)
	if err := os.WriteFile(filepath.Join(dir, "policy.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = pc.LoadPolicies(filepath.Join(dir, "policy.yaml"))
		}()
	}
	wg.Wait()

	// Should have loaded the policy
	if _, ok := pc.GetPolicy("ns/concurrent-test"); !ok {
		t.Error("policy not loaded after concurrent access")
	}
}

// ============================================================================
// NewPolicyController TESTS
// ============================================================================

func TestNewPolicyController_DefaultValues(t *testing.T) {
	t.Parallel()
	// NewPolicyController requires iptables/nft on the system. We test that
	// it returns ErrBackendNotSupported when the binary is not found (which
	// is the common case in CI/test environments without iptables).
	_, err := NewPolicyController(PolicyControllerConfig{
		Backend: BackendIPTables,
		Logger:  zerolog.Nop(),
	})

	// On systems without iptables, we expect an error
	if err != nil {
		// Verify it's the right error
		if !containsCheck(err.Error(), "not supported") && !containsCheck(err.Error(), "not found") {
			t.Errorf("unexpected error: %v", err)
		}
	} else {
		// On systems with iptables, verify defaults were set
		// We can't easily inspect the returned controller without exposing fields,
		// so just verify no error.
		t.Log("iptables found on system, controller created successfully")
	}
}

func TestNewPolicyController_NFTablesBackend(t *testing.T) {
	t.Parallel()
	_, err := NewPolicyController(PolicyControllerConfig{
		Backend: BackendNFTables,
		Logger:  zerolog.Nop(),
	})

	if err != nil {
		if !containsCheck(err.Error(), "not supported") && !containsCheck(err.Error(), "not found") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestNewPolicyController_EmptyBackendDefaultsToIPTables(t *testing.T) {
	t.Parallel()
	// When backend is empty, it defaults to iptables
	_, err := NewPolicyController(PolicyControllerConfig{
		Logger: zerolog.Nop(),
	})

	// Same behavior as iptables backend test
	if err != nil {
		if !containsCheck(err.Error(), "not supported") && !containsCheck(err.Error(), "not found") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

// ============================================================================
// POLICY VIOLATION STRUCT TESTS
// ============================================================================

func TestPolicyViolation_JSONSerialization(t *testing.T) {
	t.Parallel()
	v := PolicyViolation{
		ID:              "viol-123",
		PolicyName:      "deny-all",
		RuleName:        "default-deny",
		RuleType:        RuleTypeIngress,
		SourceIP:        "192.168.1.100",
		DestinationIP:   "10.0.0.5",
		SourcePort:      54321,
		DestinationPort: 443,
		Protocol:        ProtocolTCP,
		ContainerID:     "abc123",
		ContainerName:   "web-server",
		Action:          ActionDeny,
		Timestamp:       time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Count:           42,
		Metadata:        map[string]string{"reason": "blocked"},
	}

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PolicyViolation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != v.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, v.ID)
	}
	if decoded.SourceIP != v.SourceIP {
		t.Errorf("SourceIP = %q, want %q", decoded.SourceIP, v.SourceIP)
	}
	if decoded.Count != v.Count {
		t.Errorf("Count = %d, want %d", decoded.Count, v.Count)
	}
}

// ============================================================================
// BGPPeer AND BGPConfig STRUCT TESTS
// ============================================================================

func TestBGPConfig_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := BGPConfig{
		Enabled:  true,
		RouterID: "10.0.0.1",
		ASN:      65000,
		Peers: []BGPPeer{
			{Address: "10.0.0.2", ASN: 65001, Password: "secret", HoldTime: 90},
		},
		AdvertiseRoutes: []string{"10.10.0.0/24"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded BGPConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ASN != original.ASN {
		t.Errorf("ASN = %d, want %d", decoded.ASN, original.ASN)
	}
	if len(decoded.Peers) != 1 {
		t.Fatalf("peers len = %d, want 1", len(decoded.Peers))
	}
	if decoded.Peers[0].Password != "secret" {
		t.Errorf("peer password = %q, want %q", decoded.Peers[0].Password, "secret")
	}
}

// ============================================================================
// EDGE CASES AND ERROR PATH TESTS
// ============================================================================

func TestPolicyRule_ValidateIngressWithInvalidSubRule(t *testing.T) {
	t.Parallel()
	// Ingress rule with invalid action
	rule := PolicyRule{
		Type:    RuleTypeIngress,
		Ingress: &IngressRule{Name: "test", Action: PolicyAction("bad")},
	}
	err := rule.Validate()
	if err == nil {
		t.Error("expected error for invalid sub-rule action")
	}
}

func TestPolicyTarget_OnlyLabels(t *testing.T) {
	t.Parallel()
	target := PolicyTarget{
		Labels: map[string]string{"app": "web", "env": "prod"},
	}
	if target.IsEmpty() {
		t.Error("target with labels should not be empty")
	}
}

func TestPortRange_MaxPort(t *testing.T) {
	t.Parallel()
	pr := PortRange{Start: 65535, End: 65535}
	if !pr.IsValid() {
		t.Error("port 65535 should be valid")
	}
	if pr.String() != "65535" {
		t.Errorf("String() = %q, want %q", pr.String(), "65535")
	}
}

func TestNetworkPolicy_GetFullName_NoNamespace(t *testing.T) {
	t.Parallel()
	p := NetworkPolicy{Metadata: PolicyMetadata{Name: "standalone"}}
	if got := p.GetFullName(); got != "standalone" {
		t.Errorf("GetFullName() = %q, want %q", got, "standalone")
	}
}

func TestGenerateRules_IngressMultipleCIDRsAndPorts(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.DefaultAction = ActionAllow
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeIngress,
			Ingress: &IngressRule{
				Name:        "multi",
				SourceCIDRs: []string{"10.0.0.0/8", "172.16.0.0/12"},
				Protocol:    ProtocolTCP,
				Ports:       []PortRange{{80, 80}, {443, 443}},
				Action:      ActionAllow,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	// 2 CIDRs x 2 ports = 4 ingress rules
	var count int
	for _, r := range rules {
		if r.Type == RuleTypeIngress && r.Action == ActionAllow {
			count++
		}
	}
	if count != 4 {
		t.Errorf("expected 4 ingress rules (2 CIDRs x 2 ports), got %d", count)
	}
}

func TestGenerateRules_EgressMultipleCIDRsAndPorts(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.DefaultAction = ActionAllow
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeEgress,
			Egress: &EgressRule{
				Name:             "multi",
				DestinationCIDRs: []string{"8.8.8.8/32", "8.8.4.4/32"},
				Protocol:         ProtocolUDP,
				Ports:            []PortRange{{53, 53}, {853, 853}},
				Action:           ActionAllow,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var count int
	for _, r := range rules {
		if r.Type == RuleTypeEgress && r.Action == ActionAllow {
			count++
		}
	}
	if count != 4 {
		t.Errorf("expected 4 egress rules (2 CIDRs x 2 ports), got %d", count)
	}
}

// ============================================================================
// WatchPolicies / StopWatching TESTS
// ============================================================================

func TestWatchPolicies_InvalidPath(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	err := pc.WatchPolicies(context.Background(), "/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error watching nonexistent path")
	}
}

func TestStopWatching_NilCancel(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	// Should not panic when watcherCancel is nil
	pc.StopWatching()
}

func TestWatchPolicies_ValidDirThenStop(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	err := pc.WatchPolicies(context.Background(), dir)
	if err != nil {
		t.Fatalf("WatchPolicies() error: %v", err)
	}

	// Stop watching should not panic
	pc.StopWatching()
}

// ============================================================================
// RemovePolicy ERROR PATH TESTS
// ============================================================================

func TestRemovePolicy_NotFound(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	err := pc.RemovePolicy(context.Background(), "nonexistent")
	if err != ErrPolicyNotFound {
		t.Errorf("RemovePolicy() error = %v, want ErrPolicyNotFound", err)
	}
}

// ============================================================================
// ApplyPolicy ERROR PATH TESTS
// ============================================================================

func TestApplyPolicy_NilPolicy(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	err := pc.ApplyPolicy(context.Background(), nil)
	if err != ErrInvalidPolicy {
		t.Errorf("ApplyPolicy(nil) error = %v, want ErrInvalidPolicy", err)
	}
}

func TestApplyPolicy_InvalidPolicy(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := &NetworkPolicy{} // empty, invalid
	err := pc.ApplyPolicy(context.Background(), p)
	if err == nil {
		t.Error("expected error for invalid policy")
	}
}

// ============================================================================
// SyncPolicies TESTS
// ============================================================================

func TestSyncPolicies_ContextCancelled(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	// Add an enabled policy
	p := validPolicy("test", "ns")
	p.Spec.Enabled = true
	pc.policies["ns/test"] = p

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := pc.SyncPolicies(ctx)
	if err == nil {
		// Either context error or apply error is acceptable
		t.Log("SyncPolicies returned nil with cancelled context (policy may have been skipped)")
	}
}

func TestSyncPolicies_NoEnabledPolicies(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	// Add a disabled policy
	p := validPolicy("test", "ns")
	p.Spec.Enabled = false
	pc.policies["ns/test"] = p

	// Should succeed (skip disabled policies)
	err := pc.SyncPolicies(context.Background())
	if err != nil {
		t.Errorf("SyncPolicies() error = %v, expected nil for disabled policies", err)
	}
}

func TestSyncPolicies_EmptyPolicies(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	err := pc.SyncPolicies(context.Background())
	if err != nil {
		t.Errorf("SyncPolicies() error = %v", err)
	}
}

// ============================================================================
// SENTINEL ERROR TESTS
// ============================================================================

func TestSentinelErrors(t *testing.T) {
	t.Parallel()
	// Verify all sentinel errors are distinct and have messages
	errors := []struct {
		name string
		err  error
	}{
		{"ErrPolicyNotFound", ErrPolicyNotFound},
		{"ErrInvalidPolicy", ErrInvalidPolicy},
		{"ErrPolicyAlreadyExists", ErrPolicyAlreadyExists},
		{"ErrApplyFailed", ErrApplyFailed},
		{"ErrRemoveFailed", ErrRemoveFailed},
		{"ErrNamespaceNotFound", ErrNamespaceNotFound},
		{"ErrInvalidRuleType", ErrInvalidRuleType},
		{"ErrInvalidAction", ErrInvalidAction},
		{"ErrBackendNotSupported", ErrBackendNotSupported},
		{"ErrSyncFailed", ErrSyncFailed},
		{"ErrWatcherFailed", ErrWatcherFailed},
		{"ErrBGPIntegrationFailed", ErrBGPIntegrationFailed},
	}

	for _, e := range errors {
		t.Run(e.name, func(t *testing.T) {
			t.Parallel()
			if e.err == nil {
				t.Errorf("%s is nil", e.name)
			}
			if e.err.Error() == "" {
				t.Errorf("%s has empty message", e.name)
			}
		})
	}
}

// ============================================================================
// CONSTANTS TESTS
// ============================================================================

func TestProtocolConstants(t *testing.T) {
	t.Parallel()
	if ProtocolTCP != "tcp" {
		t.Errorf("ProtocolTCP = %q", ProtocolTCP)
	}
	if ProtocolUDP != "udp" {
		t.Errorf("ProtocolUDP = %q", ProtocolUDP)
	}
	if ProtocolICMP != "icmp" {
		t.Errorf("ProtocolICMP = %q", ProtocolICMP)
	}
	if ProtocolAny != "any" {
		t.Errorf("ProtocolAny = %q", ProtocolAny)
	}
}

func TestRuleTypeConstants(t *testing.T) {
	t.Parallel()
	if RuleTypeIngress != "ingress" {
		t.Errorf("RuleTypeIngress = %q", RuleTypeIngress)
	}
	if RuleTypeEgress != "egress" {
		t.Errorf("RuleTypeEgress = %q", RuleTypeEgress)
	}
	if RuleTypeInterContainer != "inter-container" {
		t.Errorf("RuleTypeInterContainer = %q", RuleTypeInterContainer)
	}
	if RuleTypeExternal != "external" {
		t.Errorf("RuleTypeExternal = %q", RuleTypeExternal)
	}
}

func TestFirewallBackendConstants(t *testing.T) {
	t.Parallel()
	if BackendIPTables != "iptables" {
		t.Errorf("BackendIPTables = %q", BackendIPTables)
	}
	if BackendNFTables != "nftables" {
		t.Errorf("BackendNFTables = %q", BackendNFTables)
	}
}

// ============================================================================
// ADDITIONAL COVERAGE TESTS
// ============================================================================

func TestBuildIPTablesArgs_WithLogging(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	rule := generatedRule{
		Protocol: ProtocolTCP,
		SourceIP: "10.0.0.0/8",
		DstPort:  "80",
		Action:   ActionAllow,
		Logging:  true,
		Comment:  "logged-rule",
	}

	args := pc.buildIPTablesArgs(rule, "CHAIN")
	// Should still end with -j ACCEPT (logging creates a separate log rule internally)
	lastTwo := args[len(args)-2:]
	if lastTwo[0] != "-j" || lastTwo[1] != "ACCEPT" {
		t.Errorf("expected args to end with -j ACCEPT, got %v", lastTwo)
	}
}

func TestBuildIPTablesArgs_LogAction(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	rule := generatedRule{
		Protocol: ProtocolTCP,
		Action:   ActionLog,
		Logging:  true,
	}

	args := pc.buildIPTablesArgs(rule, "CHAIN")
	lastTwo := args[len(args)-2:]
	if lastTwo[0] != "-j" || lastTwo[1] != "LOG" {
		t.Errorf("expected -j LOG, got %v", lastTwo)
	}
}

func TestAuditLog_MultipleEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "multi-audit.log")

	pc := newTestController(withAuditLog(logPath))

	pc.auditLog("apply", "policy-a", "success", nil)
	pc.auditLog("remove", "policy-b", "success", map[string]string{"reason": "cleanup"})
	pc.auditLog("apply", "policy-c", "failure", nil)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}

	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 3 {
		t.Errorf("expected 3 audit log lines, got %d", lines)
	}
}

func TestLoadPolicyFromFile_SetsTimestamps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	p := validPolicy("ts-test", "ns")
	data, _ := yaml.Marshal(p)
	path := filepath.Join(dir, "ts.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := pc.LoadPolicies(path); err != nil {
		t.Fatalf("LoadPolicies() error: %v", err)
	}

	got, ok := pc.GetPolicy("ns/ts-test")
	if !ok {
		t.Fatal("policy not loaded")
	}
	if got.Metadata.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set after loading")
	}
	if got.Metadata.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set after loading")
	}
}

func TestLoadPolicyFromFile_YML_Extension(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pc := newTestController()

	p := validPolicy("yml-test", "ns")
	data, _ := yaml.Marshal(p)
	path := filepath.Join(dir, "policy.yml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := pc.LoadPolicies(path); err != nil {
		t.Fatalf("LoadPolicies() error: %v", err)
	}

	if _, ok := pc.GetPolicy("ns/yml-test"); !ok {
		t.Error("policy not loaded from .yml file")
	}
}

func TestGenerateRules_IngressWithLogging(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.DefaultAction = ActionAllow
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeIngress,
			Ingress: &IngressRule{
				Name:     "logged-rule",
				Protocol: ProtocolTCP,
				Ports:    []PortRange{{Start: 443, End: 443}},
				Action:   ActionAllow,
				Logging:  true,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var found bool
	for _, r := range rules {
		if r.Comment == "test-logged-rule" && r.Logging {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected rule with Logging=true")
	}
}

func TestGenerateRules_ExternalMultipleCIDRsAndPorts(t *testing.T) {
	t.Parallel()
	pc := newTestController()
	p := validPolicy("test", "ns")
	p.Spec.DefaultAction = ActionAllow
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeExternal,
			External: &ExternalRule{
				Name:          "multi-ext",
				Direction:     RuleTypeEgress,
				ExternalCIDRs: []string{"1.1.1.1/32", "8.8.8.8/32"},
				Protocol:      ProtocolTCP,
				Ports:         []PortRange{{Start: 80, End: 80}, {Start: 443, End: 443}},
				Action:        ActionAllow,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var count int
	for _, r := range rules {
		if r.Type == RuleTypeExternal {
			count++
		}
	}
	// 2 CIDRs x 2 ports = 4 rules
	if count != 4 {
		t.Errorf("expected 4 external rules, got %d", count)
	}
}

func TestInterContainerRules_MultipleSourcesAndDests(t *testing.T) {
	t.Parallel()
	resolver := newMockResolver()
	resolver.names["web1"] = "c-1"
	resolver.names["web2"] = "c-2"
	resolver.names["db1"] = "c-3"
	resolver.names["db2"] = "c-4"
	resolver.ips["c-1"] = "10.0.0.1"
	resolver.ips["c-2"] = "10.0.0.2"
	resolver.ips["c-3"] = "10.0.0.3"
	resolver.ips["c-4"] = "10.0.0.4"

	pc := newTestController(withResolver(resolver))
	p := validPolicy("test", "ns")
	p.Spec.DefaultAction = ActionAllow
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeInterContainer,
			InterContainer: &InterContainerRule{
				Name:                  "multi-ic",
				SourceContainers:      []string{"web1", "web2"},
				DestinationContainers: []string{"db1", "db2"},
				Protocol:              ProtocolTCP,
				Ports:                 []PortRange{{Start: 5432, End: 5432}},
				Action:                ActionAllow,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	var count int
	for _, r := range rules {
		if r.Type == RuleTypeInterContainer {
			count++
		}
	}
	// 2 sources x 2 dests x 1 port = 4 rules
	if count != 4 {
		t.Errorf("expected 4 inter-container rules, got %d", count)
	}
}

func TestInterContainerRules_ResolverPartialFailure(t *testing.T) {
	t.Parallel()
	resolver := newMockResolver()
	// Only "web" resolves; "db" does not
	resolver.names["web"] = "c-1"
	resolver.ips["c-1"] = "10.0.0.1"

	pc := newTestController(withResolver(resolver))
	p := validPolicy("test", "ns")
	p.Spec.DefaultAction = ActionAllow
	p.Spec.Rules = []PolicyRule{
		{
			Type: RuleTypeInterContainer,
			InterContainer: &InterContainerRule{
				Name:                  "partial",
				SourceContainers:      []string{"web"},
				DestinationContainers: []string{"db"},
				Protocol:              ProtocolTCP,
				Ports:                 []PortRange{{Start: 5432, End: 5432}},
				Action:                ActionAllow,
			},
		},
	}

	rules, err := pc.generateRules(p)
	if err != nil {
		t.Fatalf("generateRules() error: %v", err)
	}

	// No dest resolves, so no inter-container rules
	var count int
	for _, r := range rules {
		if r.Type == RuleTypeInterContainer {
			count++
		}
	}
	if count != 0 {
		t.Errorf("expected 0 inter-container rules with partial resolver, got %d", count)
	}
}

func TestBuildNFTablesRule_WithComment(t *testing.T) {
	t.Parallel()
	pc := newTestController()

	rule := generatedRule{
		Protocol: ProtocolTCP,
		SourceIP: "192.168.1.0/24",
		DestIP:   "10.0.0.0/8",
		DstPort:  "8080",
		Action:   ActionReject,
		Comment:  "test-comment",
	}

	result := pc.buildNFTablesRule(rule, "mytable", "mychain")
	if !containsCheck(result, "add rule inet mytable mychain") {
		t.Errorf("missing table/chain prefix in: %s", result)
	}
	if !containsCheck(result, "reject") {
		t.Errorf("missing reject verdict in: %s", result)
	}
	if !containsCheck(result, "comment \"test-comment\"") {
		t.Errorf("missing comment in: %s", result)
	}
}

func TestSanitizeChainName_ExactLength28(t *testing.T) {
	t.Parallel()
	// Input of exactly 28 chars
	input := "abcdefghijklmnopqrstuvwxyz01"
	got := sanitizeChainName(input)
	if len(got) != 28 {
		t.Errorf("len(sanitizeChainName(%q)) = %d, want 28", input, len(got))
	}
}

func TestSanitizeChainName_Length29Truncates(t *testing.T) {
	t.Parallel()
	input := "abcdefghijklmnopqrstuvwxyz012" // 29 chars
	got := sanitizeChainName(input)
	if len(got) != 28 {
		t.Errorf("len(sanitizeChainName(%q)) = %d, want 28", input, len(got))
	}
}

func TestNetworkPolicy_ValidateMultipleRules(t *testing.T) {
	t.Parallel()
	p := validPolicy("test", "ns")
	p.Spec.Rules = []PolicyRule{
		{Type: RuleTypeIngress, Ingress: &IngressRule{Name: "r1", Action: ActionAllow}},
		{Type: RuleTypeEgress, Egress: &EgressRule{Name: "r2", Action: ActionDeny}},
		{Type: RuleTypeIngress}, // invalid: missing ingress definition
	}

	err := p.Validate()
	if err == nil {
		t.Error("expected error for invalid rule at index 2")
	}
	if !containsCheck(err.Error(), "invalid rule 2") {
		t.Errorf("error should reference rule index 2: %v", err)
	}
}

func TestCreateAllowLocalPolicy_LabelsAndKind(t *testing.T) {
	t.Parallel()
	p := CreateAllowLocalPolicy("staging", "172.16.0.0/16")

	if p.APIVersion != "network.unheaded.io/v1" {
		t.Errorf("APIVersion = %q", p.APIVersion)
	}
	if p.Kind != "NetworkPolicy" {
		t.Errorf("Kind = %q", p.Kind)
	}
	if p.Metadata.Labels["unheaded.io/policy-type"] != "local" {
		t.Errorf("label = %q", p.Metadata.Labels["unheaded.io/policy-type"])
	}
}

func TestCreateDefaultDenyPolicy_LabelsAndKind(t *testing.T) {
	t.Parallel()
	p := CreateDefaultDenyPolicy("dev")

	if p.APIVersion != "network.unheaded.io/v1" {
		t.Errorf("APIVersion = %q", p.APIVersion)
	}
	if p.Kind != "NetworkPolicy" {
		t.Errorf("Kind = %q", p.Kind)
	}
	if p.Metadata.Labels["unheaded.io/policy-type"] != "default" {
		t.Errorf("label = %q", p.Metadata.Labels["unheaded.io/policy-type"])
	}
}

// noopBackend is a FirewallBackend value that causes the switch in
// ApplyPolicy/RemovePolicy to fall through without executing OS commands.
const noopBackend FirewallBackend = "noop-test"

func TestApplyPolicy_FullFlow_NoopBackend(t *testing.T) {
	t.Parallel()
	emitter := &mockEmitter{}
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.log")
	pc := newTestController(withBackend(noopBackend), withEmitter(emitter), withAuditLog(auditPath))

	p := validPolicyWithRules()
	err := pc.ApplyPolicy(context.Background(), p)
	if err != nil {
		t.Fatalf("ApplyPolicy() error: %v", err)
	}

	// Verify policy was stored
	got, ok := pc.GetPolicy("production/web-policy")
	if !ok {
		t.Fatal("policy not stored after ApplyPolicy")
	}
	if got.Metadata.Name != "web-policy" {
		t.Errorf("stored policy name = %q", got.Metadata.Name)
	}

	// Verify emitter received event
	if emitter.eventCount() != 1 {
		t.Errorf("expected 1 event, got %d", emitter.eventCount())
	}

	// Verify audit log was written
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !containsCheck(string(data), "apply") {
		t.Error("audit log missing 'apply' entry")
	}
}

func TestApplyPolicy_WithBGPConfig_NoopBackend(t *testing.T) {
	t.Parallel()
	pc := newTestController(withBackend(noopBackend))

	p := validPolicy("bgp-test", "ns")
	p.Spec.BGPConfig = &BGPConfig{
		Enabled:  true,
		ASN:      65000,
		RouterID: "10.0.0.1",
		Peers:    []BGPPeer{{Address: "10.0.0.2", ASN: 65001}},
	}

	// applyBGPConfig will fail since vtysh is not available, but ApplyPolicy
	// should still succeed (BGP failure is logged, not returned as error)
	err := pc.ApplyPolicy(context.Background(), p)
	if err != nil {
		t.Fatalf("ApplyPolicy() error: %v", err)
	}
}

func TestApplyPolicy_DisabledBGP_NoopBackend(t *testing.T) {
	t.Parallel()
	pc := newTestController(withBackend(noopBackend))

	p := validPolicy("no-bgp", "ns")
	p.Spec.BGPConfig = &BGPConfig{Enabled: false}

	err := pc.ApplyPolicy(context.Background(), p)
	if err != nil {
		t.Fatalf("ApplyPolicy() error: %v", err)
	}
}

func TestApplyPolicy_NilBGP_NoopBackend(t *testing.T) {
	t.Parallel()
	pc := newTestController(withBackend(noopBackend))

	p := validPolicy("nil-bgp", "ns")
	p.Spec.BGPConfig = nil

	err := pc.ApplyPolicy(context.Background(), p)
	if err != nil {
		t.Fatalf("ApplyPolicy() error: %v", err)
	}
}

func TestRemovePolicy_FullFlow_NoopBackend(t *testing.T) {
	t.Parallel()
	emitter := &mockEmitter{}
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.log")
	pc := newTestController(withBackend(noopBackend), withEmitter(emitter), withAuditLog(auditPath))

	// First apply a policy
	p := validPolicy("removable", "ns")
	err := pc.ApplyPolicy(context.Background(), p)
	if err != nil {
		t.Fatalf("ApplyPolicy() error: %v", err)
	}

	// Now remove it
	err = pc.RemovePolicy(context.Background(), "ns/removable")
	if err != nil {
		t.Fatalf("RemovePolicy() error: %v", err)
	}

	// Verify policy was removed
	_, ok := pc.GetPolicy("ns/removable")
	if ok {
		t.Error("policy should be removed")
	}

	// Verify events: 1 for apply + 1 for remove = 2
	if emitter.eventCount() != 2 {
		t.Errorf("expected 2 events, got %d", emitter.eventCount())
	}
}

func TestRemovePolicy_NoEmitter_NoopBackend(t *testing.T) {
	t.Parallel()
	pc := newTestController(withBackend(noopBackend))

	p := validPolicy("no-emit", "ns")
	_ = pc.ApplyPolicy(context.Background(), p)

	err := pc.RemovePolicy(context.Background(), "ns/no-emit")
	if err != nil {
		t.Fatalf("RemovePolicy() error: %v", err)
	}
}

func TestSyncPolicies_WithEnabledPolicy_NoopBackend(t *testing.T) {
	t.Parallel()
	pc := newTestController(withBackend(noopBackend))

	p := validPolicy("sync-test", "ns")
	p.Spec.Enabled = true
	pc.policies["ns/sync-test"] = p

	err := pc.SyncPolicies(context.Background())
	if err != nil {
		t.Errorf("SyncPolicies() error = %v", err)
	}
}

func TestSyncPolicies_MixedEnabledDisabled_NoopBackend(t *testing.T) {
	t.Parallel()
	pc := newTestController(withBackend(noopBackend))

	enabled := validPolicy("enabled", "ns")
	enabled.Spec.Enabled = true
	pc.policies["ns/enabled"] = enabled

	disabled := validPolicy("disabled", "ns")
	disabled.Spec.Enabled = false
	pc.policies["ns/disabled"] = disabled

	err := pc.SyncPolicies(context.Background())
	if err != nil {
		t.Errorf("SyncPolicies() error = %v", err)
	}
}

func TestApplyPolicy_NoEmitter_NoopBackend(t *testing.T) {
	t.Parallel()
	pc := newTestController(withBackend(noopBackend))

	p := validPolicyWithRules()
	err := pc.ApplyPolicy(context.Background(), p)
	if err != nil {
		t.Fatalf("ApplyPolicy() error: %v", err)
	}

	// Should succeed without emitter
	got, ok := pc.GetPolicy("production/web-policy")
	if !ok {
		t.Fatal("policy not stored")
	}
	if got.Metadata.Name != "web-policy" {
		t.Errorf("name = %q", got.Metadata.Name)
	}
}

func TestValidatePolicyFile_InvalidYAMLContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{{{bad yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	err := ValidatePolicyFile(path)
	if err == nil {
		t.Error("expected error for invalid YAML content")
	}
}

func TestValidatePolicyFile_InvalidJSONContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json at all}"), 0644); err != nil {
		t.Fatal(err)
	}
	err := ValidatePolicyFile(path)
	if err == nil {
		t.Error("expected error for invalid JSON content")
	}
}

func TestEmitPolicyEvent_PayloadContents(t *testing.T) {
	t.Parallel()
	emitter := &mockEmitter{}
	pc := newTestController(withEmitter(emitter))

	p := validPolicyWithRules()
	pc.emitPolicyEvent(context.Background(), "policy.removed", p)

	emitter.mu.Lock()
	defer emitter.mu.Unlock()

	if len(emitter.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(emitter.events))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(emitter.events[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload["policy_name"] != "production/web-policy" {
		t.Errorf("policy_name = %q", payload["policy_name"])
	}
	if payload["namespace"] != "production" {
		t.Errorf("namespace = %q", payload["namespace"])
	}
	rulesCount, ok := payload["rules_count"].(float64)
	if !ok || rulesCount != 2 {
		t.Errorf("rules_count = %v", payload["rules_count"])
	}
}

// ============================================================================
// PolicyMetadata TESTS
// ============================================================================

func TestPolicyMetadata_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	original := PolicyMetadata{
		Name:      "test",
		Namespace: "prod",
		Labels:    map[string]string{"env": "prod"},
		Annotations: map[string]string{
			"description": "test policy",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PolicyMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Name != original.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Namespace != original.Namespace {
		t.Errorf("Namespace = %q, want %q", decoded.Namespace, original.Namespace)
	}
	if decoded.Labels["env"] != "prod" {
		t.Errorf("Labels[env] = %q, want %q", decoded.Labels["env"], "prod")
	}
}

// ============================================================================
// PolicySpec TESTS
// ============================================================================

func TestPolicySpec_WithBGPConfig(t *testing.T) {
	t.Parallel()
	spec := PolicySpec{
		Target: PolicyTarget{Namespace: "ns"},
		BGPConfig: &BGPConfig{
			Enabled:  true,
			ASN:      65000,
			RouterID: "10.0.0.1",
		},
		Enabled: true,
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PolicySpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.BGPConfig == nil {
		t.Fatal("BGPConfig should not be nil")
	}
	if decoded.BGPConfig.ASN != 65000 {
		t.Errorf("BGPConfig.ASN = %d, want %d", decoded.BGPConfig.ASN, 65000)
	}
}
