package files

import (
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     FileType
	}{
		// GGUF files
		{"gguf file", "model.gguf", FileTypeGGUF},
		{"gguf uppercase", "MODEL.GGUF", FileTypeGGUF},
		{"gguf with path", "/path/to/model.gguf", FileTypeGGUF},
		{"gguf shard", "model-00001-of-00015.gguf", FileTypeGGUF},

		// Safetensors files
		{"safetensors file", "model.safetensors", FileTypeSafetensors},
		{"safetensors uppercase", "MODEL.SAFETENSORS", FileTypeSafetensors},
		{"safetensors with path", "/path/to/model.safetensors", FileTypeSafetensors},
		{"safetensors shard", "model-00001-of-00003.safetensors", FileTypeSafetensors},

		// Chat template files
		{"jinja template", "template.jinja", FileTypeChatTemplate},
		{"jinja uppercase", "TEMPLATE.JINJA", FileTypeChatTemplate},
		{"chat_template file", "chat_template.txt", FileTypeChatTemplate},
		{"chat_template json", "chat_template.json", FileTypeChatTemplate},

		// Config files
		{"json config", "config.json", FileTypeConfig},
		{"txt config", "readme.txt", FileTypeConfig},
		{"md config", "README.md", FileTypeConfig},
		{"vocab file", "vocab.vocab", FileTypeConfig},
		{"tokenizer model", "tokenizer.model", FileTypeConfig},
		{"tokenizer model uppercase", "TOKENIZER.MODEL", FileTypeConfig},
		{"generation config", "generation_config.json", FileTypeConfig},
		{"tokenizer config", "tokenizer_config.json", FileTypeConfig},

		// License files
		{"license file", "LICENSE", FileTypeLicense},
		{"license md", "LICENSE.md", FileTypeLicense},
		{"license txt", "license.txt", FileTypeLicense},
		{"licence uk", "LICENCE", FileTypeLicense},
		{"copying", "COPYING", FileTypeLicense},
		{"notice", "NOTICE", FileTypeLicense},

		// Unknown files
		{"unknown bin", "model.bin", FileTypeUnknown},
		{"unknown py", "script.py", FileTypeUnknown},
		{"unknown empty", "", FileTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.filename)
			if got != tt.want {
				t.Errorf("Classify(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestFileTypeString(t *testing.T) {
	tests := []struct {
		ft   FileType
		want string
	}{
		{FileTypeGGUF, "gguf"},
		{FileTypeSafetensors, "safetensors"},
		{FileTypeConfig, "config"},
		{FileTypeLicense, "license"},
		{FileTypeChatTemplate, "chat_template"},
		{FileTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.ft.String()
			if got != tt.want {
				t.Errorf("FileType.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsConfigFile(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"config.json", true},
		{"readme.txt", true},
		{"README.md", true},
		{"tokenizer.model", true},
		{"chat_template.jinja", true},
		{"model.gguf", false},
		{"model.safetensors", false},
		{"LICENSE", false},
		{"unknown.bin", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsConfigFile(tt.filename)
			if got != tt.want {
				t.Errorf("IsConfigFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsWeightFile(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"model.gguf", true},
		{"model.safetensors", true},
		{"MODEL.GGUF", true},
		{"config.json", false},
		{"LICENSE", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsWeightFile(tt.filename)
			if got != tt.want {
				t.Errorf("IsWeightFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsGGUF(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"model.gguf", true},
		{"MODEL.GGUF", true},
		{"model.safetensors", false},
		{"config.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsGGUF(tt.filename)
			if got != tt.want {
				t.Errorf("IsGGUF(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsSafetensors(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"model.safetensors", true},
		{"MODEL.SAFETENSORS", true},
		{"model.gguf", false},
		{"config.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsSafetensors(tt.filename)
			if got != tt.want {
				t.Errorf("IsSafetensors(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsChatTemplate(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"template.jinja", true},
		{"chat_template.txt", true},
		{"config.json", false},
		{"model.gguf", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsChatTemplate(tt.filename)
			if got != tt.want {
				t.Errorf("IsChatTemplate(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestFilterByType(t *testing.T) {
	files := []string{
		"model.gguf",
		"model.safetensors",
		"config.json",
		"LICENSE",
		"template.jinja",
		"unknown.bin",
	}

	tests := []struct {
		name     string
		fileType FileType
		want     []string
	}{
		{"filter gguf", FileTypeGGUF, []string{"model.gguf"}},
		{"filter safetensors", FileTypeSafetensors, []string{"model.safetensors"}},
		{"filter config", FileTypeConfig, []string{"config.json"}},
		{"filter license", FileTypeLicense, []string{"LICENSE"}},
		{"filter chat template", FileTypeChatTemplate, []string{"template.jinja"}},
		{"filter unknown", FileTypeUnknown, []string{"unknown.bin"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterByType(files, tt.fileType)
			if len(got) != len(tt.want) {
				t.Errorf("FilterByType() got %d files, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("FilterByType()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSplitByType(t *testing.T) {
	files := []string{
		"model.gguf",
		"weights.safetensors",
		"config.json",
		"generation_config.json",
		"LICENSE",
		"template.jinja",
		"unknown.bin",
	}

	weights, configs, templates, licenses, unknown := SplitByType(files)

	// Check weights
	if len(weights) != 2 {
		t.Errorf("SplitByType() weights count = %d, want 2", len(weights))
	}

	// Check configs
	if len(configs) != 2 {
		t.Errorf("SplitByType() configs count = %d, want 2", len(configs))
	}

	// Check templates
	if len(templates) != 1 {
		t.Errorf("SplitByType() templates count = %d, want 1", len(templates))
	}

	// Check licenses
	if len(licenses) != 1 {
		t.Errorf("SplitByType() licenses count = %d, want 1", len(licenses))
	}

	// Check unknown
	if len(unknown) != 1 {
		t.Errorf("SplitByType() unknown count = %d, want 1", len(unknown))
	}
}

func TestFilterByType_WithPaths(t *testing.T) {
	files := []string{
		"/path/to/model.gguf",
		"/another/path/config.json",
		"./local/model.safetensors",
	}

	ggufFiles := FilterByType(files, FileTypeGGUF)
	if len(ggufFiles) != 1 || ggufFiles[0] != "/path/to/model.gguf" {
		t.Errorf("FilterByType() with paths failed, got %v", ggufFiles)
	}

	safetensorsFiles := FilterByType(files, FileTypeSafetensors)
	if len(safetensorsFiles) != 1 || safetensorsFiles[0] != "./local/model.safetensors" {
		t.Errorf("FilterByType() with paths failed, got %v", safetensorsFiles)
	}
}
