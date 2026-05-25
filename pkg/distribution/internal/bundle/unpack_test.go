package bundle

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/internal/testutil"
	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestValidatePathWithinDirectory(t *testing.T) {
	// Create a temporary directory for testing
	baseDir := t.TempDir()

	tests := []struct {
		name        string
		targetPath  string
		expectError bool
		description string
	}{
		// Valid paths - should pass
		{
			name:        "simple filename",
			targetPath:  "model.safetensors",
			expectError: false,
			description: "Simple filename should be valid",
		},
		{
			name:        "nested directory",
			targetPath:  "text_encoder/model.safetensors",
			expectError: false,
			description: "Nested path should be valid",
		},
		{
			name:        "deeply nested",
			targetPath:  "a/b/c/d/model.safetensors",
			expectError: false,
			description: "Deeply nested path should be valid",
		},

		// Directory traversal attacks - should fail
		{
			name:        "parent directory escape",
			targetPath:  "../etc/passwd",
			expectError: true,
			description: "Parent directory escape should be blocked",
		},
		{
			name:        "multiple parent escape",
			targetPath:  "../../../etc/passwd",
			expectError: true,
			description: "Multiple parent directory escape should be blocked",
		},
		{
			name:        "mixed path with escape",
			targetPath:  "text_encoder/../../../etc/passwd",
			expectError: true,
			description: "Path that starts valid but escapes should be blocked",
		},
		{
			name:        "absolute path unix",
			targetPath:  "/etc/passwd",
			expectError: true,
			description: "Absolute Unix path should be blocked",
		},
		// Note: Windows absolute path test is platform-specific.
		// On Unix, "C:\..." is treated as a relative path (it doesn't start with /),
		// so it would create a file/directory with that name, which is allowed.
		// On Windows, filepath.IsAbs() correctly identifies "C:\" as absolute.

		// Edge cases
		{
			name:        "empty path",
			targetPath:  "",
			expectError: true,
			description: "Empty path should be blocked (filepath.IsLocal returns false for empty)",
		},
		{
			name:        "dot only",
			targetPath:  ".",
			expectError: true,
			description: "Dot path should be blocked",
		},
		{
			name:        "double dot only",
			targetPath:  "..",
			expectError: true,
			description: "Double dot path should be blocked",
		},
		{
			name:        "path with null byte",
			targetPath:  "model\x00.safetensors",
			expectError: true,
			description: "Path with null byte should be blocked (invalid in most filesystems)",
		},

		// Tricky paths that might bypass naive checks
		{
			name:        ".. in middle",
			targetPath:  "foo/../bar/model.safetensors",
			expectError: false,
			description: "Path with .. that stays within directory should be valid",
		},
		{
			name:        "trailing slash",
			targetPath:  "text_encoder/",
			expectError: false,
			description: "Directory path with trailing slash should be valid",
		},

		// Edge cases for filepath.Base sanitization (re-validation after fallback)
		// filepath.Base("foo/..") returns ".." which must be rejected
		{
			name:        "filepath.Base returns dotdot",
			targetPath:  "..",
			expectError: true,
			description: "Double dot (filepath.Base output for 'foo/..') should be blocked",
		},
		// filepath.Base("/") returns "/" which must be rejected
		{
			name:        "filepath.Base returns slash",
			targetPath:  "/",
			expectError: true,
			description: "Slash (filepath.Base output for '/') should be blocked as absolute path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathWithinDirectory(baseDir, tt.targetPath)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for path %q (%s), but got nil", tt.targetPath, tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for path %q (%s), but got: %v", tt.targetPath, tt.description, err)
			}
		})
	}
}

func TestUpdateBundleFieldsFromLayer_CNCFMediaTypes(t *testing.T) {
	tests := []struct {
		name              string
		mediaType         oci.MediaType
		relPath           string
		modelFormat       string
		expectGGUF        string
		expectSafetensors string
	}{
		{
			name:              "Docker safetensors media type",
			mediaType:         types.MediaTypeSafetensors,
			relPath:           "model/model.safetensors",
			modelFormat:       "",
			expectSafetensors: "model/model.safetensors",
		},
		{
			name:              "CNCF format-specific safetensors media type",
			mediaType:         oci.MediaType(modelpack.MediaTypeWeightSafetensors),
			relPath:           "model/model.safetensors",
			modelFormat:       "",
			expectSafetensors: "model/model.safetensors",
		},
		{
			name:        "CNCF format-specific GGUF media type",
			mediaType:   oci.MediaType(modelpack.MediaTypeWeightGGUF),
			relPath:     "model/model.gguf",
			modelFormat: "",
			expectGGUF:  "model/model.gguf",
		},
		{
			name:              "CNCF generic weight raw with safetensors format",
			mediaType:         oci.MediaType(modelpack.MediaTypeWeightRaw),
			relPath:           "model/model.safetensors",
			modelFormat:       string(types.FormatSafetensors),
			expectSafetensors: "model/model.safetensors",
		},
		{
			name:        "CNCF generic weight raw with GGUF format",
			mediaType:   oci.MediaType(modelpack.MediaTypeWeightRaw),
			relPath:     "model/model.gguf",
			modelFormat: string(types.FormatGGUF),
			expectGGUF:  "model/model.gguf",
		},
		{
			name:              "CNCF generic weight raw without format does nothing",
			mediaType:         oci.MediaType(modelpack.MediaTypeWeightRaw),
			relPath:           "model/model.safetensors",
			modelFormat:       "",
			expectSafetensors: "",
			expectGGUF:        "",
		},
		{
			name:              "unknown media type does nothing",
			mediaType:         "application/vnd.cncf.model.weight.config.v1.raw",
			relPath:           "model/config.json",
			modelFormat:       string(types.FormatSafetensors),
			expectSafetensors: "",
			expectGGUF:        "",
		},
		{
			name:        "CNCF generic weight raw with DDUF format",
			mediaType:   oci.MediaType(modelpack.MediaTypeWeightRaw),
			relPath:     "model/model.dduf",
			modelFormat: string(types.FormatDDUF),
			expectGGUF:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := &Bundle{}
			updateBundleFieldsFromLayer(bundle, tt.mediaType, tt.relPath, tt.modelFormat)

			if bundle.safetensorsFile != tt.expectSafetensors {
				t.Errorf("safetensorsFile = %q, want %q", bundle.safetensorsFile, tt.expectSafetensors)
			}
			if bundle.ggufFile != tt.expectGGUF {
				t.Errorf("ggufFile = %q, want %q", bundle.ggufFile, tt.expectGGUF)
			}
			if tt.modelFormat == string(types.FormatDDUF) && bundle.ddufFile != tt.relPath {
				t.Errorf("ddufFile = %q, want %q", bundle.ddufFile, tt.relPath)
			}
		})
	}
}

func TestUpdateBundleFieldsFromLayer_CNCFMMProj(t *testing.T) {
	tests := []struct {
		name         string
		mediaType    oci.MediaType
		relPath      string
		modelFormat  string
		expectMMProj string
		expectGGUF   string
	}{
		{
			name:         "CNCF generic weight raw with mmproj filename",
			mediaType:    oci.MediaType(modelpack.MediaTypeWeightRaw),
			relPath:      "mmproj-BF16.gguf",
			modelFormat:  string(types.FormatGGUF),
			expectMMProj: "mmproj-BF16.gguf",
			expectGGUF:   "",
		},
		{
			name:         "CNCF generic weight raw with mmproj in path (case-insensitive)",
			mediaType:    oci.MediaType(modelpack.MediaTypeWeightRaw),
			relPath:      "MMProj-model-f16.gguf",
			modelFormat:  string(types.FormatGGUF),
			expectMMProj: "MMProj-model-f16.gguf",
			expectGGUF:   "",
		},
		{
			name:         "CNCF generic weight raw with regular GGUF (not mmproj)",
			mediaType:    oci.MediaType(modelpack.MediaTypeWeightRaw),
			relPath:      "model-Q4_K_XL.gguf",
			modelFormat:  string(types.FormatGGUF),
			expectMMProj: "",
			expectGGUF:   "model-Q4_K_XL.gguf",
		},
		{
			name:         "Docker mmproj media type still works",
			mediaType:    types.MediaTypeMultimodalProjector,
			relPath:      "model.mmproj",
			modelFormat:  "",
			expectMMProj: "model.mmproj",
			expectGGUF:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := &Bundle{}
			updateBundleFieldsFromLayer(bundle, tt.mediaType, tt.relPath, tt.modelFormat)

			if bundle.mmprojPath != tt.expectMMProj {
				t.Errorf("mmprojPath = %q, want %q", bundle.mmprojPath, tt.expectMMProj)
			}
			if bundle.ggufFile != tt.expectGGUF {
				t.Errorf("ggufFile = %q, want %q", bundle.ggufFile, tt.expectGGUF)
			}
		})
	}
}

func TestIsMMProjFilePath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"mmproj-BF16.gguf", true},
		{"mmproj-model-f16.gguf", true},
		{"mmproj-model-f32.gguf", true},
		{"MMProj-model.gguf", true},
		{"MMPROJ-model.gguf", true},
		{"some/path/mmproj-BF16.gguf", true},
		{"model-Q4_K_XL.gguf", false},
		{"model.gguf", false},
		{"model.safetensors", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isMMProjFilePath(tt.path)
			if got != tt.expected {
				t.Errorf("isMMProjFilePath(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestIsCNCFModel(t *testing.T) {
	tests := []struct {
		name            string
		configMediaType oci.MediaType
		artifactType    string
		expected        bool
	}{
		{
			name:            "CNCF ModelPack config V1",
			configMediaType: modelpack.MediaTypeModelConfigV1,
			artifactType:    modelpack.ArtifactTypeModelManifest,
			expected:        true,
		},
		{
			name:            "Docker V0.1 config",
			configMediaType: types.MediaTypeModelConfigV01,
			expected:        false,
		},
		{
			name:            "Docker V0.2 config",
			configMediaType: types.MediaTypeModelConfigV02,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal artifact with the given config media type and artifact type
			artifact := &testArtifactWithConfigMediaType{
				configMediaType: tt.configMediaType,
				artifactType:    tt.artifactType,
			}
			result := isCNCFModel(artifact)
			if result != tt.expected {
				t.Errorf("isCNCFModel() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// testArtifactWithConfigMediaType is a minimal ModelArtifact for testing isCNCFModel/isV02Model.
type testArtifactWithConfigMediaType struct {
	configMediaType oci.MediaType
	artifactType    string
}

func (a *testArtifactWithConfigMediaType) Manifest() (*oci.Manifest, error) {
	return &oci.Manifest{
		ArtifactType: a.artifactType,
		Config: oci.Descriptor{
			MediaType: a.configMediaType,
		},
	}, nil
}

// Stubs to satisfy types.ModelArtifact interface (not used in these tests).
func (a *testArtifactWithConfigMediaType) ID() (string, error) { return "", nil }
func (a *testArtifactWithConfigMediaType) Config() (types.ModelConfig, error) {
	return nil, nil
}
func (a *testArtifactWithConfigMediaType) Tags() []string { return nil }
func (a *testArtifactWithConfigMediaType) Descriptor() (types.Descriptor, error) {
	return types.Descriptor{}, nil
}
func (a *testArtifactWithConfigMediaType) GGUFPaths() ([]string, error) { return nil, nil }
func (a *testArtifactWithConfigMediaType) SafetensorsPaths() ([]string, error) {
	return nil, nil
}
func (a *testArtifactWithConfigMediaType) Layers() ([]oci.Layer, error)         { return nil, nil }
func (a *testArtifactWithConfigMediaType) RawConfigFile() ([]byte, error)       { return nil, nil }
func (a *testArtifactWithConfigMediaType) RawManifest() ([]byte, error)         { return nil, nil }
func (a *testArtifactWithConfigMediaType) MediaType() (oci.MediaType, error)    { return "", nil }
func (a *testArtifactWithConfigMediaType) Size() (int64, error)                 { return 0, nil }
func (a *testArtifactWithConfigMediaType) ConfigName() (oci.Hash, error)        { return oci.Hash{}, nil }
func (a *testArtifactWithConfigMediaType) ConfigFile() (*oci.ConfigFile, error) { return nil, nil }
func (a *testArtifactWithConfigMediaType) Digest() (oci.Hash, error)            { return oci.Hash{}, nil }
func (a *testArtifactWithConfigMediaType) LayerByDigest(oci.Hash) (oci.Layer, error) {
	return nil, nil
}
func (a *testArtifactWithConfigMediaType) LayerByDiffID(oci.Hash) (oci.Layer, error) {
	return nil, nil
}

// testLayerWithAnnotation is a minimal oci.Layer that carries a specific media
// type and a filepath annotation. Used to test inferFormatFromLayerAnnotations.
type testLayerWithAnnotation struct {
	mediaType  oci.MediaType
	annotation string
}

func (l *testLayerWithAnnotation) MediaType() (oci.MediaType, error) { return l.mediaType, nil }
func (l *testLayerWithAnnotation) GetDescriptor() oci.Descriptor {
	annotations := map[string]string{}
	if l.annotation != "" {
		annotations[types.AnnotationFilePath] = l.annotation
	}
	return oci.Descriptor{MediaType: l.mediaType, Annotations: annotations}
}

// Stubs to satisfy oci.Layer interface.
func (l *testLayerWithAnnotation) Digest() (oci.Hash, error)            { return oci.Hash{}, nil }
func (l *testLayerWithAnnotation) DiffID() (oci.Hash, error)            { return oci.Hash{}, nil }
func (l *testLayerWithAnnotation) Compressed() (io.ReadCloser, error)   { return nil, nil }
func (l *testLayerWithAnnotation) Uncompressed() (io.ReadCloser, error) { return nil, nil }
func (l *testLayerWithAnnotation) Size() (int64, error)                 { return 0, nil }

func TestInferFormatFromLayerAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		layers   []oci.Layer
		expected string
	}{
		{
			name: "GGUF via MediaTypeWeightRaw annotation",
			layers: []oci.Layer{
				&testLayerWithAnnotation{
					mediaType:  oci.MediaType(modelpack.MediaTypeWeightRaw),
					annotation: "model.gguf",
				},
			},
			expected: string(types.FormatGGUF),
		},
		{
			name: "safetensors via MediaTypeWeightRaw annotation",
			layers: []oci.Layer{
				&testLayerWithAnnotation{
					mediaType:  oci.MediaType(modelpack.MediaTypeWeightRaw),
					annotation: "model.safetensors",
				},
			},
			expected: string(types.FormatSafetensors),
		},
		{
			name: "DDUF via MediaTypeWeightRaw annotation",
			layers: []oci.Layer{
				&testLayerWithAnnotation{
					mediaType:  oci.MediaType(modelpack.MediaTypeWeightRaw),
					annotation: "model.dduf",
				},
			},
			expected: string(types.FormatDDUF),
		},
		{
			name: "no weight layers returns empty",
			layers: []oci.Layer{
				&testLayerWithAnnotation{
					mediaType:  "application/json",
					annotation: "config.json",
				},
			},
			expected: "",
		},
		{
			name:     "empty layers returns empty",
			layers:   []oci.Layer{},
			expected: "",
		},
		{
			name: "weight layer without annotation returns empty",
			layers: []oci.Layer{
				&testLayerWithAnnotation{
					mediaType:  oci.MediaType(modelpack.MediaTypeWeightRaw),
					annotation: "",
				},
			},
			expected: "",
		},
		{
			name: "GGUF via MediaTypeWeightGGUF annotation",
			layers: []oci.Layer{
				&testLayerWithAnnotation{
					mediaType:  oci.MediaType(modelpack.MediaTypeWeightGGUF),
					annotation: "model.gguf",
				},
			},
			expected: string(types.FormatGGUF),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferFormatFromLayerAnnotations(tt.layers)
			if got != tt.expected {
				t.Errorf("inferFormatFromLayerAnnotations() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSanitizeRelativePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no leading dotdot",
			input:    "text_encoder/model.safetensors",
			expected: "text_encoder/model.safetensors",
		},
		{
			name:     "single leading dotdot",
			input:    "../model.gguf",
			expected: "model.gguf",
		},
		{
			name:     "multiple leading dotdots",
			input:    "../../home/user/text_encoder/model.safetensors",
			expected: "home/user/text_encoder/model.safetensors",
		},
		{
			name:     "deep leading dotdots preserving nested dirs",
			input:    "../../../a/b/c/model.safetensors",
			expected: "a/b/c/model.safetensors",
		},
		{
			name:     "only dotdot",
			input:    "..",
			expected: "",
		},
		{
			name:     "simple filename",
			input:    "model.gguf",
			expected: "model.gguf",
		},
		{
			name:     "dotdot in middle is resolved by Clean",
			input:    "a/../b/model.safetensors",
			expected: "b/model.safetensors",
		},
		{
			name:     "trailing slash stripped by Clean",
			input:    "../models/",
			expected: "models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeRelativePath(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeRelativePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestUnpackFromLayers_PathCollisionAfterSanitization(t *testing.T) {
	// Build a ModelPack artifact where two distinct raw annotations collapse to
	// the same sanitized destination path.
	artifact := testutil.NewModelPackArtifact(
		t,
		modelpack.Model{
			Config: modelpack.ModelConfig{Format: string(types.FormatGGUF)},
		},
		testutil.LayerSpec{
			Path:         filepath.Join("..", "..", "assets", "dummy.gguf"),
			RelativePath: "foo/model.gguf",
			MediaType:    oci.MediaType(modelpack.MediaTypeWeightGGUF),
		},
		testutil.LayerSpec{
			Path:         filepath.Join("..", "..", "assets", "dummy.gguf"),
			RelativePath: "../foo/model.gguf",
			MediaType:    oci.MediaType(modelpack.MediaTypeWeightGGUF),
		},
	)

	_, err := UnpackFromLayers(t.TempDir(), artifact)
	if err == nil {
		t.Fatal("Expected unpack collision error, got nil")
	}
	if !strings.Contains(err.Error(), "filepath collision") {
		t.Fatalf("Expected collision error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "foo/model.gguf") {
		t.Fatalf("Expected collision error to mention original annotation, got: %v", err)
	}
	if !strings.Contains(err.Error(), "../foo/model.gguf") {
		t.Fatalf("Expected collision error to mention sanitized annotation, got: %v", err)
	}
}

func TestUnpackFromLayers_DuplicateRawAnnotationAllowed(t *testing.T) {
	// Build a ModelPack artifact with the same raw annotation twice. This
	// should behave like a duplicate layer, not a collision.
	artifact := testutil.NewModelPackArtifact(
		t,
		modelpack.Model{
			Config: modelpack.ModelConfig{Format: string(types.FormatGGUF)},
		},
		testutil.LayerSpec{
			Path:         filepath.Join("..", "..", "assets", "dummy.gguf"),
			RelativePath: "foo/model.gguf",
			MediaType:    oci.MediaType(modelpack.MediaTypeWeightGGUF),
		},
		testutil.LayerSpec{
			Path:         filepath.Join("..", "..", "assets", "dummy.gguf"),
			RelativePath: "foo/model.gguf",
			MediaType:    oci.MediaType(modelpack.MediaTypeWeightGGUF),
		},
	)

	bundleRoot := t.TempDir()
	bundle, err := UnpackFromLayers(bundleRoot, artifact)
	if err != nil {
		t.Fatalf("Expected duplicate annotation to be ignored, got: %v", err)
	}
	if bundle.ggufFile != filepath.Join("foo", "model.gguf") {
		t.Errorf("Expected ggufFile to track unpacked path, got: %s", bundle.ggufFile)
	}
	if _, err := os.Stat(bundle.GGUFPath()); err != nil {
		t.Fatalf("Expected GGUF file to exist after unpack, got: %v", err)
	}
}

func TestUnpackFromLayers_PathSanitizationRejectsCollapsedPath(t *testing.T) {
	// Build a ModelPack artifact whose annotation collapses entirely during
	// sanitization. This must fail before any file is written.
	artifact := testutil.NewModelPackArtifact(
		t,
		modelpack.Model{
			Config: modelpack.ModelConfig{Format: string(types.FormatGGUF)},
		},
		testutil.LayerSpec{
			Path:         filepath.Join("..", "..", "assets", "dummy.gguf"),
			RelativePath: "../../..",
			MediaType:    oci.MediaType(modelpack.MediaTypeWeightGGUF),
		},
	)

	bundleRoot := t.TempDir()
	_, err := UnpackFromLayers(bundleRoot, artifact)
	if err == nil {
		t.Fatal("Expected sanitization error, got nil")
	}
	if !strings.Contains(err.Error(), `invalid filepath annotation "../../.."`) {
		t.Fatalf("Expected error to mention original annotation, got: %v", err)
	}
	if !strings.Contains(err.Error(), `sanitized as ""`) {
		t.Fatalf("Expected error to mention sanitized path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "empty path is not allowed") {
		t.Fatalf("Expected error to mention sanitized validation failure, got: %v", err)
	}

	modelDir := filepath.Join(bundleRoot, ModelSubdir)
	entries, readErr := os.ReadDir(modelDir)
	if readErr != nil {
		t.Fatalf("Expected model directory to exist, got: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("Expected no files to be written for rejected annotation, got %d entries", len(entries))
	}
}

func TestUnpackFromLayers_CNCFModelWithMMProj(t *testing.T) {
	// Simulate the exact scenario from the bug: a CNCF ModelPack model with
	// two layers using MediaTypeWeightRaw — one is the main GGUF weight and
	// the other is the mmproj file. Both share the same media type; mmproj
	// detection relies on the filepath annotation containing "mmproj".
	artifact := testutil.NewModelPackArtifact(
		t,
		modelpack.Model{
			Config: modelpack.ModelConfig{Format: string(types.FormatGGUF)},
		},
		testutil.LayerSpec{
			Path:         filepath.Join("..", "..", "assets", "dummy.gguf"),
			RelativePath: "gemma-4-E2B-it-UD-Q4_K_XL.gguf",
			MediaType:    oci.MediaType(modelpack.MediaTypeWeightRaw),
		},
		testutil.LayerSpec{
			Path:         filepath.Join("..", "..", "assets", "dummy.mmproj"),
			RelativePath: "mmproj-BF16.gguf",
			MediaType:    oci.MediaType(modelpack.MediaTypeWeightRaw),
		},
	)

	bundleRoot := t.TempDir()
	bundle, err := UnpackFromLayers(bundleRoot, artifact)
	if err != nil {
		t.Fatalf("UnpackFromLayers failed: %v", err)
	}

	// The main weight file should be tracked as ggufFile.
	if bundle.ggufFile != "gemma-4-E2B-it-UD-Q4_K_XL.gguf" {
		t.Errorf("ggufFile = %q, want %q", bundle.ggufFile, "gemma-4-E2B-it-UD-Q4_K_XL.gguf")
	}
	if _, err := os.Stat(bundle.GGUFPath()); err != nil {
		t.Fatalf("Expected GGUF file to exist at %s, got: %v", bundle.GGUFPath(), err)
	}

	// The mmproj file should be tracked as mmprojPath.
	if bundle.mmprojPath != "mmproj-BF16.gguf" {
		t.Errorf("mmprojPath = %q, want %q", bundle.mmprojPath, "mmproj-BF16.gguf")
	}
	if bundle.MMPROJPath() == "" {
		t.Fatal("Expected MMPROJPath() to return non-empty path")
	}
	if _, err := os.Stat(bundle.MMPROJPath()); err != nil {
		t.Fatalf("Expected mmproj file to exist at %s, got: %v", bundle.MMPROJPath(), err)
	}
}

func TestValidatePathWithinDirectory_RealFilesystem(t *testing.T) {
	// Create a temporary directory structure
	baseDir := t.TempDir()

	// Create a sibling directory that attacker might try to access
	siblingDir := filepath.Join(filepath.Dir(baseDir), "sibling-secret")
	if err := os.MkdirAll(siblingDir, 0755); err != nil {
		t.Fatalf("Failed to create sibling dir: %v", err)
	}
	defer os.RemoveAll(siblingDir)

	// Create a secret file in the sibling directory
	secretFile := filepath.Join(siblingDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret data"), 0644); err != nil {
		t.Fatalf("Failed to create secret file: %v", err)
	}

	// Try to escape to the sibling directory
	escapePath := "../sibling-secret/secret.txt"
	err := validatePathWithinDirectory(baseDir, escapePath)
	if err == nil {
		t.Errorf("Expected error when attempting to escape to sibling directory, but validation passed")
	}
}
