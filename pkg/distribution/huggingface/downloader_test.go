package huggingface

import (
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/internal/archive"
)

func TestCheckRelativeRejectsPathTraversal(t *testing.T) {
	baseDir := t.TempDir()

	tests := []struct {
		name     string
		filePath string
		wantErr  bool
		wantPath string // expected result when no error
	}{
		{
			name:     "simple filename",
			filePath: "model.safetensors",
			wantErr:  false,
			wantPath: filepath.Join(baseDir, "model.safetensors"),
		},
		{
			name:     "nested subdirectory",
			filePath: "subdir/model.gguf",
			wantErr:  false,
			wantPath: filepath.Join(baseDir, "subdir/model.gguf"),
		},
		{
			name:     "deeply nested path",
			filePath: "a/b/c/config.json",
			wantErr:  false,
			wantPath: filepath.Join(baseDir, "a/b/c/config.json"),
		},
		{
			name:     "path traversal with dot-dot",
			filePath: "../../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "path traversal single level",
			filePath: "../evil.txt",
			wantErr:  true,
		},
		{
			name:     "path traversal embedded in subdir",
			filePath: "subdir/../../etc/shadow",
			wantErr:  true,
		},
		{
			name:     "absolute path is rejected",
			filePath: "/etc/passwd",
			wantErr:  true,
		},
		{
			name:     "dot-dot at boundary of base name",
			filePath: "../" + filepath.Base(baseDir) + "-other/evil.txt",
			wantErr:  true,
		},
		{
			name:     "path with dot segment that stays inside",
			filePath: "subdir/../model.gguf",
			wantErr:  false,
			wantPath: filepath.Join(baseDir, "model.gguf"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := archive.CheckRelative(baseDir, tt.filePath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("CheckRelative(%q, %q) = %q, want error", baseDir, tt.filePath, got)
				}
				return
			}
			if err != nil {
				t.Errorf("CheckRelative(%q, %q) returned unexpected error: %v", baseDir, tt.filePath, err)
				return
			}
			if got != tt.wantPath {
				t.Errorf("CheckRelative(%q, %q) = %q, want %q", baseDir, tt.filePath, got, tt.wantPath)
			}
		})
	}
}
