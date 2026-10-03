package iniconfig_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	if !slices.Equal(f.Entries(), wantEntries) {
		t.Fatalf("got %q, want %q", f.Entries(), wantEntries)
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

func TestParse_SectionHeaderSuffix(t *testing.T) {
	for _, hdr := range []string{"[core]", "[core]  ", "[core]# c", "[core];c", "[core] \t# c"} {
		roundTrip(t, hdr+"\n\tbare = false\n", []iniconfig.Entry{
			{Key: "core.bare", Value: "false"},
		})
	}
	roundTrip(t, "[core \"x\"] ; c\n\tbare = false\n", []iniconfig.Entry{
		{Key: "core.x.bare", Value: "false"},
	})
	for _, hdr := range []string{"[core]typo", "[core] typo", "[core \"x\"]typo", "[core]]"} {
		path := filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(path, []byte(hdr+"\n\tbare = false\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := iniconfig.Load(path); err == nil {
			t.Errorf("Load(%q): expected error", hdr)
		}
	}
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
		{"core.\u05d0b", "core", "", "\u05d0b", false},
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

func TestParse_BracketInSubsection(t *testing.T) {
	roundTrip(t, "[branch \"x]y\"]\n\tremote = origin\n", []iniconfig.Entry{
		{Key: "branch.x]y.remote", Value: "origin"},
	})
}

func TestParse_BooleanKeyInlineComment(t *testing.T) {
	roundTrip(t, "[core]\n\tbare # enable this\n", []iniconfig.Entry{
		{Key: "core.bare", Value: "true"},
	})
}

func TestParse_TrailingBackslashRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[core]\n\tname = abc\\\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := iniconfig.Load(path); err == nil {
		t.Fatal("expected error for trailing backslash")
	}
}

func TestSetRoundTripSpecialValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	f, _ := iniconfig.Load(path)
	vals := map[string]string{
		"a.trail": "value ",
		"a.lead":  " value",
		"a.bs":    `C:\dir\`,
		// Invalid UTF-8 must survive both the quoted and unquoted paths.
		"a.badquoted": "\xff#\xc3",
		"a.badplain":  "\xfe\x80",
	}
	for k, v := range vals {
		if err := f.Set(k, v); err != nil {
			t.Fatalf("Set(%q): %v", k, err)
		}
	}
	f2, err := iniconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range vals {
		if got, ok := f2.Get(k); !ok || got != want {
			t.Errorf("Get(%q) = %q, %v; want %q", k, got, ok, want)
		}
	}
}

// controlCharValues are values containing line-break characters.
var controlCharValues = []string{
	"a\nb", "a\rb", "a\r\nb", "\n", "\r", "tail\r", "tail\n", "\r\n\r\n", "x\ty\n#z",
}

func TestSetRoundTripControlChars(t *testing.T) {
	for _, want := range controlCharValues {
		t.Run(fmt.Sprintf("%q", want), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			f, _ := iniconfig.Load(path)
			if err := f.Set("a.v", want); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.ContainsRune(raw, '\r') || bytes.Count(raw, []byte("\n")) != 2 {
				t.Errorf("file has raw line breaks in value: %q", raw)
			}
			f2, err := iniconfig.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if got, ok := f2.Get("a.v"); !ok || got != want {
				t.Errorf("Get = %q, %v; want %q", got, ok, want)
			}
		})
	}
}

func TestList_OneLinePerKey(t *testing.T) {
	for _, want := range append([]string{"plain", "a b", " pad ", `q"uote`, `C:\dir`, "a#b"}, controlCharValues...) {
		t.Run(fmt.Sprintf("%q", want), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			f, _ := iniconfig.Load(path)
			if err := f.Set("a.v", want); err != nil {
				t.Fatal(err)
			}
			if err := f.Set("a.w", "next"); err != nil {
				t.Fatal(err)
			}
			var sb strings.Builder
			if err := f.List(&sb); err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSuffix(sb.String(), "\n"), "\n")
			if len(lines) != 2 || strings.Contains(sb.String(), "\r") || lines[1] != "a.w=next" {
				t.Fatalf("want one line per key, got %q", sb.String())
			}
			// The listed value must decode back to the original.
			val, ok := strings.CutPrefix(lines[0], "a.v=")
			if !ok {
				t.Fatalf("unexpected line %q", lines[0])
			}
			rt := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(rt, []byte("[a]\n\tv = "+val+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			f2, err := iniconfig.Load(rt)
			if err != nil {
				t.Fatal(err)
			}
			if got, ok := f2.Get("a.v"); !ok || got != want {
				t.Errorf("listed value decodes to %q, %v; want %q", got, ok, want)
			}
		})
	}
}

func TestSetSubsectionWithBracket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	f, _ := iniconfig.Load(path)
	if err := f.Set("branch.x]y.remote", "origin"); err != nil {
		t.Fatal(err)
	}
	f2, err := iniconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := f2.Get("branch.x]y.remote"); !ok || v != "origin" {
		t.Fatalf("got %q, %v", v, ok)
	}
}

func TestParseKey_Invalid(t *testing.T) {
	for _, k := range []string{"core..name", "core.a\nb.name", "core.a\rb.name", "core.a\x00b.name", "core]x.name", `co"re.name`, "co re.name", "core.\u0663x"} {
		if _, _, _, err := iniconfig.ParseKey(k); err == nil {
			t.Errorf("ParseKey(%q): expected error", k)
		}
	}
}

func TestSet_LockHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f, _ := iniconfig.Load(path)
	if err := f.Set("core.bare", "true"); err == nil {
		t.Fatal("expected error while lock is held")
	}
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Errorf("foreign lock file must not be removed: %v", err)
	}
}

func TestSet_KeepsConcurrentUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	f1, _ := iniconfig.Load(path)
	f2, _ := iniconfig.Load(path)
	if err := f1.Set("core.a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := f2.Set("core.b", "2"); err != nil {
		t.Fatal(err)
	}
	got, err := iniconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []iniconfig.Entry{{Key: "core.a", Value: "1"}, {Key: "core.b", Value: "2"}}
	if !slices.Equal(got.Entries(), want) {
		t.Errorf("got %q, want %q", got.Entries(), want)
	}
}

func TestLoad_LineLengthLimit(t *testing.T) {
	const limit = 1 << 20
	head := "[core]\n\tk = "
	for _, tt := range []struct {
		name    string
		content string
		wantErr bool
	}{
		{"exact limit LF", head + strings.Repeat("a", limit-len("\tk = ")) + "\n", false},
		{"exact limit CRLF", head + strings.Repeat("a", limit-len("\tk = ")) + "\r\n", false},
		{"over limit", head + strings.Repeat("a", limit-len("\tk = ")+1) + "\n", true},
	} {
		path := filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := iniconfig.Load(path); (err != nil) != tt.wantErr {
			t.Errorf("%s: err=%v, wantErr=%v", tt.name, err, tt.wantErr)
		}
	}
}

func TestSet_RejectsOverLongLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	f, _ := iniconfig.Load(path)
	if err := f.Set("core.a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := f.Set("core.big", strings.Repeat("a", 1<<20)); err == nil {
		t.Fatal("expected error for over-long value")
	}
	got, err := iniconfig.Load(path)
	if err != nil {
		t.Fatalf("config must stay loadable: %v", err)
	}
	if want := []iniconfig.Entry{{Key: "core.a", Value: "1"}}; !slices.Equal(got.Entries(), want) {
		t.Errorf("got %q, want %q", got.Entries(), want)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Errorf("lock file must be removed: %v", err)
	}
}

func TestLoad_InvalidSectionName(t *testing.T) {
	for _, content := range []string{"[co re]\nname = x\n", "[]\nname = x\n", "[co.re]\nname = x\n", "[co re \"sub\"]\nname = x\n", "[branch \"\"]\nremote = x\n", "[branch \"a\x00b\"]\nremote = x\n"} {
		path := filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := iniconfig.Load(path); err == nil {
			t.Errorf("Load(%q): expected error", content)
		}
	}
}
