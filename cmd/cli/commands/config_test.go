package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runConfig(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newConfigCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestResolveConfigPathExclusiveFlags(t *testing.T) {
	tests := []struct {
		name    string
		global  bool
		system  bool
		file    string
		wantErr bool
	}{
		{"none", false, false, "", false},
		{"global", true, false, "", false},
		{"system", false, true, "", false},
		{"file", false, false, "x", false},
		{"global+system", true, true, "", true},
		{"global+file", true, false, "x", true},
		{"system+file", false, true, "x", true},
		{"all", true, true, "x", true},
	}
	for _, tt := range tests {
		_, err := resolveConfigPath(tt.global, tt.system, tt.file)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err=%v, wantErr=%v", tt.name, err, tt.wantErr)
		}
	}
}

func TestConfigGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	content := "[user]\n\tname = a\n\tname = b\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want string
		err  bool
	}{
		{"last", []string{"get", "-f", path, "user.name"}, "b\n", false},
		{"all", []string{"get", "-f", path, "--all", "user.name"}, "a\nb\n", false},
		{"origin", []string{"get", "-f", path, "--show-origin", "user.name"}, "file:" + path + "\tb\n", false},
		{"all origin", []string{"get", "-f", path, "--all", "--show-origin", "user.name"}, "file:" + path + "\ta\nfile:" + path + "\tb\n", false},
		{"default", []string{"get", "-f", path, "--default", "d", "user.none"}, "d\n", false},
		{"default all", []string{"get", "-f", path, "--all", "--default", "d", "user.none"}, "d\n", false},
		{"missing", []string{"get", "-f", path, "user.none"}, "", true},
		{"missing all", []string{"get", "-f", path, "--all", "user.none"}, "", true},
		{"bad key", []string{"get", "-f", path, "bad-key"}, "", true},
		{"bad key default", []string{"get", "-f", path, "--default", "d", "bad-key"}, "", true},
		{"bad key all default", []string{"get", "-f", path, "--all", "--default", "d", "user..name"}, "", true},
	}
	for _, tt := range tests {
		out, err := runConfig(t, tt.args...)
		if (err != nil) != tt.err {
			t.Errorf("%s: err=%v, wantErr=%v", tt.name, err, tt.err)
			continue
		}
		if !tt.err && out != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, out, tt.want)
		}
	}
}

func TestConfigListOneLinePerKey(t *testing.T) {
	for _, v := range []string{"plain", "a\nb", "a\rb", "a\r\nb", "\n", "tail\r", `q"uote`, "a#b"} {
		path := filepath.Join(t.TempDir(), "config")
		for _, args := range [][]string{{"set", "-f", path, "a.v", v}, {"set", "-f", path, "a.w", "next"}} {
			if _, err := runConfig(t, args...); err != nil {
				t.Fatalf("%q: %v", args, err)
			}
		}
		plain, err := runConfig(t, "list", "-f", path)
		if err != nil {
			t.Fatal(err)
		}
		origin, err := runConfig(t, "list", "-f", path, "--show-origin")
		if err != nil {
			t.Fatal(err)
		}
		for name, out := range map[string]string{"list": plain, "show-origin": origin} {
			if n := strings.Count(out, "\n"); n != 2 || strings.Contains(out, "\r") {
				t.Errorf("%q %s: want one line per key, got %q", v, name, out)
			}
		}
		// --show-origin must use the same value representation as list.
		var want strings.Builder
		for _, line := range strings.SplitAfter(plain, "\n") {
			if line != "" {
				want.WriteString("file:" + path + "\t" + line)
			}
		}
		if origin != want.String() {
			t.Errorf("%q: show-origin got %q, want %q", v, origin, want.String())
		}
	}
}

func TestConfigGetWritesToStdout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[user]\n\tname = a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newConfigCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"get", "-f", path, "user.name"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "a\n" || stderr.Len() != 0 {
		t.Errorf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestDefaultConfigPathNoHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if p, err := defaultConfigPath(); err == nil {
		t.Skipf("home directory still resolved: %q", p)
	}
}

func TestConfigLocationFlagsConflict(t *testing.T) {
	_, err := runConfig(t, "get", "--global", "--system", "user.name")
	if err == nil || !strings.Contains(err.Error(), "only one of") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestEditorCommand(t *testing.T) {
	tests := []struct {
		name, visual, editor string
		want                 string
	}{
		{"visual", "code --wait", "nano", "code --wait"},
		{"editor", "", "nano -w", "nano -w"},
		{"blank visual", "  ", "nano", "nano"},
		{"blank both", " ", "\t", ""},
	}
	for _, tt := range tests {
		t.Setenv("VISUAL", tt.visual)
		t.Setenv("EDITOR", tt.editor)
		got := editorCommand()
		if len(got) == 0 {
			t.Fatalf("%s: empty editor command", tt.name)
		}
		if tt.want != "" && strings.Join(got, " ") != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
