package huggingface

import (
	"testing"
)

func TestSelectGGUFFiles(t *testing.T) {
	tests := []struct {
		name           string
		files          []RepoFile
		requestedQuant string
		expectedFiles  []string // filenames
		expectedMMProj string   // mmproj filename (or "")
	}{
		{
			name: "single file, no selection needed",
			files: []RepoFile{
				{Type: "file", Path: "model.gguf"},
			},
			requestedQuant: "latest",
			expectedFiles:  []string{"model.gguf"},
			expectedMMProj: "",
		},
		{
			name: "multiple quantizations, select Q4_K_M explicitly",
			files: []RepoFile{
				{Type: "file", Path: "model-Q2_K.gguf"},
				{Type: "file", Path: "model-Q4_K_M.gguf"},
				{Type: "file", Path: "model-Q8_0.gguf"},
			},
			requestedQuant: "Q4_K_M",
			expectedFiles:  []string{"model-Q4_K_M.gguf"},
			expectedMMProj: "",
		},
		{
			name: "multiple quantizations, select Q8_0",
			files: []RepoFile{
				{Type: "file", Path: "model-Q2_K.gguf"},
				{Type: "file", Path: "model-Q4_K_M.gguf"},
				{Type: "file", Path: "model-Q8_0.gguf"},
			},
			requestedQuant: "Q8_0",
			expectedFiles:  []string{"model-Q8_0.gguf"},
			expectedMMProj: "",
		},
		{
			name: "latest tag defaults to Q4_K_M",
			files: []RepoFile{
				{Type: "file", Path: "model-Q2_K.gguf"},
				{Type: "file", Path: "model-Q4_K_M.gguf"},
				{Type: "file", Path: "model-Q8_0.gguf"},
			},
			requestedQuant: "latest",
			expectedFiles:  []string{"model-Q4_K_M.gguf"},
			expectedMMProj: "",
		},
		{
			name: "empty tag defaults to Q4_K_M",
			files: []RepoFile{
				{Type: "file", Path: "model-Q2_K.gguf"},
				{Type: "file", Path: "model-Q4_K_M.gguf"},
				{Type: "file", Path: "model-Q8_0.gguf"},
			},
			requestedQuant: "",
			expectedFiles:  []string{"model-Q4_K_M.gguf"},
			expectedMMProj: "",
		},
		{
			name: "no Q4_K_M, fallback to first file",
			files: []RepoFile{
				{Type: "file", Path: "model-Q2_K.gguf"},
				{Type: "file", Path: "model-Q8_0.gguf"},
			},
			requestedQuant: "latest",
			expectedFiles:  []string{"model-Q2_K.gguf"},
			expectedMMProj: "",
		},
		{
			name: "case insensitive matching",
			files: []RepoFile{
				{Type: "file", Path: "model-q4_k_m.gguf"},
				{Type: "file", Path: "model-q8_0.gguf"},
			},
			requestedQuant: "Q4_K_M",
			expectedFiles:  []string{"model-q4_k_m.gguf"},
			expectedMMProj: "",
		},
		{
			name: "multimodal with mmproj, prefers F16",
			files: []RepoFile{
				{Type: "file", Path: "model-Q4_K_M.gguf"},
				{Type: "file", Path: "mmproj-model-f32.gguf"},
				{Type: "file", Path: "mmproj-model-f16.gguf"},
			},
			requestedQuant: "Q4_K_M",
			expectedFiles:  []string{"model-Q4_K_M.gguf"},
			expectedMMProj: "mmproj-model-f16.gguf",
		},
		{
			name: "multimodal with only f32 mmproj",
			files: []RepoFile{
				{Type: "file", Path: "model-Q4_K_M.gguf"},
				{Type: "file", Path: "mmproj-model-f32.gguf"},
			},
			requestedQuant: "Q4_K_M",
			expectedFiles:  []string{"model-Q4_K_M.gguf"},
			expectedMMProj: "mmproj-model-f32.gguf",
		},
		{
			name: "bartowski style naming",
			files: []RepoFile{
				{Type: "file", Path: "Llama-3.2-1B-Instruct-Q2_K.gguf"},
				{Type: "file", Path: "Llama-3.2-1B-Instruct-Q4_K_M.gguf"},
				{Type: "file", Path: "Llama-3.2-1B-Instruct-Q5_K_M.gguf"},
				{Type: "file", Path: "Llama-3.2-1B-Instruct-Q6_K.gguf"},
				{Type: "file", Path: "Llama-3.2-1B-Instruct-Q8_0.gguf"},
				{Type: "file", Path: "Llama-3.2-1B-Instruct-IQ4_XS.gguf"},
			},
			requestedQuant: "Q5_K_M",
			expectedFiles:  []string{"Llama-3.2-1B-Instruct-Q5_K_M.gguf"},
			expectedMMProj: "",
		},
		{
			name: "IQ quantization",
			files: []RepoFile{
				{Type: "file", Path: "model-IQ2_XXS.gguf"},
				{Type: "file", Path: "model-IQ4_XS.gguf"},
				{Type: "file", Path: "model-Q4_K_M.gguf"},
			},
			requestedQuant: "IQ4_XS",
			expectedFiles:  []string{"model-IQ4_XS.gguf"},
			expectedMMProj: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, mmproj := SelectGGUFFiles(tt.files, tt.requestedQuant)

			// Check selected files
			if len(selected) != len(tt.expectedFiles) {
				t.Errorf("SelectGGUFFiles() returned %d files, want %d", len(selected), len(tt.expectedFiles))
				return
			}

			for i, f := range selected {
				if f.Filename() != tt.expectedFiles[i] {
					t.Errorf("SelectGGUFFiles() file[%d] = %q, want %q", i, f.Filename(), tt.expectedFiles[i])
				}
			}

			// Check mmproj
			if tt.expectedMMProj == "" {
				if mmproj != nil {
					t.Errorf("SelectGGUFFiles() mmproj = %q, want nil", mmproj.Filename())
				}
			} else {
				if mmproj == nil {
					t.Errorf("SelectGGUFFiles() mmproj = nil, want %q", tt.expectedMMProj)
				} else if mmproj.Filename() != tt.expectedMMProj {
					t.Errorf("SelectGGUFFiles() mmproj = %q, want %q", mmproj.Filename(), tt.expectedMMProj)
				}
			}
		})
	}
}

func TestContainsQuantization(t *testing.T) {
	tests := []struct {
		filename string
		quant    string
		expected bool
	}{
		{"model-Q4_K_M.gguf", "Q4_K_M", true},
		{"model-Q4_K_M.gguf", "q4_k_m", true}, // case insensitive
		{"model-Q4_K_M.gguf", "Q8_0", false},
		{"model.Q4_K_M.gguf", "Q4_K_M", true}, // dot separator
		{"model_Q4_K_M.gguf", "Q4_K_M", true}, // underscore separator
		{"Llama-3.2-1B-Instruct-Q4_K_M.gguf", "Q4_K_M", true},
		{"model-Q4_K_M-00001-of-00003.gguf", "Q4_K_M", true}, // sharded
		{"model-IQ4_XS.gguf", "IQ4_XS", true},
		{"model.gguf", "Q4_K_M", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename+"_"+tt.quant, func(t *testing.T) {
			result := containsQuantization(tt.filename, tt.quant)
			if result != tt.expected {
				t.Errorf("containsQuantization(%q, %q) = %v, want %v", tt.filename, tt.quant, result, tt.expected)
			}
		})
	}
}

func TestIsMMProjFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"mmproj-model-f16.gguf", true},
		{"mmproj-model-f32.gguf", true},
		{"MMPROJ-model.gguf", true}, // case insensitive
		{"model-Q4_K_M.gguf", false},
		{"model.gguf", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := isMMProjFile(tt.filename)
			if result != tt.expected {
				t.Errorf("isMMProjFile(%q) = %v, want %v", tt.filename, result, tt.expected)
			}
		})
	}
}
