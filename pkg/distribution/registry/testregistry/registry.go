// Package testregistry provides a simple in-memory OCI registry for testing.
package testregistry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/opencontainers/go-digest"
)

// ociError represents an OCI registry error.
type ociError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ociErrorResponse represents an OCI registry error response.
type ociErrorResponse struct {
	Errors []ociError `json:"errors"`
}

// Registry is an in-memory OCI distribution registry for testing.
type Registry struct {
	mu        sync.RWMutex
	blobs     map[string][]byte            // digest -> content
	manifests map[string]map[string][]byte // repo -> tag/digest -> manifest
}

// New creates a new test registry handler.
func New() http.Handler {
	r := &Registry{
		blobs:     make(map[string][]byte),
		manifests: make(map[string]map[string][]byte),
	}
	return r
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/v2/")

	// Handle /v2/ base endpoint
	if path == "" || path == "/" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Route requests
	switch {
	case strings.Contains(path, "/blobs/uploads/"):
		r.handleBlobUpload(w, req, path)
	case strings.Contains(path, "/blobs/"):
		r.handleBlob(w, req, path)
	case strings.Contains(path, "/manifests/"):
		r.handleManifest(w, req, path)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (r *Registry) handleBlobUpload(w http.ResponseWriter, req *http.Request, path string) {
	// Parse repo from path
	parts := strings.SplitN(path, "/blobs/uploads/", 2)
	repo := parts[0]

	switch req.Method {
	case http.MethodPost:
		// Start upload
		uploadID := fmt.Sprintf("upload-%d", len(r.blobs))
		location := fmt.Sprintf("/v2/%s/blobs/uploads/%s", repo, uploadID)
		w.Header().Set("Location", location)
		w.Header().Set("Docker-Upload-UUID", uploadID)
		w.WriteHeader(http.StatusAccepted)

	case http.MethodPut:
		// Complete upload
		dgst := req.URL.Query().Get("digest")
		if dgst == "" {
			http.Error(w, "digest required", http.StatusBadRequest)
			return
		}

		content, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		r.mu.Lock()
		r.blobs[dgst] = content
		r.mu.Unlock()

		w.Header().Set("Docker-Content-Digest", dgst)
		w.WriteHeader(http.StatusCreated)

	case http.MethodPatch:
		// Chunked upload - accumulate data
		content, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// For simplicity, compute digest and store directly
		dgst := digest.FromBytes(content)
		r.mu.Lock()
		r.blobs[dgst.String()] = content
		r.mu.Unlock()

		location := req.URL.Path
		w.Header().Set("Location", location)
		w.Header().Set("Range", fmt.Sprintf("0-%d", len(content)-1))
		w.WriteHeader(http.StatusAccepted)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (r *Registry) handleBlob(w http.ResponseWriter, req *http.Request, path string) {
	// Parse digest from path
	parts := strings.SplitN(path, "/blobs/", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	dgst := parts[1]

	switch req.Method {
	case http.MethodHead:
		r.mu.RLock()
		content, ok := r.blobs[dgst]
		r.mu.RUnlock()

		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Header().Set("Docker-Content-Digest", dgst)
		w.WriteHeader(http.StatusOK)

	case http.MethodGet:
		r.mu.RLock()
		content, ok := r.blobs[dgst]
		r.mu.RUnlock()

		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Check for Range header for resumable downloads
		rangeHeader := req.Header.Get("Range")
		if rangeHeader != "" {
			// Parse Range header (format: "bytes=start-" or "bytes=start-end")
			var start, end int64
			end = int64(len(content) - 1)
			n, err := fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
			if err != nil || n != 1 {
				// Try parsing with end
				n, err = fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
				if err != nil || n < 1 {
					http.Error(w, "invalid range", http.StatusBadRequest)
					return
				}
			}

			if start >= int64(len(content)) {
				http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
				return
			}

			partialContent := content[start : end+1]
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(partialContent)))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
			w.Header().Set("Docker-Content-Digest", dgst)
			w.WriteHeader(http.StatusPartialContent)
			w.Write(partialContent)
			return
		}

		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Header().Set("Docker-Content-Digest", dgst)
		w.WriteHeader(http.StatusOK)
		w.Write(content)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (r *Registry) handleManifest(w http.ResponseWriter, req *http.Request, path string) {
	// Parse repo and reference from path
	parts := strings.SplitN(path, "/manifests/", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	repo := parts[0]
	ref := parts[1]

	switch req.Method {
	case http.MethodHead, http.MethodGet:
		r.mu.RLock()
		repoManifests, ok := r.manifests[repo]
		if !ok {
			r.mu.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			//nolint:errchkjson // test registry, ignore write errors
			_ = json.NewEncoder(w).Encode(ociErrorResponse{
				Errors: []ociError{{Code: "NAME_UNKNOWN", Message: "Repository not found"}},
			})
			return
		}

		manifest, ok := repoManifests[ref]
		r.mu.RUnlock()

		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			//nolint:errchkjson // test registry, ignore write errors
			_ = json.NewEncoder(w).Encode(ociErrorResponse{
				Errors: []ociError{{Code: "MANIFEST_UNKNOWN", Message: "Manifest not found"}},
			})
			return
		}

		dgst := digest.FromBytes(manifest)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(manifest)))
		w.Header().Set("Docker-Content-Digest", dgst.String())
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")

		if req.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			w.Write(manifest)
		} else {
			w.WriteHeader(http.StatusOK)
		}

	case http.MethodPut:
		content, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		dgst := digest.FromBytes(content)

		r.mu.Lock()
		if r.manifests[repo] == nil {
			r.manifests[repo] = make(map[string][]byte)
		}
		r.manifests[repo][ref] = content
		// Also store by digest for digest-based lookups
		r.manifests[repo][dgst.String()] = content
		r.mu.Unlock()

		w.Header().Set("Docker-Content-Digest", dgst.String())
		w.WriteHeader(http.StatusCreated)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
