// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package testing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureBasic(t *testing.T) {
	setupCalled := false
	teardownCalled := false
	testRan := false

	fixture := NewFixture(t)
	fixture.Setup(func() error {
		setupCalled = true
		return nil
	})
	fixture.Teardown(func() error {
		teardownCalled = true
		return nil
	})

	fixture.Run(func() {
		testRan = true
		if !setupCalled {
			t.Error("Setup should be called before test")
		}
	})

	if !testRan {
		t.Error("Test function should have run")
	}
	if !teardownCalled {
		t.Error("Teardown should be called after test")
	}
}

func TestFixtureTempDir(t *testing.T) {
	var tempDir string

	fixture := NewFixture(t)
	fixture.Run(func() {
		tempDir = fixture.TempDir()
		if _, err := os.Stat(tempDir); os.IsNotExist(err) {
			t.Error("Temp dir should exist during test")
		}

		// Create a file in temp dir
		testFile := filepath.Join(tempDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			t.Errorf("Failed to write test file: %v", err)
		}
	})

	// After fixture.Run, temp dir should be cleaned up
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Error("Temp dir should be cleaned up after test")
	}
}

func TestFixtureTempFile(t *testing.T) {
	var tempFile string

	fixture := NewFixture(t)
	fixture.Run(func() {
		tempFile = fixture.TempFile("test.txt", "hello world")

		content, err := os.ReadFile(tempFile)
		if err != nil {
			t.Errorf("Failed to read temp file: %v", err)
		}
		if string(content) != "hello world" {
			t.Errorf("Expected 'hello world', got '%s'", content)
		}
	})
}

func TestFixtureDataStore(t *testing.T) {
	fixture := NewFixture(t)
	fixture.Run(func() {
		fixture.Set("key", "value")
		fixture.Set("number", 42)

		if fixture.GetString("key") != "value" {
			t.Error("Expected 'value'")
		}
		if fixture.GetInt("number") != 42 {
			t.Error("Expected 42")
		}
	})
}

func TestSuiteBasic(t *testing.T) {
	beforeAllCalled := false
	afterAllCalled := false
	beforeEachCount := 0
	afterEachCount := 0
	testCount := 0

	suite := NewSuite(t)
	suite.BeforeAll(func() error {
		beforeAllCalled = true
		return nil
	})
	suite.AfterAll(func() error {
		afterAllCalled = true
		return nil
	})
	suite.BeforeEach(func() error {
		beforeEachCount++
		return nil
	})
	suite.AfterEach(func() error {
		afterEachCount++
		return nil
	})
	suite.Test("test1", func(t *testing.T) {
		testCount++
	})
	suite.Test("test2", func(t *testing.T) {
		testCount++
	})
	suite.Run()

	if !beforeAllCalled {
		t.Error("BeforeAll should be called")
	}
	if !afterAllCalled {
		t.Error("AfterAll should be called")
	}
	if beforeEachCount != 2 {
		t.Errorf("BeforeEach should be called twice, got %d", beforeEachCount)
	}
	if afterEachCount != 2 {
		t.Errorf("AfterEach should be called twice, got %d", afterEachCount)
	}
	if testCount != 2 {
		t.Errorf("Both tests should run, got %d", testCount)
	}
}

func TestSuiteDataStore(t *testing.T) {
	suite := NewSuite(t)
	suite.BeforeAll(func() error {
		suite.Set("shared", "value")
		return nil
	})
	suite.Test("access shared data", func(t *testing.T) {
		if suite.Get("shared") != "value" {
			t.Error("Expected shared value")
		}
	})
	suite.Run()
}

func TestContext(t *testing.T) {
	testRan := false

	ctx := Describe("Calculator")
	ctx.BeforeEach(func() error {
		return nil
	})
	ctx.It("adds numbers", func(t *testing.T) {
		testRan = true
	})
	ctx.Context("when multiplying", func(c *Context) {
		c.It("multiplies numbers", func(t *testing.T) {
			// test
		})
	})
	ctx.Run(t)

	if !testRan {
		t.Error("Test should have run")
	}
}

func TestTestEnv(t *testing.T) {
	env := NewTestEnv(t)

	// Store original value if set
	originalValue := os.Getenv("TEST_VAR")

	env.Set("TEST_VAR", "test_value")
	if os.Getenv("TEST_VAR") != "test_value" {
		t.Error("Expected TEST_VAR to be set")
	}

	env.Unset("TEST_VAR")
	if os.Getenv("TEST_VAR") != "" {
		t.Error("Expected TEST_VAR to be unset")
	}

	env.Restore()

	// Value should be restored to original
	if os.Getenv("TEST_VAR") != originalValue {
		t.Errorf("Expected TEST_VAR to be restored to '%s', got '%s'", originalValue, os.Getenv("TEST_VAR"))
	}
}

func TestCleanupRegistry(t *testing.T) {
	cleanupOrder := []int{}

	registry := NewCleanupRegistry(t)
	registry.Register(func() error {
		cleanupOrder = append(cleanupOrder, 1)
		return nil
	})
	registry.Register(func() error {
		cleanupOrder = append(cleanupOrder, 2)
		return nil
	})
	registry.Register(func() error {
		cleanupOrder = append(cleanupOrder, 3)
		return nil
	})

	registry.Cleanup()

	// Should run in reverse order
	expected := []int{3, 2, 1}
	if len(cleanupOrder) != len(expected) {
		t.Errorf("Expected %d cleanups, got %d", len(expected), len(cleanupOrder))
	}
	for i, v := range expected {
		if cleanupOrder[i] != v {
			t.Errorf("Expected cleanup order %v, got %v", expected, cleanupOrder)
			break
		}
	}
}

func TestResourcePool(t *testing.T) {
	createCount := 0
	destroyCount := 0

	pool := NewResourcePool(
		func() (int, error) {
			createCount++
			return createCount, nil
		},
		func(r int) error {
			destroyCount++
			return nil
		},
	)

	// Get first resource
	r1, err := pool.Get()
	if err != nil {
		t.Fatalf("Failed to get resource: %v", err)
	}
	if r1 != 1 {
		t.Errorf("Expected resource 1, got %d", r1)
	}

	// Return resource
	pool.Put(r1)

	// Get again - should reuse
	r2, err := pool.Get()
	if err != nil {
		t.Fatalf("Failed to get resource: %v", err)
	}
	if r2 != 1 {
		t.Errorf("Expected reused resource 1, got %d", r2)
	}
	if createCount != 1 {
		t.Errorf("Expected 1 create, got %d", createCount)
	}

	// Get new resource
	r3, err := pool.Get()
	if err != nil {
		t.Fatalf("Failed to get resource: %v", err)
	}
	if r3 != 2 {
		t.Errorf("Expected new resource 2, got %d", r3)
	}
	if createCount != 2 {
		t.Errorf("Expected 2 creates, got %d", createCount)
	}

	// Return and close
	pool.Put(r2)
	pool.Put(r3)
	pool.Close()

	if destroyCount != 2 {
		t.Errorf("Expected 2 destroys, got %d", destroyCount)
	}
}
