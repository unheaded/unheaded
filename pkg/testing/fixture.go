// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package testing

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Fixture provides test fixture management with setup and teardown.
type Fixture struct {
	t         *testing.T
	setupFns  []func() error
	teardowns []func() error
	tempDirs  []string
	tempFiles []string
	data      map[string]interface{}
	mu        sync.Mutex
}

// NewFixture creates a new test fixture.
func NewFixture(t *testing.T) *Fixture {
	return &Fixture{
		t:    t,
		data: make(map[string]interface{}),
	}
}

// Setup adds a setup function to be called before the test.
func (f *Fixture) Setup(fn func() error) *Fixture {
	f.setupFns = append(f.setupFns, fn)
	return f
}

// Teardown adds a teardown function to be called after the test.
// Teardowns are called in reverse order (LIFO).
func (f *Fixture) Teardown(fn func() error) *Fixture {
	f.teardowns = append(f.teardowns, fn)
	return f
}

// Run executes the setup, runs the test function, and executes teardowns.
func (f *Fixture) Run(testFn func()) {
	f.t.Helper()

	// Run setup functions
	for _, setup := range f.setupFns {
		if err := setup(); err != nil {
			f.t.Fatalf("Setup failed: %v", err)
		}
	}

	// Ensure teardowns run even if test panics
	defer func() {
		// Run teardowns in reverse order
		for i := len(f.teardowns) - 1; i >= 0; i-- {
			if err := f.teardowns[i](); err != nil {
				f.t.Errorf("Teardown failed: %v", err)
			}
		}

		// Clean up temp dirs
		for _, dir := range f.tempDirs {
			_ = os.RemoveAll(dir)
		}

		// Clean up temp files
		for _, file := range f.tempFiles {
			_ = os.Remove(file)
		}
	}()

	testFn()
}

// TempDir creates a temporary directory that will be cleaned up after the test.
func (f *Fixture) TempDir() string {
	f.t.Helper()

	dir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		f.t.Fatalf("Failed to create temp dir: %v", err)
	}

	f.mu.Lock()
	f.tempDirs = append(f.tempDirs, dir)
	f.mu.Unlock()

	return dir
}

// TempFile creates a temporary file with the given content.
func (f *Fixture) TempFile(name, content string) string {
	f.t.Helper()

	dir := f.TempDir()
	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, []byte(content), 0640); err != nil { // #nosec G306 -- 0640 — group-readable only, no world access
		f.t.Fatalf("Failed to create temp file: %v", err)
	}

	f.mu.Lock()
	f.tempFiles = append(f.tempFiles, path)
	f.mu.Unlock()

	return path
}

// Set stores a value in the fixture's data store.
func (f *Fixture) Set(key string, value interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = value
}

// Get retrieves a value from the fixture's data store.
func (f *Fixture) Get(key string) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data[key]
}

// GetString retrieves a string value from the fixture's data store.
func (f *Fixture) GetString(key string) string {
	v := f.Get(key)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// GetInt retrieves an int value from the fixture's data store.
func (f *Fixture) GetInt(key string) int {
	v := f.Get(key)
	if i, ok := v.(int); ok {
		return i
	}
	return 0
}

// Suite provides test suite functionality with shared setup/teardown.
type Suite struct {
	t           *testing.T
	beforeAll   []func() error
	afterAll    []func() error
	beforeEach  []func() error
	afterEach   []func() error
	tests       []namedTest
	data        map[string]interface{}
	mu          sync.Mutex
	setupDone   bool
	cleanupDone bool
}

type namedTest struct {
	name string
	fn   func(*testing.T)
}

// NewSuite creates a new test suite.
func NewSuite(t *testing.T) *Suite {
	return &Suite{
		t:    t,
		data: make(map[string]interface{}),
	}
}

// BeforeAll adds a function to run once before all tests in the suite.
func (s *Suite) BeforeAll(fn func() error) *Suite {
	s.beforeAll = append(s.beforeAll, fn)
	return s
}

// AfterAll adds a function to run once after all tests in the suite.
func (s *Suite) AfterAll(fn func() error) *Suite {
	s.afterAll = append(s.afterAll, fn)
	return s
}

// BeforeEach adds a function to run before each test.
func (s *Suite) BeforeEach(fn func() error) *Suite {
	s.beforeEach = append(s.beforeEach, fn)
	return s
}

// AfterEach adds a function to run after each test.
func (s *Suite) AfterEach(fn func() error) *Suite {
	s.afterEach = append(s.afterEach, fn)
	return s
}

// Test adds a test to the suite.
func (s *Suite) Test(name string, fn func(*testing.T)) *Suite {
	s.tests = append(s.tests, namedTest{name: name, fn: fn})
	return s
}

// Run executes all tests in the suite.
func (s *Suite) Run() {
	s.t.Helper()

	// Run beforeAll
	for _, fn := range s.beforeAll {
		if err := fn(); err != nil {
			s.t.Fatalf("BeforeAll failed: %v", err)
		}
	}
	s.setupDone = true

	// Ensure afterAll runs
	defer func() {
		for i := len(s.afterAll) - 1; i >= 0; i-- {
			if err := s.afterAll[i](); err != nil {
				s.t.Errorf("AfterAll failed: %v", err)
			}
		}
		s.cleanupDone = true
	}()

	// Run tests
	for _, test := range s.tests {
		s.t.Run(test.name, func(t *testing.T) {
			// BeforeEach
			for _, fn := range s.beforeEach {
				if err := fn(); err != nil {
					t.Fatalf("BeforeEach failed: %v", err)
				}
			}

			// AfterEach
			defer func() {
				for i := len(s.afterEach) - 1; i >= 0; i-- {
					if err := s.afterEach[i](); err != nil {
						t.Errorf("AfterEach failed: %v", err)
					}
				}
			}()

			test.fn(t)
		})
	}
}

// Set stores a value in the suite's data store.
func (s *Suite) Set(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Get retrieves a value from the suite's data store.
func (s *Suite) Get(key string) interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[key]
}

// Context provides a nested context for test organization.
type Context struct {
	name        string
	parent      *Context
	beforeEach  []func() error
	afterEach   []func() error
	tests       []namedTest
	subContexts []*Context
}

// Describe creates a new test context (alias for NewContext).
func Describe(name string) *Context {
	return &Context{name: name}
}

// NewContext creates a new test context.
func NewContext(name string) *Context {
	return &Context{name: name}
}

// Context creates a nested context.
func (c *Context) Context(name string, configure func(*Context)) *Context {
	sub := &Context{
		name:   name,
		parent: c,
	}
	configure(sub)
	c.subContexts = append(c.subContexts, sub)
	return c
}

// BeforeEach adds a function to run before each test in this context.
func (c *Context) BeforeEach(fn func() error) *Context {
	c.beforeEach = append(c.beforeEach, fn)
	return c
}

// AfterEach adds a function to run after each test in this context.
func (c *Context) AfterEach(fn func() error) *Context {
	c.afterEach = append(c.afterEach, fn)
	return c
}

// It adds a test to the context.
func (c *Context) It(name string, fn func(*testing.T)) *Context {
	c.tests = append(c.tests, namedTest{name: name, fn: fn})
	return c
}

// Run executes all tests in the context.
func (c *Context) Run(t *testing.T) {
	t.Helper()
	c.runWithHooks(t, nil, nil)
}

func (c *Context) runWithHooks(t *testing.T, parentBefore, parentAfter []func() error) {
	t.Helper()

	// Combine parent and current hooks
	allBefore := append(parentBefore, c.beforeEach...)
	allAfter := append(c.afterEach, parentAfter...)

	t.Run(c.name, func(t *testing.T) {
		// Run tests in this context
		for _, test := range c.tests {
			t.Run(test.name, func(t *testing.T) {
				// Run all before hooks
				for _, fn := range allBefore {
					if err := fn(); err != nil {
						t.Fatalf("BeforeEach failed: %v", err)
					}
				}

				// Run after hooks
				defer func() {
					for i := len(allAfter) - 1; i >= 0; i-- {
						if err := allAfter[i](); err != nil {
							t.Errorf("AfterEach failed: %v", err)
						}
					}
				}()

				test.fn(t)
			})
		}

		// Run nested contexts
		for _, sub := range c.subContexts {
			sub.runWithHooks(t, allBefore, allAfter)
		}
	})
}

// TestEnv provides environment variable management for tests.
type TestEnv struct {
	t        *testing.T
	original map[string]string
	set      map[string]string
}

// NewTestEnv creates a new test environment manager.
func NewTestEnv(t *testing.T) *TestEnv {
	return &TestEnv{
		t:        t,
		original: make(map[string]string),
		set:      make(map[string]string),
	}
}

// Set sets an environment variable for the duration of the test.
func (e *TestEnv) Set(key, value string) *TestEnv {
	e.t.Helper()

	// Store original value
	if _, exists := e.original[key]; !exists {
		if orig, ok := os.LookupEnv(key); ok {
			e.original[key] = orig
		} else {
			e.original[key] = "\x00" // Marker for "not set"
		}
	}

	if err := os.Setenv(key, value); err != nil {
		e.t.Fatalf("Failed to set env var %s: %v", key, err)
	}
	e.set[key] = value

	return e
}

// Unset removes an environment variable for the duration of the test.
func (e *TestEnv) Unset(key string) *TestEnv {
	e.t.Helper()

	// Store original value
	if _, exists := e.original[key]; !exists {
		if orig, ok := os.LookupEnv(key); ok {
			e.original[key] = orig
		} else {
			e.original[key] = "\x00"
		}
	}

	if err := os.Unsetenv(key); err != nil {
		e.t.Fatalf("Failed to unset env var %s: %v", key, err)
	}

	return e
}

// Restore restores all environment variables to their original values.
func (e *TestEnv) Restore() {
	e.t.Helper()

	for key, value := range e.original {
		if value == "\x00" {
			_ = os.Unsetenv(key)
		} else {
			_ = os.Setenv(key, value)
		}
	}
}

// CleanupRegistry allows registering cleanup functions.
type CleanupRegistry struct {
	t        *testing.T
	cleanups []func() error
	mu       sync.Mutex
}

// NewCleanupRegistry creates a new cleanup registry.
func NewCleanupRegistry(t *testing.T) *CleanupRegistry {
	return &CleanupRegistry{t: t}
}

// Register adds a cleanup function.
func (c *CleanupRegistry) Register(fn func() error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanups = append(c.cleanups, fn)
}

// Cleanup runs all registered cleanup functions.
func (c *CleanupRegistry) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Run in reverse order
	for i := len(c.cleanups) - 1; i >= 0; i-- {
		if err := c.cleanups[i](); err != nil {
			c.t.Errorf("Cleanup failed: %v", err)
		}
	}
}

// ResourcePool manages reusable test resources.
type ResourcePool[T any] struct {
	create  func() (T, error)
	destroy func(T) error
	pool    []T
	mu      sync.Mutex
}

// NewResourcePool creates a new resource pool.
func NewResourcePool[T any](create func() (T, error), destroy func(T) error) *ResourcePool[T] {
	return &ResourcePool[T]{
		create:  create,
		destroy: destroy,
	}
}

// Get retrieves a resource from the pool or creates a new one.
func (p *ResourcePool[T]) Get() (T, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.pool) > 0 {
		r := p.pool[len(p.pool)-1]
		p.pool = p.pool[:len(p.pool)-1]
		return r, nil
	}

	return p.create()
}

// Put returns a resource to the pool.
func (p *ResourcePool[T]) Put(r T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pool = append(p.pool, r)
}

// Close destroys all resources in the pool.
func (p *ResourcePool[T]) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for _, r := range p.pool {
		if err := p.destroy(r); err != nil {
			lastErr = err
		}
	}
	p.pool = nil

	return lastErr
}
