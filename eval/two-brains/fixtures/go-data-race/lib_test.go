// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import (
	"sync"
	"testing"
)

func TestCounterConcurrent(t *testing.T) {
	var c Counter
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	if c.Value() != 5000 {
		t.Fatalf("count = %d, want 5000", c.Value())
	}
}
