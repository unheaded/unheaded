// Package network provides container networking management
package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrNetworkNotFound      = errors.New("network not found")
	ErrNetworkExists        = errors.New("network already exists")
	ErrNetworkInUse         = errors.New("network is in use")
	ErrInvalidConfig        = errors.New("invalid network configuration")
	ErrEndpointNotFound     = errors.New("endpoint not found")
	ErrEndpointExists       = errors.New("endpoint already exists")
	ErrIPAMFailed           = errors.New("IP address allocation failed")
	ErrSubnetExhausted      = errors.New("subnet address space exhausted")
)

// ============================================================================
// TYPES
// ============================================================================

// Network represents a container network
type Network struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Driver      string            `json:"driver"`
	Scope       string            `json:"scope"`
	IPAM        IPAMConfig        `json:"ipam"`
	Internal    bool              `json:"internal,omitempty"`
	Attachable  bool              `json:"attachable,omitempty"`
	Ingress     bool              `json:"ingress,omitempty"`
	EnableIPv6  bool              `json:"enable_ipv6,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	Created     time.Time         `json:"created"`
	Containers  map[string]Endpoint `json:"containers,omitempty"`
}

// IPAMConfig represents IP Address Management configuration
type IPAMConfig struct {
	Driver  string       `json:"driver,omitempty"`
	Config  []IPAMPool   `json:"config,omitempty"`
	Options map[string]string `json:"options,omitempty"`
}

// IPAMPool represents an IP address pool
type IPAMPool struct {
	Subnet     string            `json:"subnet"`
	IPRange    string            `json:"ip_range,omitempty"`
	Gateway    string            `json:"gateway,omitempty"`
	AuxAddress map[string]string `json:"aux_addresses,omitempty"`
}

// Endpoint represents a container's connection to a network
type Endpoint struct {
	ID          string `json:"id"`
	NetworkID   string `json:"network_id"`
	ContainerID string `json:"container_id"`
	Name        string `json:"name,omitempty"`
	IPAddress   string `json:"ip_address,omitempty"`
	IPv6Address string `json:"ipv6_address,omitempty"`
	MacAddress  string `json:"mac_address,omitempty"`
	Gateway     string `json:"gateway,omitempty"`
	DNSNames    []string `json:"dns_names,omitempty"`
}

// CreateOptions configures network creation
type CreateOptions struct {
	Driver     string            `json:"driver,omitempty"`
	IPAM       *IPAMConfig       `json:"ipam,omitempty"`
	Internal   bool              `json:"internal,omitempty"`
	Attachable bool              `json:"attachable,omitempty"`
	Ingress    bool              `json:"ingress,omitempty"`
	EnableIPv6 bool              `json:"enable_ipv6,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Options    map[string]string `json:"options,omitempty"`
}

// ConnectOptions configures container connection to network
type ConnectOptions struct {
	Container   string            `json:"container"`
	IPAddress   string            `json:"ip_address,omitempty"`
	IPv6Address string            `json:"ipv6_address,omitempty"`
	MacAddress  string            `json:"mac_address,omitempty"`
	Aliases     []string          `json:"aliases,omitempty"`
	Links       []string          `json:"links,omitempty"`
	DriverOpts  map[string]string `json:"driver_opts,omitempty"`
}

// DisconnectOptions configures container disconnection from network
type DisconnectOptions struct {
	Container string `json:"container"`
	Force     bool   `json:"force,omitempty"`
}

// ListOptions configures network listing
type ListOptions struct {
	Filters map[string]string `json:"filters,omitempty"`
}

// PruneOptions configures network pruning
type PruneOptions struct {
	Filters map[string]string `json:"filters,omitempty"`
}

// PruneReport contains results of network pruning
type PruneReport struct {
	NetworksDeleted []string `json:"networks_deleted"`
}

// ============================================================================
// MANAGER INTERFACE
// ============================================================================

// Manager defines the interface for network management
type Manager interface {
	// Network operations
	Create(ctx context.Context, name string, opts *CreateOptions) (*Network, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (*Network, error)
	List(ctx context.Context, opts *ListOptions) ([]*Network, error)

	// Connection management
	Connect(ctx context.Context, networkID string, opts *ConnectOptions) (*Endpoint, error)
	Disconnect(ctx context.Context, networkID string, opts *DisconnectOptions) error

	// Cleanup
	Prune(ctx context.Context, opts *PruneOptions) (*PruneReport, error)
}

// ============================================================================
// DEFAULT MANAGER IMPLEMENTATION
// ============================================================================

// DefaultManager is a basic network manager implementation
type DefaultManager struct {
	mu       sync.RWMutex
	networks map[string]*Network
	ipam     *IPAMManager
	logger   zerolog.Logger
}

// NewDefaultManager creates a new default network manager
func NewDefaultManager() *DefaultManager {
	return &DefaultManager{
		networks: make(map[string]*Network),
		ipam:     NewIPAMManager(),
		logger:   log.With().Str("component", "network-manager").Logger(),
	}
}

// Create creates a new network
func (m *DefaultManager) Create(ctx context.Context, name string, opts *CreateOptions) (*Network, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info().Str("name", name).Msg("creating network")

	// Check if network exists
	for _, n := range m.networks {
		if n.Name == name {
			return nil, ErrNetworkExists
		}
	}

	if opts == nil {
		opts = &CreateOptions{}
	}

	// Set defaults
	driver := opts.Driver
	if driver == "" {
		driver = "bridge"
	}

	network := &Network{
		ID:         generateNetworkID(),
		Name:       name,
		Driver:     driver,
		Scope:      "local",
		Internal:   opts.Internal,
		Attachable: opts.Attachable,
		Ingress:    opts.Ingress,
		EnableIPv6: opts.EnableIPv6,
		Labels:     opts.Labels,
		Options:    opts.Options,
		Created:    time.Now(),
		Containers: make(map[string]Endpoint),
	}

	// Configure IPAM
	if opts.IPAM != nil {
		network.IPAM = *opts.IPAM
	} else {
		// Default IPAM configuration
		subnet, gateway, err := m.ipam.AllocateSubnet()
		if err != nil {
			return nil, fmt.Errorf("allocate subnet: %w", err)
		}
		network.IPAM = IPAMConfig{
			Driver: "default",
			Config: []IPAMPool{
				{
					Subnet:  subnet,
					Gateway: gateway,
				},
			},
		}
	}

	m.networks[network.ID] = network

	return network, nil
}

// Delete deletes a network
func (m *DefaultManager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info().Str("id", id).Msg("deleting network")

	network, exists := m.networks[id]
	if !exists {
		// Try finding by name
		for _, n := range m.networks {
			if n.Name == id {
				network = n
				exists = true
				break
			}
		}
	}

	if !exists {
		return ErrNetworkNotFound
	}

	// Check if network has containers
	if len(network.Containers) > 0 {
		return ErrNetworkInUse
	}

	delete(m.networks, network.ID)
	return nil
}

// Get retrieves a network by ID or name
func (m *DefaultManager) Get(ctx context.Context, id string) (*Network, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if network, exists := m.networks[id]; exists {
		return network, nil
	}

	// Try finding by name
	for _, n := range m.networks {
		if n.Name == id {
			return n, nil
		}
	}

	return nil, ErrNetworkNotFound
}

// List lists all networks
func (m *DefaultManager) List(ctx context.Context, opts *ListOptions) ([]*Network, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	networks := make([]*Network, 0, len(m.networks))
	for _, n := range m.networks {
		if m.matchFilters(n, opts) {
			networks = append(networks, n)
		}
	}

	return networks, nil
}

// Connect connects a container to a network
func (m *DefaultManager) Connect(ctx context.Context, networkID string, opts *ConnectOptions) (*Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if opts == nil || opts.Container == "" {
		return nil, ErrInvalidConfig
	}

	m.logger.Info().Str("network", networkID).Str("container", opts.Container).Msg("connecting container to network")

	network, exists := m.networks[networkID]
	if !exists {
		// Try finding by name
		for _, n := range m.networks {
			if n.Name == networkID {
				network = n
				exists = true
				break
			}
		}
	}

	if !exists {
		return nil, ErrNetworkNotFound
	}

	// Check if already connected
	if _, exists := network.Containers[opts.Container]; exists {
		return nil, ErrEndpointExists
	}

	// Allocate IP address
	var ipAddress string
	var err error
	if opts.IPAddress != "" {
		ipAddress = opts.IPAddress
	} else if len(network.IPAM.Config) > 0 {
		ipAddress, err = m.ipam.AllocateIP(network.IPAM.Config[0].Subnet)
		if err != nil {
			return nil, fmt.Errorf("allocate IP: %w", err)
		}
	}

	endpoint := Endpoint{
		ID:          generateEndpointID(),
		NetworkID:   network.ID,
		ContainerID: opts.Container,
		IPAddress:   ipAddress,
		IPv6Address: opts.IPv6Address,
		MacAddress:  opts.MacAddress,
		DNSNames:    opts.Aliases,
	}

	if len(network.IPAM.Config) > 0 {
		endpoint.Gateway = network.IPAM.Config[0].Gateway
	}

	network.Containers[opts.Container] = endpoint

	return &endpoint, nil
}

// Disconnect disconnects a container from a network
func (m *DefaultManager) Disconnect(ctx context.Context, networkID string, opts *DisconnectOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if opts == nil || opts.Container == "" {
		return ErrInvalidConfig
	}

	m.logger.Info().Str("network", networkID).Str("container", opts.Container).Msg("disconnecting container from network")

	network, exists := m.networks[networkID]
	if !exists {
		// Try finding by name
		for _, n := range m.networks {
			if n.Name == networkID {
				network = n
				exists = true
				break
			}
		}
	}

	if !exists {
		return ErrNetworkNotFound
	}

	endpoint, exists := network.Containers[opts.Container]
	if !exists {
		if opts.Force {
			return nil
		}
		return ErrEndpointNotFound
	}

	// Release IP address
	if endpoint.IPAddress != "" && len(network.IPAM.Config) > 0 {
		m.ipam.ReleaseIP(network.IPAM.Config[0].Subnet, endpoint.IPAddress)
	}

	delete(network.Containers, opts.Container)
	return nil
}

// Prune removes unused networks
func (m *DefaultManager) Prune(ctx context.Context, opts *PruneOptions) (*PruneReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info().Msg("pruning networks")

	report := &PruneReport{
		NetworksDeleted: []string{},
	}

	for id, network := range m.networks {
		// Skip networks with containers
		if len(network.Containers) > 0 {
			continue
		}

		// Skip built-in networks
		if network.Name == "bridge" || network.Name == "host" || network.Name == "none" {
			continue
		}

		delete(m.networks, id)
		report.NetworksDeleted = append(report.NetworksDeleted, network.Name)
	}

	return report, nil
}

// matchFilters checks if a network matches the given filters
func (m *DefaultManager) matchFilters(network *Network, opts *ListOptions) bool {
	if opts == nil || opts.Filters == nil {
		return true
	}

	for key, value := range opts.Filters {
		switch key {
		case "name":
			if network.Name != value {
				return false
			}
		case "driver":
			if network.Driver != value {
				return false
			}
		case "scope":
			if network.Scope != value {
				return false
			}
		case "label":
			if network.Labels == nil {
				return false
			}
			if _, ok := network.Labels[value]; !ok {
				return false
			}
		}
	}

	return true
}

// ============================================================================
// IPAM MANAGER
// ============================================================================

// IPAMManager handles IP address allocation
type IPAMManager struct {
	mu         sync.Mutex
	subnets    map[string]*SubnetPool
	subnetPool int // Next subnet to allocate (172.17.x.0/24)
}

// SubnetPool tracks IP allocations within a subnet
type SubnetPool struct {
	subnet    string
	gateway   string
	allocated map[string]bool
	nextIP    int
}

// NewIPAMManager creates a new IPAM manager
func NewIPAMManager() *IPAMManager {
	return &IPAMManager{
		subnets:    make(map[string]*SubnetPool),
		subnetPool: 17, // Start at 172.17.0.0/24
	}
}

// AllocateSubnet allocates a new subnet
func (m *IPAMManager) AllocateSubnet() (subnet string, gateway string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate a new subnet in the 172.x.0.0/24 range
	subnet = fmt.Sprintf("172.%d.0.0/24", m.subnetPool)
	gateway = fmt.Sprintf("172.%d.0.1", m.subnetPool)
	m.subnetPool++

	if m.subnetPool > 255 {
		return "", "", ErrSubnetExhausted
	}

	m.subnets[subnet] = &SubnetPool{
		subnet:    subnet,
		gateway:   gateway,
		allocated: make(map[string]bool),
		nextIP:    2, // Start after gateway
	}

	return subnet, gateway, nil
}

// AllocateIP allocates an IP address from a subnet
func (m *IPAMManager) AllocateIP(subnet string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pool, exists := m.subnets[subnet]
	if !exists {
		// Create new pool for this subnet
		_, ipnet, err := net.ParseCIDR(subnet)
		if err != nil {
			return "", ErrInvalidConfig
		}

		gateway := ipnet.IP
		gateway[3] = 1

		pool = &SubnetPool{
			subnet:    subnet,
			gateway:   gateway.String(),
			allocated: make(map[string]bool),
			nextIP:    2,
		}
		m.subnets[subnet] = pool
	}

	// Parse subnet to get base IP
	_, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", ErrInvalidConfig
	}

	// Allocate next available IP
	for pool.nextIP < 255 {
		ip := make(net.IP, 4)
		copy(ip, ipnet.IP.To4())
		ip[3] = byte(pool.nextIP)
		ipStr := ip.String()

		if !pool.allocated[ipStr] {
			pool.allocated[ipStr] = true
			pool.nextIP++
			return ipStr, nil
		}
		pool.nextIP++
	}

	return "", ErrSubnetExhausted
}

// ReleaseIP releases an IP address back to the pool
func (m *IPAMManager) ReleaseIP(subnet, ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pool, exists := m.subnets[subnet]
	if !exists {
		return
	}

	delete(pool.allocated, ip)
}

// ============================================================================
// HELPERS
// ============================================================================

// generateNetworkID generates a unique network ID
func generateNetworkID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

// generateEndpointID generates a unique endpoint ID
func generateEndpointID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

// Ensure DefaultManager implements Manager interface
var _ Manager = (*DefaultManager)(nil)
