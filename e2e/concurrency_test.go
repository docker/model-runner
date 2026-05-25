//go:build e2e

package e2e

import (
	"fmt"
	"testing"
)

// TestE2E_ConcurrentRequests sends parallel chat completions to verify
// the scheduler handles concurrent slot allocation correctly.
func TestE2E_ConcurrentRequests(t *testing.T) {
	model := ggufModel
	pullModel(t, model)
	t.Cleanup(func() {
		removeModel(t, model)
	})

	for i := range 5 {
		t.Run(fmt.Sprintf("request-%d", i), func(t *testing.T) {
			t.Parallel()
			resp := chatCompletion(t, model, "Say a single word.")
			if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
				t.Fatal("empty response")
			}
		})
	}
}
