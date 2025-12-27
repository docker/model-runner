package tarball

import (
	"archive/tar"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/oci"
)

type Reader struct {
	tr          *tar.Reader
	rawManifest []byte
	digest      oci.Hash
	done        bool
}

type Blob struct {
	diffID oci.Hash
	rc     io.ReadCloser
}

func (b Blob) DiffID() (oci.Hash, error) {
	return b.diffID, nil
}

func (b Blob) Uncompressed() (io.ReadCloser, error) {
	return b.rc, nil
}

func (r *Reader) Next() (oci.Hash, error) {
	for {
		hdr, err := r.tr.Next()
		if err != nil {
			if err == io.EOF {
				r.done = true
			}
			return oci.Hash{}, err
		}
		// fi := hdr.FileInfo()
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if hdr.Name == "manifest.json" {
			// save the manifest
			hasher, err := oci.Hasher("sha256")
			if err != nil {
				return oci.Hash{}, err
			}
			rm, err := io.ReadAll(io.TeeReader(r.tr, hasher))
			if err != nil {
				return oci.Hash{}, err
			}
			r.rawManifest = rm
			r.digest = oci.Hash{
				Algorithm: "sha256",
				Hex:       hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size()))),
			}
			continue
		}
		cleanPath := filepath.Clean(hdr.Name)
		if strings.Contains(cleanPath, "..") {
			return oci.Hash{}, fmt.Errorf("invalid path detected: %s", hdr.Name)
		}
		parts := strings.Split(cleanPath, "/")
		if len(parts) != 3 || parts[0] != "blobs" && parts[0] != "manifests" {
			continue
		}
		return oci.Hash{
			Algorithm: parts[1],
			Hex:       parts[2],
		}, nil
	}
}

func (r *Reader) Read(p []byte) (n int, err error) {
	return r.tr.Read(p)
}

func (r *Reader) Manifest() ([]byte, oci.Hash, error) {
	if !r.done {
		return nil, oci.Hash{}, errors.New("must read all blobs first before getting manifest")
	}
	if r.done && r.rawManifest == nil {
		return nil, oci.Hash{}, errors.New("manifest not found")
	}
	return r.rawManifest, r.digest, nil
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		tr: tar.NewReader(r),
	}
}
