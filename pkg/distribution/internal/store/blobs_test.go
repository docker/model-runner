package store

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/oci"
)

func TestBlobs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "blob-test")
	if err != nil {
		t.Fatalf("error creating temp dir: %v", err)
	}
	rootDir := filepath.Join(tmpDir, "store")
	store, err := New(Options{RootPath: rootDir})
	if err != nil {
		t.Fatalf("error creating store: %v", err)
	}

	t.Run("WriteBlob with missing dir", func(t *testing.T) {
		// remove blobs directory to ensure it is recreated as needed
		if err := os.RemoveAll(store.blobsDir()); err != nil {
			t.Fatalf("expected blobs directory not be present")
		}

		// create the blob
		expectedContent := "some data"
		hash, _, err := oci.SHA256(bytes.NewBufferString(expectedContent))
		if err != nil {
			t.Fatalf("error calculating hash: %v", err)
		}

		// write the blob
		if err := store.WriteBlob(hash, bytes.NewBufferString(expectedContent)); err != nil {
			t.Fatalf("error writing blob: %v", err)
		}

		// ensure blob file exists
		blobPath, err := store.blobPath(hash)
		if err != nil {
			t.Fatalf("error getting blob path: %v", err)
		}
		content, err := os.ReadFile(blobPath)
		if err != nil {
			t.Fatalf("error reading blob file: %v", err)
		}

		// ensure correct content
		if string(content) != expectedContent {
			t.Fatalf("unexpected blob content: got %v expected %s", string(content), expectedContent)
		}

		// ensure incomplete blob file does not exist
		blobPath, err = store.blobPath(hash)
		if err != nil {
			t.Fatalf("error getting blob path: %v", err)
		}
		tmpFile := incompletePath(blobPath)
		if _, err := os.Stat(tmpFile); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected incomplete blob file %s not be present", tmpFile)
		}
	})

	t.Run("WriteBlob fails", func(t *testing.T) {
		// simulate lingering incomplete blob file (if program crashed)
		hash := oci.Hash{
			Algorithm: "sha256",
			Hex:       "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		}
		blobPath, err := store.blobPath(hash)
		if err != nil {
			t.Fatalf("error getting blob path: %v", err)
		}
		if err := writeFile(incompletePath(blobPath), []byte("incomplete")); err != nil {
			t.Fatalf("error creating incomplete blob file for test: %v", err)
		}

		if err := store.WriteBlob(hash, &errorReader{}); err == nil {
			t.Fatalf("expected error writing blob")
		}

		// ensure blob file does not exist
		blobPath2, err := store.blobPath(hash)
		if err != nil {
			t.Fatalf("error getting blob path: %v", err)
		}
		if _, err := os.ReadFile(blobPath2); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected blob file not to exist")
		}

		// ensure incomplete file is not left behind
		blobPath3, err := store.blobPath(hash)
		if err != nil {
			t.Fatalf("error getting blob path: %v", err)
		}
		if _, err := os.ReadFile(incompletePath(blobPath3)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected incomplete blob file not to exist")
		}
	})

	t.Run("WriteBlob reuses existing blob", func(t *testing.T) {
		// simulate existing blob
		hash := oci.Hash{
			Algorithm: "sha256",
			Hex:       "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		}

		if err := store.WriteBlob(hash, bytes.NewReader([]byte("some-data"))); err != nil {
			t.Fatalf("error writing blob: %v", err)
		}

		if err := store.WriteBlob(hash, &errorReader{}); err != nil {
			t.Fatalf("error writing blob: %v", err)
		}

		// ensure blob file exists
		blobPath4, err := store.blobPath(hash)
		if err != nil {
			t.Fatalf("error getting blob path: %v", err)
		}
		content, err := os.ReadFile(blobPath4)
		if err != nil {
			t.Fatalf("error reading blob file: %v", err)
		}

		// ensure correct content
		if string(content) != "some-data" {
			t.Fatalf("unexpected blob content: got %v expected %s", string(content), "some-data")
		}
	})
}

var _ io.Reader = &errorReader{}

type errorReader struct {
}

func (e errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("fake error")
}

func (e errorReader) Close() error {
	return nil
}
