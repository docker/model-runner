package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyModelfile_Empty(t *testing.T) {
	var opts packageOptions
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("empty modelfile path: unexpected error: %v", err)
	}
}

func TestApplyModelfile_MissingFile(t *testing.T) {
	opts := packageOptions{modelfile: "/nonexistent/Modelfile"}
	if err := applyModelfile(&opts); err == nil {
		t.Fatal("expected error for missing Modelfile, got nil")
	}
}

func TestApplyModelfile_FROM(t *testing.T) {
	dir := t.TempDir()
	writeModelfile(t, dir, "FROM myorg/llama3:8b\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.fromModel != "myorg/llama3:8b" {
		t.Errorf("fromModel = %q, want %q", opts.fromModel, "myorg/llama3:8b")
	}
}

func TestApplyModelfile_FROM_CLIPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeModelfile(t, dir, "FROM modelfile-model:latest\n")

	opts := packageOptions{
		modelfile: filepath.Join(dir, "Modelfile"),
		fromModel: "cli-model:latest",
	}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.fromModel != "cli-model:latest" {
		t.Errorf("CLI fromModel should take precedence, got %q", opts.fromModel)
	}
}

func TestApplyModelfile_FROM_PathRejected(t *testing.T) {
	cases := []string{
		"FROM ./model.gguf\n",
		"FROM ../model.gguf\n",
		"FROM /absolute/path/model.gguf\n",
	}
	for _, c := range cases {
		dir := t.TempDir()
		writeModelfile(t, dir, c)
		opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
		if err := applyModelfile(&opts); err == nil {
			t.Errorf("expected error for %q, got nil", c)
		}
	}
}

func TestApplyModelfile_GGUF(t *testing.T) {
	dir := t.TempDir()
	ggufFile := filepath.Join(dir, "model.gguf")
	writeFile(t, ggufFile, "fake gguf")
	writeModelfile(t, dir, "GGUF ./model.gguf\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.ggufPath != ggufFile {
		t.Errorf("ggufPath = %q, want %q", opts.ggufPath, ggufFile)
	}
}

func TestApplyModelfile_GGUF_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	ggufFile := filepath.Join(dir, "model.gguf")
	writeFile(t, ggufFile, "fake gguf")
	writeModelfile(t, dir, "GGUF "+ggufFile+"\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.ggufPath != ggufFile {
		t.Errorf("ggufPath = %q, want %q", opts.ggufPath, ggufFile)
	}
}

func TestApplyModelfile_GGUF_IsDir(t *testing.T) {
	dir := t.TempDir()
	writeModelfile(t, dir, "GGUF ./\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestApplyModelfile_SAFETENSORS_DIR(t *testing.T) {
	dir := t.TempDir()
	modelDir := filepath.Join(dir, "weights")
	if err := os.Mkdir(modelDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeModelfile(t, dir, "SAFETENSORS_DIR ./weights\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.safetensorsDir != modelDir {
		t.Errorf("safetensorsDir = %q, want %q", opts.safetensorsDir, modelDir)
	}
}

func TestApplyModelfile_SAFETENSORS_DIR_Alias(t *testing.T) {
	dir := t.TempDir()
	modelDir := filepath.Join(dir, "weights")
	if err := os.Mkdir(modelDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeModelfile(t, dir, "SAFETENSORS-DIR ./weights\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.safetensorsDir != modelDir {
		t.Errorf("safetensorsDir = %q, want %q", opts.safetensorsDir, modelDir)
	}
}

func TestApplyModelfile_SAFETENSORS_DIR_NotDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "notadir.txt")
	writeFile(t, f, "contents")
	writeModelfile(t, dir, "SAFETENSORS_DIR ./notadir.txt\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestApplyModelfile_LICENSE(t *testing.T) {
	dir := t.TempDir()
	lic := filepath.Join(dir, "LICENSE")
	writeFile(t, lic, "MIT")
	writeModelfile(t, dir, "LICENSE ./LICENSE\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts.licensePaths) != 1 || opts.licensePaths[0] != lic {
		t.Errorf("licensePaths = %v, want [%q]", opts.licensePaths, lic)
	}
}

func TestApplyModelfile_LICENSE_Deduplication(t *testing.T) {
	dir := t.TempDir()
	lic := filepath.Join(dir, "LICENSE")
	writeFile(t, lic, "MIT")
	writeModelfile(t, dir, "LICENSE ./LICENSE\nLICENSE ./LICENSE\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts.licensePaths) != 1 {
		t.Errorf("expected 1 license path after deduplication, got %d", len(opts.licensePaths))
	}
}

func TestApplyModelfile_CONTEXT(t *testing.T) {
	dir := t.TempDir()
	writeModelfile(t, dir, "CONTEXT 4096\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.contextSize != 4096 {
		t.Errorf("contextSize = %d, want 4096", opts.contextSize)
	}
	if !opts.contextSizeSet {
		t.Error("contextSizeSet not set")
	}
}

func TestApplyModelfile_CTX_Alias(t *testing.T) {
	dir := t.TempDir()
	writeModelfile(t, dir, "CTX 2048\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.contextSize != 2048 {
		t.Errorf("contextSize = %d, want 2048", opts.contextSize)
	}
}

func TestApplyModelfile_CONTEXT_CLIPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeModelfile(t, dir, "CONTEXT 4096\n")

	opts := packageOptions{
		modelfile:   filepath.Join(dir, "Modelfile"),
		contextSize: 8192,
	}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.contextSize != 8192 {
		t.Errorf("contextSize = %d, want 8192", opts.contextSize)
	}
	if opts.contextSizeSet {
		t.Error("contextSizeSet unexpectedly set")
	}
}

func TestApplyModelfile_CONTEXT_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"zero", "CONTEXT 0\n"},
		{"non-integer", "CONTEXT abc\n"},
		{"float", "CONTEXT 4096.5\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeModelfile(t, dir, tc.content)
			opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
			if err := applyModelfile(&opts); err == nil {
				t.Fatalf("expected error for CONTEXT %q, got nil", tc.content)
			}
		})
	}
}

func TestApplyModelfile_CaseInsensitiveInstructions(t *testing.T) {
	dir := t.TempDir()
	writeModelfile(t, dir, "from myorg/model:latest\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.fromModel != "myorg/model:latest" {
		t.Errorf("fromModel = %q, want %q", opts.fromModel, "myorg/model:latest")
	}
}

func TestApplyModelfile_CommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	writeModelfile(t, dir, `# This is a comment

FROM myorg/model:latest

# Another comment
`)

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.fromModel != "myorg/model:latest" {
		t.Errorf("fromModel = %q, want %q", opts.fromModel, "myorg/model:latest")
	}
}

func TestApplyModelfile_UnknownInstructionIgnored(t *testing.T) {
	dir := t.TempDir()
	// PARAMETER and SYSTEM are Ollama Modelfile instructions irrelevant to packaging.
	writeModelfile(t, dir, "FROM myorg/model:latest\nPARAMETER temperature 0.7\nSYSTEM You are helpful.\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.fromModel != "myorg/model:latest" {
		t.Errorf("fromModel = %q, want %q", opts.fromModel, "myorg/model:latest")
	}
}

func TestApplyModelfile_GGUFPathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	ggufFile := filepath.Join(dir, "my model.gguf")
	writeFile(t, ggufFile, "fake gguf")
	writeModelfile(t, dir, "GGUF \"my model.gguf\"\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.ggufPath != ggufFile {
		t.Errorf("ggufPath = %q, want %q", opts.ggufPath, ggufFile)
	}
}

func TestApplyModelfile_MissingValue(t *testing.T) {
	dir := t.TempDir()
	writeModelfile(t, dir, "FROM\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err == nil {
		t.Fatal("expected error for instruction without value, got nil")
	}
}

func TestApplyModelfile_PathNotFound(t *testing.T) {
	dir := t.TempDir()
	writeModelfile(t, dir, "GGUF ./nonexistent.gguf\n")

	opts := packageOptions{modelfile: filepath.Join(dir, "Modelfile")}
	if err := applyModelfile(&opts); err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}
}

func TestModelfileResolvePath(t *testing.T) {
	base := "/home/user/project"
	tests := []struct {
		name    string
		path    string
		baseDir string
		want    string
	}{
		{
			name:    "relative path",
			path:    "model.gguf",
			baseDir: base,
			want:    "/home/user/project/model.gguf",
		},
		{
			name:    "relative path with subdir",
			path:    "weights/model.gguf",
			baseDir: base,
			want:    "/home/user/project/weights/model.gguf",
		},
		{
			name:    "absolute path unchanged",
			path:    "/data/model.gguf",
			baseDir: base,
			want:    "/data/model.gguf",
		},
		{
			name:    "relative path with dots cleaned",
			path:    "./subdir/../model.gguf",
			baseDir: base,
			want:    "/home/user/project/model.gguf",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := modelfileResolvePath(tc.path, tc.baseDir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// writeModelfile writes content to a file named "Modelfile" in dir.
func writeModelfile(t *testing.T, dir, content string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "Modelfile"), content)
}

// writeFile writes content to path, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write file %q: %v", path, err)
	}
}
