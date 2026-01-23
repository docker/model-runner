package progress

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// mockLayer implements oci.Layer for testing
type mockLayer struct {
	size      int64
	diffID    string
	mediaType oci.MediaType
}

func (m *mockLayer) Digest() (oci.Hash, error) {
	return oci.Hash{}, nil
}

func (m *mockLayer) DiffID() (oci.Hash, error) {
	return oci.NewHash(m.diffID)
}

func (m *mockLayer) Compressed() (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockLayer) Uncompressed() (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockLayer) Size() (int64, error) {
	return m.size, nil
}

func (m *mockLayer) MediaType() (oci.MediaType, error) {
	return m.mediaType, nil
}

func newMockLayer(size int64) *mockLayer {
	return &mockLayer{
		size:      size,
		diffID:    "sha256:c7790a0a70161f1bfd441cf157313e9efb8fcd1f0831193101def035ead23b32",
		mediaType: types.MediaTypeGGUF,
	}
}

func TestMessages(t *testing.T) {
	t.Run("writeProgress", func(t *testing.T) {
		var buf bytes.Buffer
		update := oci.Update{
			Complete: 1024 * 1024,
		}
		layer1 := newMockLayer(2016)
		layer2 := newMockLayer(1)

		err := WriteProgress(&buf, PullMsg(update), uint64(layer1.size+layer2.size), uint64(layer1.size), uint64(update.Complete), layer1.diffID, oci.ModePull)
		if err != nil {
			t.Fatalf("Failed to write progress message: %v", err)
		}

		var msg oci.ProgressMessage
		if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if msg.Type != oci.TypeProgress {
			t.Errorf("Expected type %q, got %q", oci.TypeProgress, msg.Type)
		}
		if msg.Mode != oci.ModePull {
			t.Errorf("Expected mode %q, got %q", oci.ModePull, msg.Mode)
		}
		if msg.Message != "Downloaded: 1.00 MB" {
			t.Errorf("Expected message 'Downloaded: 1.00 MB', got '%s'", msg.Message)
		}
		if msg.Total != uint64(2017) {
			t.Errorf("Expected total 2017, got %d", msg.Total)
		}
		if msg.Layer == (oci.ProgressLayer{}) {
			t.Errorf("Expected layer to be set")
		}
		if msg.Layer.ID != "sha256:c7790a0a70161f1bfd441cf157313e9efb8fcd1f0831193101def035ead23b32" {
			t.Errorf("Expected layer ID to be %s, got %s", "sha256:c7790a0a70161f1bfd441cf157313e9efb8fcd1f0831193101def035ead23b32", msg.Layer.ID)
		}
		if msg.Layer.Size != uint64(2016) {
			t.Errorf("Expected layer size to be %d, got %d", 2016, msg.Layer.Size)
		}
		if msg.Layer.Current != uint64(1048576) {
			t.Errorf("Expected layer current to be %d, got %d", 1048576, msg.Layer.Current)
		}
	})

	t.Run("writeSuccess", func(t *testing.T) {
		var buf bytes.Buffer
		err := WriteSuccess(&buf, "Model pulled successfully", oci.ModePull)
		if err != nil {
			t.Fatalf("Failed to write success message: %v", err)
		}

		var msg oci.ProgressMessage
		if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if msg.Type != oci.TypeSuccess {
			t.Errorf("Expected type %q, got %q", oci.TypeSuccess, msg.Type)
		}
		if msg.Mode != oci.ModePull {
			t.Errorf("Expected mode %q, got %q", oci.ModePull, msg.Mode)
		}
		if msg.Message != "Model pulled successfully" {
			t.Errorf("Expected message 'Model pulled successfully', got '%s'", msg.Message)
		}
	})

	t.Run("writeError", func(t *testing.T) {
		var buf bytes.Buffer
		err := WriteError(&buf, "Error: something went wrong", oci.ModePull)
		if err != nil {
			t.Fatalf("Failed to write error message: %v", err)
		}

		var msg oci.ProgressMessage
		if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if msg.Type != oci.TypeError {
			t.Errorf("Expected type %q, got %q", oci.TypeError, msg.Type)
		}
		if msg.Mode != oci.ModePull {
			t.Errorf("Expected mode %q, got %q", oci.ModePull, msg.Mode)
		}
		if msg.Message != "Error: something went wrong" {
			t.Errorf("Expected message 'Error: something went wrong', got '%s'", msg.Message)
		}
	})
}

func TestProgressEmissionScenarios(t *testing.T) {
	tests := []struct {
		name          string
		updates       []oci.Update
		delays        []time.Duration
		expectedCount int
		description   string
		layerSize     int64
	}{
		{
			name: "time-based updates",
			updates: []oci.Update{
				{Complete: 100},  // First update always sent
				{Complete: 100},  // Sent after interval
				{Complete: 1000}, // Sent after interval
			},
			delays: []time.Duration{
				UpdateInterval + 100*time.Millisecond,
				UpdateInterval + 100*time.Millisecond,
			},
			expectedCount: 3, // First update + 2 time-based updates
			description:   "should emit updates based on time interval",
			layerSize:     100,
		},
		{
			name: "byte-based updates",
			updates: []oci.Update{
				{Complete: MinBytesForUpdate},     // First update always sent
				{Complete: MinBytesForUpdate * 2}, // Second update with 1MB difference
			},
			delays: []time.Duration{
				10 * time.Millisecond, // Short delay, should trigger based on bytes
			},
			expectedCount: 2, // First update + 1 byte-based update
			description:   "should emit update based on byte threshold",
			layerSize:     MinBytesForUpdate + 1,
		},
		{
			name: "no updates - too frequent",
			updates: []oci.Update{
				{Complete: 100}, // First update always sent
				{Complete: 100}, // Too frequent, no update
				{Complete: 100}, // Too frequent, no update
			},
			delays: []time.Duration{
				10 * time.Millisecond, // Too short
				10 * time.Millisecond, // Too short
			},
			expectedCount: 1, // Only first update
			description:   "should not emit updates if too frequent",
			layerSize:     200,
		},
		{
			name: "finsh update",
			updates: []oci.Update{
				{Complete: 100}, // First update always sent
				{Complete: 100}, // Too frequent, no update
				{Complete: 200}, // Too frequent, but finished, report update
			},
			delays: []time.Duration{
				10 * time.Millisecond, // Too short
				10 * time.Millisecond, // Too short
			},
			expectedCount: 2, // first and last update
			description:   "should emit updates if finished",
			layerSize:     200,
		},
		{
			name: "no updates - too few bytes",
			updates: []oci.Update{
				{Complete: 50},                      // First update always sent
				{Complete: MinBytesForUpdate},       // Too few bytes
				{Complete: MinBytesForUpdate + 100}, // enough bytes now
			},
			delays: []time.Duration{
				10 * time.Millisecond,
			},
			expectedCount: 2, // First update and last update
			description:   "should emit updates based on time even with few bytes",
			layerSize:     100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			layer := newMockLayer(tt.layerSize)

			reporter := NewProgressReporter(&buf, PullMsg, 0, layer, oci.ModePull)
			updates := reporter.Updates()

			// Send updates with delays
			for i, update := range tt.updates {
				updates <- update
				if i < len(tt.delays) {
					time.Sleep(tt.delays[i])
				}
			}
			close(updates)

			// Wait for processing to complete
			if err := reporter.Wait(); err != nil {
				t.Fatalf("Reporter.Wait() failed: %v", err)
			}

			// Parse messages
			lines := bytes.Split(buf.Bytes(), []byte("\n"))
			var messages []oci.ProgressMessage
			for _, line := range lines {
				if len(line) == 0 {
					continue
				}
				var msg oci.ProgressMessage
				if err := json.Unmarshal(line, &msg); err != nil {
					t.Fatalf("Failed to parse JSON: %v", err)
				}
				messages = append(messages, msg)
			}

			if len(messages) != tt.expectedCount {
				t.Errorf("%s: expected %d messages, got %d", tt.description, tt.expectedCount, len(messages))
			}

			// Verify message format for any messages received
			for i, msg := range messages {
				if msg.Type != oci.TypeProgress {
					t.Errorf("message %d: expected type %q, got %q", i, oci.TypeProgress, msg.Type)
				}
				if msg.Mode != oci.ModePull {
					t.Errorf("message %d: expected mode %q, got %q", i, oci.ModePull, msg.Mode)
				}
				if msg.Layer.ID == "" {
					t.Errorf("message %d: expected layer ID to be set", i)
				}
				if msg.Layer.Size != uint64(tt.layerSize) {
					t.Errorf("message %d: expected layer size %d, got %d", i, tt.layerSize, msg.Layer.Size)
				}
			}
		})
	}
}
