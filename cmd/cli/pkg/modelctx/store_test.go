package modelctx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleConfig returns a ContextConfig suitable for use in tests.
func sampleConfig(host string) ContextConfig {
	return ContextConfig{
		Host:        host,
		Description: "test context",
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
}

// newTestStore creates a Store backed by a temporary directory.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(t.TempDir())
	require.NoError(t, err)
	return store
}

// TestNew verifies that New creates the storage directory and that opening an
// existing store on the same path succeeds.
func TestNew(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	require.NoError(t, err)
	require.NotNil(t, store)

	// Re-opening the same path should also succeed.
	store2, err := New(dir)
	require.NoError(t, err)
	require.NotNil(t, store2)
}

// TestCreate verifies that a newly created context can be retrieved via Get.
func TestCreate(t *testing.T) {
	store := newTestStore(t)
	cfg := sampleConfig("http://localhost:12434")
	require.NoError(t, store.Create("myctx", cfg))

	got, err := store.Get("myctx")
	require.NoError(t, err)
	assert.Equal(t, cfg.Host, got.Host)
	assert.Equal(t, cfg.Description, got.Description)
}

// TestCreate_reservedName verifies that "default" cannot be used as a context
// name.
func TestCreate_reservedName(t *testing.T) {
	store := newTestStore(t)
	err := store.Create(DefaultContextName, sampleConfig("http://localhost:12434"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved")
}

// TestCreate_invalidNames verifies that names violating the naming rules are
// rejected.
func TestCreate_invalidNames(t *testing.T) {
	store := newTestStore(t)
	cfg := sampleConfig("http://localhost:12434")
	for _, name := range []string{"", "-leading-dash", "has space", "has/slash"} {
		err := store.Create(name, cfg)
		require.Errorf(t, err, "expected error for name %q", name)
	}
}

// TestCreate_duplicate verifies that creating a context with an already-used
// name returns an error.
func TestCreate_duplicate(t *testing.T) {
	store := newTestStore(t)
	cfg := sampleConfig("http://localhost:12434")
	require.NoError(t, store.Create("myctx", cfg))

	err := store.Create("myctx", sampleConfig("http://other:12434"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestGet_notFound verifies that Get returns an error for unknown names.
func TestGet_notFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Get("nosuchctx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestList verifies that List returns an empty map when no contexts exist and
// returns all created contexts afterwards.
func TestList(t *testing.T) {
	store := newTestStore(t)

	// Empty store.
	contexts, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, contexts)

	// Add two contexts.
	require.NoError(t, store.Create("alpha", sampleConfig("http://alpha:12434")))
	require.NoError(t, store.Create("beta", sampleConfig("http://beta:12434")))

	contexts, err = store.List()
	require.NoError(t, err)
	assert.Len(t, contexts, 2)
	assert.Contains(t, contexts, "alpha")
	assert.Contains(t, contexts, "beta")
}

// TestRemove verifies that a context is gone after removal.
func TestRemove(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("myctx", sampleConfig("http://localhost:12434")))

	require.NoError(t, store.Remove("myctx"))

	_, err := store.Get("myctx")
	require.Error(t, err)
}

// TestRemove_default verifies that the reserved "default" name cannot be
// removed.
func TestRemove_default(t *testing.T) {
	store := newTestStore(t)
	err := store.Remove(DefaultContextName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved")
}

// TestRemove_notFound verifies that removing an unknown context returns an
// error.
func TestRemove_notFound(t *testing.T) {
	store := newTestStore(t)
	err := store.Remove("nosuchctx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestRemove_activeContext verifies that the currently active context cannot
// be removed and that it remains in the store after the attempt.
func TestRemove_activeContext(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("myctx", sampleConfig("http://localhost:12434")))
	require.NoError(t, store.SetActive("myctx"))

	err := store.Remove("myctx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "currently active")

	// Context must still be present.
	_, err = store.Get("myctx")
	require.NoError(t, err)
}

// TestActive_default verifies that Active returns DefaultContextName when no
// context file exists.
func TestActive_default(t *testing.T) {
	store := newTestStore(t)
	active, err := store.Active()
	require.NoError(t, err)
	assert.Equal(t, DefaultContextName, active)
}

// TestSetActive verifies that SetActive changes the value returned by Active.
func TestSetActive(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("myctx", sampleConfig("http://localhost:12434")))

	require.NoError(t, store.SetActive("myctx"))

	active, err := store.Active()
	require.NoError(t, err)
	assert.Equal(t, "myctx", active)
}

// TestSetActive_backToDefault verifies that SetActive("default") resets the
// active context to the auto-detect sentinel.
func TestSetActive_backToDefault(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("myctx", sampleConfig("http://localhost:12434")))
	require.NoError(t, store.SetActive("myctx"))

	require.NoError(t, store.SetActive(DefaultContextName))

	active, err := store.Active()
	require.NoError(t, err)
	assert.Equal(t, DefaultContextName, active)
}

// TestSetActive_notFound verifies that SetActive returns an error when the
// named context does not exist.
func TestSetActive_notFound(t *testing.T) {
	store := newTestStore(t)
	err := store.SetActive("nosuchctx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestSetActive_invalidName verifies that SetActive rejects invalid names.
func TestSetActive_invalidName(t *testing.T) {
	store := newTestStore(t)
	err := store.SetActive("has space")
	require.Error(t, err)
}

// TestPersistence verifies that context data written by one Store instance is
// readable by a new instance opened on the same directory.
func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	cfg := sampleConfig("http://remote:12434")

	// Write with the first instance.
	s1, err := New(dir)
	require.NoError(t, err)
	require.NoError(t, s1.Create("remote", cfg))
	require.NoError(t, s1.SetActive("remote"))

	// Read back with a new instance.
	s2, err := New(dir)
	require.NoError(t, err)

	active, err := s2.Active()
	require.NoError(t, err)
	assert.Equal(t, "remote", active)

	got, err := s2.Get("remote")
	require.NoError(t, err)
	assert.Equal(t, cfg.Host, got.Host)
	assert.Equal(t, cfg.Description, got.Description)
}

// TestTLSConfig verifies that TLS settings are stored and retrieved correctly.
func TestTLSConfig(t *testing.T) {
	store := newTestStore(t)
	cfg := ContextConfig{
		Host: "https://secure:12444",
		TLS: TLSConfig{
			Enabled:    true,
			SkipVerify: false,
			CACert:     "/etc/ssl/certs/ca.pem",
		},
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, store.Create("secure", cfg))

	got, err := store.Get("secure")
	require.NoError(t, err)
	assert.True(t, got.TLS.Enabled)
	assert.False(t, got.TLS.SkipVerify)
	assert.Equal(t, "/etc/ssl/certs/ca.pem", got.TLS.CACert)
}
