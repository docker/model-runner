// Package modelctx provides persistent storage for named Docker Model Runner
// contexts, allowing users to switch between different Model Runner backends
// without setting environment variables each time.
package modelctx

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// DefaultContextName is the reserved name for the auto-detected context.
// It is never written to disk; a missing or empty "current" value implies it.
const DefaultContextName = "default"

// contextFileVersion is the version of the on-disk context file format.
const contextFileVersion = 1

// validContextName matches names that follow Docker's context naming rules.
var validContextName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// TLSConfig holds optional TLS settings for a named context.
type TLSConfig struct {
	// Enabled indicates whether TLS is used for this context.
	Enabled bool `json:"enabled"`
	// SkipVerify disables TLS server certificate verification.
	SkipVerify bool `json:"skipVerify,omitempty"`
	// CACert is the absolute path to a custom CA certificate PEM file.
	CACert string `json:"caCert,omitempty"`
}

// ContextConfig is the configuration for a named Model Runner context.
type ContextConfig struct {
	// Host is the Model Runner API base URL (e.g. "http://192.168.1.100:12434").
	Host string `json:"host"`
	// TLS holds optional TLS settings.
	TLS TLSConfig `json:"tls,omitempty"`
	// Description is an optional human-readable note.
	Description string `json:"description,omitempty"`
	// CreatedAt records when the context was created.
	CreatedAt time.Time `json:"createdAt"`
}

// contextFile is the versioned on-disk representation of the context store.
type contextFile struct {
	// Version is the schema version; currently always 1.
	Version int `json:"version"`
	// Current is the active context name; empty means DefaultContextName.
	Current string `json:"current,omitempty"`
	// Contexts is a map from context name to its configuration.
	Contexts map[string]ContextConfig `json:"contexts"`
}

// Store manages named Model Runner contexts stored in a single JSON file.
type Store struct {
	// path is the absolute path to the contexts.json file.
	path string
}

// New returns a Store that persists contexts in
// <dockerConfigDir>/model/contexts.json. It creates the parent directory if
// it does not exist.
func New(dockerConfigDir string) (*Store, error) {
	dir := filepath.Join(dockerConfigDir, "model")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("unable to create model context directory: %w", err)
	}
	return &Store{path: filepath.Join(dir, "contexts.json")}, nil
}

// ValidateName returns an error if name is reserved or does not match the
// allowed pattern.
func ValidateName(name string) error {
	if name == DefaultContextName {
		return fmt.Errorf("context name %q is reserved", name)
	}
	if !validContextName.MatchString(name) {
		return fmt.Errorf(
			"invalid context name %q: must match %s",
			name, validContextName,
		)
	}
	return nil
}

// List returns all named contexts. The synthetic "default" context is not
// included.
func (s *Store) List() (map[string]ContextConfig, error) {
	cf, err := s.read()
	if err != nil {
		return nil, err
	}
	return cf.Contexts, nil
}

// Get returns the configuration for the named context.
func (s *Store) Get(name string) (ContextConfig, error) {
	cf, err := s.read()
	if err != nil {
		return ContextConfig{}, err
	}
	cfg, ok := cf.Contexts[name]
	if !ok {
		return ContextConfig{}, fmt.Errorf("context %q not found", name)
	}
	return cfg, nil
}

// Create writes a new named context. It returns an error if the name is
// reserved, fails validation, or already exists.
func (s *Store) Create(name string, cfg ContextConfig) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	return s.update(func(cf *contextFile) error {
		if _, exists := cf.Contexts[name]; exists {
			return fmt.Errorf("context %q already exists", name)
		}
		cf.Contexts[name] = cfg
		return nil
	})
}

// Remove deletes the named context. It returns an error if name is
// DefaultContextName or if the context is currently active.
func (s *Store) Remove(name string) error {
	if name == DefaultContextName {
		return fmt.Errorf("context name %q is reserved and cannot be removed", name)
	}
	return s.update(func(cf *contextFile) error {
		if _, exists := cf.Contexts[name]; !exists {
			return fmt.Errorf("context %q not found", name)
		}
		if cf.Current == name {
			return fmt.Errorf(
				"context %q is currently active; switch to another context first",
				name,
			)
		}
		delete(cf.Contexts, name)
		return nil
	})
}

// Active returns the name of the currently active context, or
// DefaultContextName if none has been set.
func (s *Store) Active() (string, error) {
	cf, err := s.read()
	if err != nil {
		return "", err
	}
	if cf.Current == "" {
		return DefaultContextName, nil
	}
	return cf.Current, nil
}

// SetActive makes the named context active. Pass DefaultContextName to revert
// to auto-detection. The named context must already exist unless name is
// DefaultContextName.
func (s *Store) SetActive(name string) error {
	if name != DefaultContextName {
		if err := ValidateName(name); err != nil {
			return err
		}
	}
	return s.update(func(cf *contextFile) error {
		if name != DefaultContextName {
			if _, exists := cf.Contexts[name]; !exists {
				return fmt.Errorf("context %q not found", name)
			}
		}
		// Store DefaultContextName as an empty string so the JSON omits the
		// field, keeping the file clean.
		if name == DefaultContextName {
			cf.Current = ""
		} else {
			cf.Current = name
		}
		return nil
	})
}

// read loads the context file from disk. A missing file is treated as an
// empty store rather than an error.
func (s *Store) read() (contextFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return contextFile{
				Version:  contextFileVersion,
				Contexts: make(map[string]ContextConfig),
			}, nil
		}
		return contextFile{}, fmt.Errorf("unable to read context file: %w", err)
	}
	var cf contextFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return contextFile{}, fmt.Errorf("unable to parse context file: %w", err)
	}
	if cf.Contexts == nil {
		cf.Contexts = make(map[string]ContextConfig)
	}
	return cf, nil
}

// update applies a mutation function under a file lock and writes the result
// atomically. This serialises concurrent writers while allowing readers to
// always see a complete file.
func (s *Store) update(mutate func(*contextFile) error) error {
	lockPath := filepath.Join(filepath.Dir(s.path), "contexts.lock")

	// Open (or create) the lock file, then acquire an exclusive advisory
	// lock via flock(2) (Unix) or LockFileEx (Windows). The OS-level lock
	// prevents concurrent processes from entering this critical section at
	// the same time. The lock is released on close.
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("unable to open context lock file: %w", err)
	}
	defer func() {
		_ = unlockFile(lf)
		_ = lf.Close()
	}()

	if err := lockFile(lf); err != nil {
		return fmt.Errorf("unable to acquire context lock: %w", err)
	}

	// Re-read under lock to pick up any changes made since the caller last read.
	cf, err := s.read()
	if err != nil {
		return err
	}

	// Apply the mutation.
	if err := mutate(&cf); err != nil {
		return err
	}

	// Serialise the updated state.
	data, err := json.MarshalIndent(cf, "", "    ")
	if err != nil {
		return fmt.Errorf("unable to serialise context file: %w", err)
	}
	data = append(data, '\n')

	// Write to a uniquely named temp file then rename atomically.
	var rndBuf [8]byte
	_, _ = rand.Read(rndBuf[:])
	tmpPath := fmt.Sprintf(
		"%s.tmp.%d.%x",
		s.path, os.Getpid(), rndBuf,
	)
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("unable to write context file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("unable to write context file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("unable to sync context file: %w", err)
	}
	_ = f.Close()

	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("unable to commit context file: %w", err)
	}
	return nil
}
