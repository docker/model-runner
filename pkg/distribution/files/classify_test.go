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
