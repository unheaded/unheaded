// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

// Registry maps service names to ports.
type Registry struct {
	ports map[string]int
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Add records a service.
func (r *Registry) Add(name string, port int) { r.ports[name] = port }

// Port looks a service up.
func (r *Registry) Port(name string) (int, bool) {
	p, ok := r.ports[name]
	return p, ok
}
