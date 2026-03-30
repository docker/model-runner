package registry

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs goleak after the test suite to detect goroutine leaks.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
