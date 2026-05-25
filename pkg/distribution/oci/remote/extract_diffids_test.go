package remote

import (
	"encoding/json"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/oci"
)

// Valid 64-char hex strings for SHA256 test hashes.
const (
	hexA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hexB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hexC = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	hex1 = "1111111111111111111111111111111111111111111111111111111111111111"
	hex2 = "2222222222222222222222222222222222222222222222222222222222222222"
)

func TestExtractDiffIDs_DockerFormat(t *testing.T) {
	config := map[string]interface{}{
		"rootfs": map[string]interface{}{
			"type":     "rootfs",
			"diff_ids": []string{"sha256:" + hexA, "sha256:" + hexB, "sha256:" + hexC},
		},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	tests := []struct {
		name    string
		index   int
		wantHex string
		wantOk  bool
	}{
		{"first layer", 0, hexA, true},
		{"second layer", 1, hexB, true},
		{"last layer", 2, hexC, true},
		{"index out of bounds", 3, "", false},
		{"negative index", -1, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := extractDiffIDs(raw, tt.index)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantOk {
				if h == (oci.Hash{}) {
					t.Fatal("expected non-zero hash, got zero")
				}
				if h.Hex != tt.wantHex {
					t.Errorf("expected hex %q, got %q", tt.wantHex, h.Hex)
				}
			} else {
				if h != (oci.Hash{}) {
					t.Errorf("expected zero hash, got %v", h)
				}
			}
		})
	}
}

func TestExtractDiffIDs_CNCFModelPackFormat(t *testing.T) {
	config := map[string]interface{}{
		"modelfs": map[string]interface{}{
			"type":    "layers",
			"diffIds": []string{"sha256:" + hex1, "sha256:" + hex2},
		},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	tests := []struct {
		name    string
		index   int
		wantHex string
		wantOk  bool
	}{
		{"first layer", 0, hex1, true},
		{"second layer", 1, hex2, true},
		{"index out of bounds", 2, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := extractDiffIDs(raw, tt.index)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantOk {
				if h == (oci.Hash{}) {
					t.Fatal("expected non-zero hash, got zero")
				}
				if h.Hex != tt.wantHex {
					t.Errorf("expected hex %q, got %q", tt.wantHex, h.Hex)
				}
			} else {
				if h != (oci.Hash{}) {
					t.Errorf("expected zero hash, got %v", h)
				}
			}
		})
	}
}

func TestExtractDiffIDs_DockerTakesPrecedence(t *testing.T) {
	// When both rootfs and modelfs are present, Docker format should win.
	config := map[string]interface{}{
		"rootfs": map[string]interface{}{
			"type":     "rootfs",
			"diff_ids": []string{"sha256:" + hexA},
		},
		"modelfs": map[string]interface{}{
			"type":    "layers",
			"diffIds": []string{"sha256:" + hex1},
		},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	h, err := extractDiffIDs(raw, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.Hex != hexA {
		t.Errorf("expected Docker format to take precedence (hex %q), got %q", hexA, h.Hex)
	}
}

func TestExtractDiffIDs_EmptyConfig(t *testing.T) {
	raw := []byte(`{}`)
	h, err := extractDiffIDs(raw, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h != (oci.Hash{}) {
		t.Errorf("expected zero hash for empty config, got %v", h)
	}
}

func TestExtractDiffIDs_InvalidJSON(t *testing.T) {
	raw := []byte(`not valid json`)
	_, err := extractDiffIDs(raw, 0)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestExtractDiffIDs_MalformedRootFS(t *testing.T) {
	// rootfs exists but is not an object — should fall through gracefully.
	config := map[string]interface{}{
		"rootfs": "not an object",
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	h, err := extractDiffIDs(raw, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h != (oci.Hash{}) {
		t.Errorf("expected zero hash for malformed rootfs, got %v", h)
	}
}

func TestExtractDiffIDs_MalformedModelFS(t *testing.T) {
	// modelfs exists but diffIds contains invalid hashes (not valid SHA256).
	config := map[string]interface{}{
		"modelfs": map[string]interface{}{
			"type":    "layers",
			"diffIds": []string{"not-a-valid-hash"},
		},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	h, err := extractDiffIDs(raw, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h != (oci.Hash{}) {
		t.Errorf("expected zero hash for malformed modelfs hash, got %v", h)
	}
}
