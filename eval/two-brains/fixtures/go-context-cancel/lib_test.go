// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import (
	"context"
	"errors"
	"testing"
)

func TestRetryStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Retry(ctx, 10, func() error {
		calls++
		cancel()
		return errors.New("boom")
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err = %v after %d calls; want context.Canceled after 1", err, calls)
	}
}

func TestRetrySucceeds(t *testing.T) {
	n := 0
	if err := Retry(context.Background(), 3, func() error {
		n++
		if n < 2 {
			return errors.New("again")
		}
		return nil
	}); err != nil || n != 2 {
		t.Fatalf("err = %v, n = %d", err, n)
	}
}
