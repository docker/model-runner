package tarball_test

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/internal/gguf"
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

	mdl, err := gguf.NewModel(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

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
