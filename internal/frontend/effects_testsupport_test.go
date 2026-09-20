package frontend

import (
	"testing"
	"time"
)

func receiveWithTimeout[T any](t *testing.T, ch <-chan T, description string) T {
	t.Helper()

	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
		var zero T
		return zero
	}
}
