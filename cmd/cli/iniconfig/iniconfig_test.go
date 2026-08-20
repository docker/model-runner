package iniconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-runner/cmd/cli/iniconfig"
)

// roundTrip writes entries to a temp file, reads them back, and checks they
// match the expected key/value pairs.
func roundTrip(t *testing.T, content string, wantEntries []iniconfig.Entry) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := iniconfig.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(f.Entries()) != len(wantEntries) {
		t.Fatalf("got %d entries, want %d\nentries: %v", len(f.Entries()), len(wantEntries), f.Entries())
	}
	for i, e := range f.Entries() {
		if e.Key != wantEntries[i].Key || e.Value != wantEntries[i].Value {
			t.Errorf("entry[%d]: got {%q %q}, want {%q %q}", i, e.Key, e.Value, wantEntries[i].Key, wantEntries[i].Value)
		}
	}
}

func TestParse_SimpleSection(t *testing.T) {
	roundTrip(t, `
[core]
	bare = false
	filemode = true
`, []iniconfig.Entry{
		{Key: "core.bare", Value: "false"},
		{Key: "core.filemode", Value: "true"},
	})
}

func TestParse_Subsection(t *testing.T) {
	roundTrip(t, `
[branch "main"]
	remote = origin
	merge = refs/heads/main
`, []iniconfig.Entry{
		{Key: "branch.main.remote", Value: "origin"},
		{Key: "branch.main.merge", Value: "refs/heads/main"},
	})
}

func TestParse_CaseInsensitiveSection(t *testing.T) {
	roundTrip(t, `
[Core]
	Bare = false
`, []iniconfig.Entry{
		{Key: "core.bare", Value: "false"},
	})
}

func TestParse_SubsectionCaseSensitive(t *testing.T) {
	roundTrip(t, `
[branch "Main"]
	remote = origin
[branch "main"]
	remote = upstream
`, []iniconfig.Entry{
		{Key: "branch.Main.remote", Value: "origin"},
		{Key: "branch.main.remote", Value: "upstream"},
	})
}

func TestParse_BooleanKey(t *testing.T) {
	roundTrip(t, `
[core]
	bare
`, []iniconfig.Entry{
		{Key: "core.bare", Value: "true"},
	})
}

func TestParse_InlineComment(t *testing.T) {
	roundTrip(t, `
[core]
	name = hello # world
`, []iniconfig.Entry{
		{Key: "core.name", Value: "hello"},
	})
}

func TestParse_QuotedValue(t *testing.T) {
	roundTrip(t, `
[core]
	name = "hello world"
`, []iniconfig.Entry{
		{Key: "core.name", Value: "hello world"},
	})
}

func TestParse_EscapeSequences(t *testing.T) {
	roundTrip(t, `
[core]
	name = "hello\nworld"
`, []iniconfig.Entry{
		{Key: "core.name", Value: "hello\nworld"},
	})
}

func TestParse_BOM(t *testing.T) {
	content := "\xEF\xBB\xBF[core]\n\tbare = false\n"
	roundTrip(t, content, []iniconfig.Entry{
		{Key: "core.bare", Value: "false"},
	})
}

func TestParse_Comments(t *testing.T) {
	roundTrip(t, `
# This is a comment
; This is also a comment
[core]
	# inline section comment
	bare = false
`, []iniconfig.Entry{
		{Key: "core.bare", Value: "false"},
	})
}

func TestParse_SectionHeaderTrailingComment(t *testing.T) {
	roundTrip(t, `
[core] # this is a trailing comment
	bare = false
`, []iniconfig.Entry{
		{Key: "core.bare", Value: "false"},
	})
}

func TestParse_FilePermissionsPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	// Create with restrictive permissions.
	if err := os.WriteFile(path, []byte("[core]\n\tbare = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := iniconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Set("core.filemode", "true"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != 0o600 {
		t.Errorf("expected mode 0600, got %v", info.Mode())
	}
}

func TestLoadMissing(t *testing.T) {
	f, err := iniconfig.Load("/nonexistent/path/to/config")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(f.Entries()) != 0 {
		t.Fatalf("expected empty entries for missing file, got: %v", f.Entries())
	}
}

func TestGetAndSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	f, err := iniconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Set("core.bare", "false"); err != nil {
		t.Fatal(err)
	}
	if v, ok := f.Get("core.bare"); !ok || v != "false" {
		t.Fatalf("Get after Set: got %q, %v; want %q, true", v, ok, "false")
	}

	// Overwrite
	if err := f.Set("core.bare", "true"); err != nil {
		t.Fatal(err)
	}
	if v, ok := f.Get("core.bare"); !ok || v != "true" {
		t.Fatalf("Get after overwrite: got %q, %v; want %q, true", v, ok, "true")
	}
}

func TestSetSubsection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	f, _ := iniconfig.Load(path)

	if err := f.Set(`branch.main.remote`, "origin"); err != nil {
		t.Fatal(err)
	}
	if v, ok := f.Get("branch.main.remote"); !ok || v != "origin" {
		t.Fatalf("got %q, %v", v, ok)
	}
}

func TestUnset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	f, _ := iniconfig.Load(path)
	_ = f.Set("core.bare", "false")
	_ = f.Set("core.filemode", "true")

	if err := f.Unset("core.bare"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Get("core.bare"); ok {
		t.Fatal("expected core.bare to be removed")
	}
	if v, ok := f.Get("core.filemode"); !ok || v != "true" {
		t.Fatalf("core.filemode should still be present, got %q, %v", v, ok)
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	f, _ := iniconfig.Load(path)
	_ = f.Set("user.name", "Alice")

	// Reload from disk and verify.
	f2, err := iniconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := f2.Get("user.name"); !ok || v != "Alice" {
		t.Fatalf("reload: got %q, %v", v, ok)
	}
}

func TestParseKey(t *testing.T) {
	tests := []struct {
		input      string
		section    string
		subsection string
		variable   string
		wantErr    bool
	}{
		{"core.bare", "core", "", "bare", false},
		{"branch.main.remote", "branch", "main", "remote", false},
		{"url.https://example.com/.insteadof", "url", "https://example.com/", "insteadof", false},
		{"nokey", "", "", "", true},
		{"section.", "", "", "", true},
	}
	for _, tt := range tests {
		sec, sub, vari, err := iniconfig.ParseKey(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseKey(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && (sec != tt.section || sub != tt.subsection || vari != tt.variable) {
			t.Errorf("ParseKey(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.input, sec, sub, vari, tt.section, tt.subsection, tt.variable)
		}
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	f, _ := iniconfig.Load(path)
	_ = f.Set("user.name", "Alice")
	_ = f.Set("user.email", "alice@example.com")

	var sb strings.Builder
	if err := f.List(&sb); err != nil {
		t.Fatal(err)
	}
	got := sb.String()
	if !strings.Contains(got, "user.name=Alice\n") {
		t.Errorf("missing user.name in list output:\n%s", got)
	}
	if !strings.Contains(got, "user.email=alice@example.com\n") {
		t.Errorf("missing user.email in list output:\n%s", got)
	}
}

func TestSerialiseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	f, _ := iniconfig.Load(path)
	_ = f.Set("core.bare", "false")
	_ = f.Set("core.filemode", "true")
	_ = f.Set("branch.main.remote", "origin")

	// Reload and verify structure is preserved.
	f2, err := iniconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	entries := f2.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(entries), entries)
	}
}

func TestQuotedValueWithSpecialChars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	f, _ := iniconfig.Load(path)
	_ = f.Set("url.value", "value with # hash")

	f2, _ := iniconfig.Load(path)
	if v, ok := f2.Get("url.value"); !ok || v != "value with # hash" {
		t.Fatalf("got %q, %v", v, ok)
	}
}
