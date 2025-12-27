package tarball_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/tarball"
)

func TestStream(t *testing.T) {
	// archive.tar manually generated from testdata/archive
	f, err := os.Open(filepath.Join("testdata", "archive.tar"))
	if err != nil {
		t.Fatalf("Failed to open archive: %v", err)
	}
	defer f.Close()
	r := tarball.NewReader(f)

	// Read blobs
	assertNextBlob(t, r, oci.Hash{
		Algorithm: "sha256",
		Hex:       "bec7cb2222b54879bf3c7e70504960bdfbd898a05ab1f8247808484869a46bad",
	}, "some-blob-contents")
	assertNextBlob(t, r, oci.Hash{
		Algorithm: "sha512",
		Hex:       "d302a5a946106425f12177a93f87c1b7d4ee8ad851937a6a59dc6e0b758fbed5ab10a116509f73165e2b29b40e870f8c28a6a4f6c1ebfe9fa7d295ba7ff151c9",
	}, "other-blob-contents")
	if _, err = r.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("Should have gotten EOF")
	}

	// Read manifest
	rawManifest, digest, _ := r.Manifest()
	if string(rawManifest) != "some-manifest-contents" {
		t.Errorf("Unexpected manifest contents: got %q expected %q", string(rawManifest), "some-manifest-contents")
	}
	if digest.Algorithm != "sha256" {
		t.Errorf("Unexpected digest algorithm: %s", digest.Algorithm)
	}
	if digest.Hex != "a069ed344ddcd0ce7091471826d225dd080ccb53a4483c3d0364c16c63508955" {
		t.Errorf("Unexpected digest: %s", digest.Algorithm)
	}
}

func assertNextBlob(t *testing.T, r *tarball.Reader, expectedDiffID oci.Hash, expectedContents string) {
	diffID, err := r.Next()
	if err != nil {
		t.Fatalf("Failed to read blob: %v", err)
	}
	if diffID.Algorithm != expectedDiffID.Algorithm {
		t.Fatalf("Expected diffID with alg %q but got %q", expectedDiffID.Algorithm, diffID.Algorithm)
	}
	if diffID.Hex != expectedDiffID.Hex {
		t.Fatalf("Expected diffID with hex %q but got %q", expectedDiffID.Hex, diffID.Hex)
	}
	contents, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("Failed to read blob: %v", err)
	}
	if string(contents) != expectedContents {
		t.Fatalf("Expected blob contents %q but got %q", expectedContents, string(contents))
	}
}
