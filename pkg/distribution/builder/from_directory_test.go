package builder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFromDirectory(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create test files
	createTestFile(t, tmpDir, "model.safetensors", "fake safetensors content")
	createTestFile(t, tmpDir, "config.json", "{}")
	createTestFile(t, tmpDir, "tokenizer.json", "{}")

	// Create nested structure
	createTestDir(t, tmpDir, "text_encoder")
	createTestFile(t, tmpDir, "text_encoder/model.safetensors", "text encoder weights")
	createTestFile(t, tmpDir, "text_encoder/config.json", "{}")

	// Test basic functionality
	b, err := FromDirectory(tmpDir)
	if err != nil {
		t.Fatalf("FromDirectory failed: %v", err)
	}

	mdl := b.Model()
	layers, err := mdl.Layers()
	if err != nil {
		t.Fatalf("Failed to get layers: %v", err)
	}

	// Should have 5 files
	if len(layers) != 5 {
		t.Errorf("Expected 5 layers, got %d", len(layers))
	}
}

func TestFromDirectoryWithExclusions(t *testing.T) {
	tests := []struct {
		name           string
		files          map[string]string // path -> content
		dirs           []string
		exclusions     []string
		expectedLayers int
		expectedFiles  []string
	}{
		{
			name: "exclude directory by name",
			files: map[string]string{
				"model.safetensors":              "weights",
				"config.json":                    "{}",
				"__pycache__/cache.pyc":          "cached",
				"text_encoder/__pycache__/a.pyc": "cached",
			},
			dirs:           []string{"__pycache__", "text_encoder/__pycache__"},
			exclusions:     []string{"__pycache__"},
			expectedLayers: 2, // model.safetensors, config.json
			expectedFiles:  []string{"model.safetensors", "config.json"},
		},
		{
			name: "exclude file by name",
			files: map[string]string{
				"model.safetensors": "weights",
				"config.json":       "{}",
				"README.md":         "readme",
				"docs/README.md":    "docs readme",
			},
			dirs:           []string{"docs"},
			exclusions:     []string{"README.md"},
			expectedLayers: 2, // model.safetensors, config.json
			expectedFiles:  []string{"model.safetensors", "config.json"},
		},
		{
			name: "exclude by glob pattern",
			files: map[string]string{
				"model.safetensors": "weights",
				"config.json":       "{}",
				"debug.log":         "log content",
				"error.log":         "error content",
				"output.txt":        "output",
			},
			exclusions:     []string{"*.log"},
			expectedLayers: 3, // model.safetensors, config.json, output.txt
			expectedFiles:  []string{"model.safetensors", "config.json", "output.txt"},
		},
		{
			name: "exclude specific path",
			files: map[string]string{
				"model.safetensors":               "weights",
				"config.json":                     "{}",
				"logs/debug.log":                  "log",
				"text_encoder/logs/important.log": "important",
			},
			dirs:           []string{"logs", "text_encoder/logs"},
			exclusions:     []string{"logs/debug.log"},
			expectedLayers: 3, // model.safetensors, config.json, text_encoder/logs/important.log
		},
		{
			name: "exclude directory with trailing slash",
			files: map[string]string{
				"model.safetensors":  "weights",
				"config.json":        "{}",
				"cache/data.bin":     "cached",
				"cache/index.json":   "index",
				"important/data.bin": "important",
			},
			dirs:           []string{"cache", "important"},
			exclusions:     []string{"cache/"},
			expectedLayers: 3, // model.safetensors, config.json, important/data.bin
		},
		{
			name: "multiple exclusions",
			files: map[string]string{
				"model.safetensors":  "weights",
				"config.json":        "{}",
				"README.md":          "readme",
				"debug.log":          "log",
				"__pycache__/a.pyc":  "cache",
				".git/config":        "git", // hidden, already excluded
				"node_modules/a.js":  "js",
				"important/data.bin": "important",
			},
			dirs:           []string{"__pycache__", "node_modules", "important"},
			exclusions:     []string{"__pycache__", "node_modules", "*.log", "README.md"},
			expectedLayers: 3, // model.safetensors, config.json, important/data.bin
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create directories
			for _, dir := range tt.dirs {
				createTestDir(t, tmpDir, dir)
			}

			// Create files
			for path, content := range tt.files {
				createTestFile(t, tmpDir, path, content)
			}

			// Run FromDirectory with exclusions
			b, err := FromDirectory(tmpDir, WithExclusions(tt.exclusions...))
			if err != nil {
				t.Fatalf("FromDirectory failed: %v", err)
			}

			mdl := b.Model()
			layers, err := mdl.Layers()
			if err != nil {
				t.Fatalf("Failed to get layers: %v", err)
			}

			if len(layers) != tt.expectedLayers {
				// Print what we got for debugging
				t.Logf("Got %d layers:", len(layers))
				for i, layer := range layers {
					if dp, ok := layer.(interface{ GetDescriptor() interface{} }); ok {
						t.Logf("  Layer %d: %+v", i, dp.GetDescriptor())
					}
				}
				t.Errorf("Expected %d layers, got %d", tt.expectedLayers, len(layers))
			}
		})
	}
}

func TestShouldExclude(t *testing.T) {
	tests := []struct {
		name       string
		fileName   string
		relPath    string
		isDir      bool
		exclusions []string
		expected   bool
	}{
		// Simple name matching
		{"match file name", "README.md", "README.md", false, []string{"README.md"}, true},
		{"no match file name", "config.json", "config.json", false, []string{"README.md"}, false},
		{"match dir name", "__pycache__", "__pycache__", true, []string{"__pycache__"}, true},

		// Glob patterns
		{"glob match log files", "debug.log", "debug.log", false, []string{"*.log"}, true},
		{"glob no match", "debug.txt", "debug.txt", false, []string{"*.log"}, false},
		{"glob match nested", "error.log", "logs/error.log", false, []string{"*.log"}, true},

		// Directory with trailing slash
		{"dir slash match", "cache", "cache", true, []string{"cache/"}, true},
		{"dir slash no match file", "cache", "cache", false, []string{"cache/"}, false},

		// Path matching
		{"path match exact", "debug.log", "logs/debug.log", false, []string{"logs/debug.log"}, true},
		{"path no match different path", "debug.log", "other/debug.log", false, []string{"logs/debug.log"}, false},

		// Nested directory matching
		{"nested dir match", "__pycache__", "src/__pycache__", true, []string{"__pycache__"}, true},
		{"nested file in excluded dir", "a.pyc", "__pycache__/a.pyc", false, []string{"__pycache__"}, false}, // file, not dir
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &mockFileInfo{name: tt.fileName, isDir: tt.isDir}
			result := shouldExclude(info, tt.relPath, tt.exclusions)
			if result != tt.expected {
				t.Errorf("shouldExclude(%q, %q, %v) = %v, want %v",
					tt.fileName, tt.relPath, tt.exclusions, result, tt.expected)
			}
		})
	}
}

// Helper functions

func createTestDir(t *testing.T, base, path string) {
	t.Helper()
	fullPath := filepath.Join(base, path)
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		t.Fatalf("Failed to create dir %s: %v", path, err)
	}
}

func createTestFile(t *testing.T, base, path, content string) {
	t.Helper()
	fullPath := filepath.Join(base, path)
	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("Failed to create parent dir for %s: %v", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create file %s: %v", path, err)
	}
}

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct {
	name  string
	isDir bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() interface{}   { return nil }

// Need to import time for mockFileInfo
