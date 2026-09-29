package tarball_test

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/tarball"
)

func TestTarget(t *testing.T) {
	f, err := os.CreateTemp("", "tar-test")
	if err != nil {
		t.Fatalf("Failed to file for tar: %v", err)
	}
	path := f.Name()
	defer os.Remove(f.Name())
	defer f.Close()

	target, err := tarball.NewTarget(f)
	if err != nil {
		t.Fatalf("Failed to create tar target: %v", err)
	}

	b, err := builder.FromPath(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	mdl := b.Model()

	blobContents, err := os.ReadFile(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	blobHash, _, err := oci.SHA256(bytes.NewReader(blobContents))
	if err != nil {
		t.Fatalf("Failed to calculate hash: %v", err)
	}
	configDigest, err := mdl.ConfigName()
	if err != nil {
		t.Fatalf("Failed to get raw config: %v", err)
	}
	configContents, err := mdl.RawConfigFile()
	if err != nil {
		t.Fatalf("Failed to get raw config: %v", err)
	}
	manifestContents, err := mdl.RawManifest()
	if err != nil {
		t.Fatalf("Failed to get raw manifest contents: %v", err)
	}

	if err := target.Write(t.Context(), mdl, nil); err != nil {
		t.Fatalf("Failed to write model to tar file: %v", err)
	}

	tf, err := os.Open(path)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	tr := tar.NewReader(tf)
	hasDir(t, tr, "blobs")
	hasDir(t, tr, "blobs/sha256")
	hasFile(t, tr, "blobs/sha256/"+blobHash.Hex, blobContents)
	hasFile(t, tr, "blobs/sha256/"+configDigest.Hex, configContents)
	hasFile(t, tr, "manifest.json", manifestContents)
}

// TestTargetEntryNamesUseForwardSlashes verifies that all tar entry names
// produced by Target.Write use forward slashes, even for models with multiple
// layers (e.g., GGUF + chat template).
//
// This is a regression test for https://github.com/docker/model-runner/issues/894
// where filepath.Join on Windows produced backslash-separated entry names
// (e.g., "blobs\sha256\hex"), causing the daemon reader to skip the blobs.
func TestTargetEntryNamesUseForwardSlashes(t *testing.T) {
	b, err := builder.FromPath(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create builder from GGUF: %v", err)
	}
	b, err = b.WithChatTemplateFile(filepath.Join("..", "assets", "template.jinja"))
	if err != nil {
		t.Fatalf("Failed to add chat template: %v", err)
	}

	var buf bytes.Buffer
	target, err := tarball.NewTarget(&buf)
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}
	if err := target.Write(t.Context(), b.Model(), nil); err != nil {
		t.Fatalf("Failed to write model: %v", err)
	}

	// Read all tar entries and verify none contain backslashes.
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read tar entry: %v", err)
		}
		if strings.Contains(hdr.Name, "\\") {
			t.Errorf("Tar entry name contains backslash: %q — "+
				"tar entry names must use forward slashes for cross-platform compatibility "+
				"(see https://github.com/docker/model-runner/issues/894)", hdr.Name)
		}
	}
}

func hasFile(t *testing.T, tr *tar.Reader, name string, contents []byte) {
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("Failed to read header: %v", err)
	}
	if hdr.Name != name {
		t.Fatalf("Unexpected next entry with name %q got %q", name, hdr.Name)
	}
	if hdr.Typeflag != tar.TypeReg {
		t.Fatalf("Unexpected entry with name %q to be a file got type %v", name, hdr.Typeflag)
	}
	if hdr.Size != int64(len(contents)) {
		t.Fatalf("Unexpected entry with name %q size %d got %d", name, hdr.Size, hdr.Size)
	}
	c, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("Failed to read contents: %v", err)
	}
	if !bytes.Equal(contents, c) {
		t.Fatalf("Unexpected contents for file %q", name)
	}
}

func hasDir(t *testing.T, tr *tar.Reader, name string) {
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("Failed to read header: %v", err)
	}
	if hdr.Name != name {
		t.Fatalf("Unexpected next entry with name %q got %q", name, hdr.Name)
	}
	if hdr.Typeflag != tar.TypeDir {
		t.Fatalf("Unexpected entry with name %q to be a directory got type %v", name, hdr.Typeflag)
	}
}
