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
