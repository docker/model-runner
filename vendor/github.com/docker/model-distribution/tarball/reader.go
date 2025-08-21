package tarball

import (
	"archive/tar"
	"encoding/hex"
	"errors"
	"io"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type Reader struct {
	tr          *tar.Reader
	rawManifest []byte
	digest      v1.Hash
	done        bool
}

type Blob struct {
	diffID v1.Hash
	rc     io.ReadCloser
}

func (b Blob) DiffID() (v1.Hash, error) {
	return b.diffID, nil
}

func (b Blob) Uncompressed() (io.ReadCloser, error) {
	return b.rc, nil
}

func (r *Reader) Next() (v1.Hash, error) {
	for {
		hdr, err := r.tr.Next()
		if err != nil {
			if err == io.EOF {
				r.done = true
			}
			return v1.Hash{}, err
		}
		//fi := hdr.FileInfo()
		if !(hdr.Typeflag == tar.TypeReg) {
			continue
		}
		if hdr.Name == "manifest.json" {
			// save the manifest
			hasher, err := v1.Hasher("sha256")
			if err != nil {
				return v1.Hash{}, err
			}
			rm, err := io.ReadAll(io.TeeReader(r.tr, hasher))
			if err != nil {
				return v1.Hash{}, err
			}
			r.rawManifest = rm
			r.digest = v1.Hash{
				Algorithm: "sha256",
				Hex:       hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size()))),
			}
			continue
		}
		parts := strings.Split(filepath.Clean(hdr.Name), "/")
		if len(parts) != 3 || parts[0] != "blobs" && parts[0] != "manifests" {
			continue
		}
		return v1.Hash{
			Algorithm: parts[1],
			Hex:       parts[2],
		}, nil
	}
}

func (r *Reader) Read(p []byte) (n int, err error) {
	return r.tr.Read(p)
}

func (r *Reader) Manifest() ([]byte, v1.Hash, error) {
	if !r.done {
		return nil, v1.Hash{}, errors.New("must read all blobs first before getting manifest")
	}
	if r.done && r.rawManifest == nil {
		return nil, v1.Hash{}, errors.New("manifest not found")
	}
	return r.rawManifest, r.digest, nil
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		tr: tar.NewReader(r),
	}
}
