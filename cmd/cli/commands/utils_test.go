package commands

import (
	"errors"
	"fmt"
	"testing"

)

// TestHandleClientErrorFormat verifies that the error format follows the expected pattern.
func TestHandleClientErrorFormat(t *testing.T) {
	t.Run("error format is message: original error", func(t *testing.T) {
		originalErr := fmt.Errorf("network timeout")
		message := "Failed to fetch data"

		result := handleClientError(originalErr, message)

		expected := fmt.Errorf("%s: %w", message, originalErr).Error()
		if result.Error() != expected {
			t.Errorf("Error format mismatch.\nExpected: %q\nGot: %q", expected, result.Error())
		}

		if !errors.Is(result, originalErr) {
			t.Error("Error wrapping is not preserved - errors.Is() check failed")
		}
	})
}
