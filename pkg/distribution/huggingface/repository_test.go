package huggingface

import (
	"testing"
)

func TestFilterModelFiles(t *testing.T) {
	repoFiles := []RepoFile{
		{Type: "file", Path: "model.safetensors", Size: 1000},
		{Type: "file", Path: "config.json", Size: 100},
		{Type: "file", Path: "tokenizer.json", Size: 200},
		{Type: "file", Path: "README.md", Size: 50},
		{Type: "file", Path: "model.py", Size: 500},
		{Type: "directory", Path: "subdir", Size: 0},
		{Type: "file", Path: "model-00001-of-00002.safetensors", Size: 2000},
		{Type: "file", Path: "model-00002-of-00002.safetensors", Size: 2000},
	}

	safetensors, configs := FilterModelFiles(repoFiles)

	if len(safetensors) != 3 {
		t.Errorf("Expected 3 safetensors files, got %d", len(safetensors))
	}
	if len(configs) != 3 {
		t.Errorf("Expected 3 config files, got %d", len(configs))
	}
}

func TestTotalSize(t *testing.T) {
	repoFiles := []RepoFile{
		{Type: "file", Path: "a.safetensors", Size: 1000},
		{Type: "file", Path: "b.safetensors", Size: 2000, LFS: &LFSInfo{Size: 5000}},
	}

	total := TotalSize(repoFiles)
	if total != 6000 { // 1000 + 5000 (LFS size takes precedence)
		t.Errorf("TotalSize() = %d, want 6000", total)
	}
}

func TestRepoFileActualSize(t *testing.T) {
	tests := []struct {
		name string
		file RepoFile
		want int64
	}{
		{
			name: "regular file",
			file: RepoFile{Size: 1000},
			want: 1000,
		},
		{
			name: "LFS file",
			file: RepoFile{Size: 100, LFS: &LFSInfo{Size: 5000}},
			want: 5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.file.ActualSize(); got != tt.want {
				t.Errorf("ActualSize() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsSafetensorsModel(t *testing.T) {
	tests := []struct {
		name  string
		files []RepoFile
		want  bool
	}{
		{
			name: "has safetensors",
			files: []RepoFile{
				{Type: "file", Path: "model.safetensors"},
				{Type: "file", Path: "config.json"},
			},
			want: true,
		},
		{
			name: "no safetensors",
			files: []RepoFile{
				{Type: "file", Path: "config.json"},
				{Type: "file", Path: "README.md"},
			},
			want: false,
		},
		{
			name:  "empty",
			files: []RepoFile{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafetensorsModel(tt.files); got != tt.want {
				t.Errorf("isSafetensorsModel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindMatchingSubdirectory(t *testing.T) {
	tests := []struct {
		name  string
		files []RepoFile
		tag   string
		want  string
	}{
		{
			name: "finds matching directory",
			files: []RepoFile{
				{Type: "directory", Path: "UD-Q4_K_XL"},
				{Type: "directory", Path: "Q4_K_M"},
				{Type: "file", Path: "README.md"},
			},
			tag:  "UD-Q4_K_XL",
			want: "UD-Q4_K_XL",
		},
		{
			name: "case insensitive matching",
			files: []RepoFile{
				{Type: "directory", Path: "UD-Q4_K_XL"},
				{Type: "directory", Path: "Q4_K_M"},
			},
			tag:  "ud-q4_k_xl",
			want: "UD-Q4_K_XL",
		},
		{
			name: "no matching directory",
			files: []RepoFile{
				{Type: "directory", Path: "Q4_K_M"},
				{Type: "file", Path: "README.md"},
			},
			tag:  "UD-Q4_K_XL",
			want: "",
		},
		{
			name: "file with same name is not matched",
			files: []RepoFile{
				{Type: "file", Path: "UD-Q4_K_XL"},
			},
			tag:  "UD-Q4_K_XL",
			want: "",
		},
		{
			name:  "empty files list",
			files: []RepoFile{},
			tag:   "Q4_K_M",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findMatchingSubdirectory(tt.files, tt.tag); got != tt.want {
				t.Errorf("findMatchingSubdirectory() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrefixPaths(t *testing.T) {
	files := []RepoFile{
		{Type: "file", Path: "model-00001-of-00003.gguf", Size: 1000},
		{Type: "file", Path: "model-00002-of-00003.gguf", Size: 1000},
		{Type: "file", Path: "model-00003-of-00003.gguf", Size: 1000},
	}

	result := prefixPaths(files, "UD-Q4_K_XL")

	if len(result) != 3 {
		t.Fatalf("Expected 3 files, got %d", len(result))
	}

	expectedPaths := []string{
		"UD-Q4_K_XL/model-00001-of-00003.gguf",
		"UD-Q4_K_XL/model-00002-of-00003.gguf",
		"UD-Q4_K_XL/model-00003-of-00003.gguf",
	}

	for i, f := range result {
		if f.Path != expectedPaths[i] {
			t.Errorf("File[%d].Path = %q, want %q", i, f.Path, expectedPaths[i])
		}
		// Verify original file properties are preserved
		if f.Size != 1000 {
			t.Errorf("File[%d].Size = %d, want 1000", i, f.Size)
		}
		if f.Type != "file" {
			t.Errorf("File[%d].Type = %q, want 'file'", i, f.Type)
		}
	}

	// Verify original slice is not modified
	if files[0].Path != "model-00001-of-00003.gguf" {
		t.Error("Original slice was modified")
	}
}
