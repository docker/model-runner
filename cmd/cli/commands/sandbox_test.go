package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadSandboxToolConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := writeSandboxToolConfig("sbx"); err != nil {
		t.Fatalf("writeSandboxToolConfig() error = %v", err)
	}

	got, err := readSandboxToolConfig()
	if err != nil {
		t.Fatalf("readSandboxToolConfig() error = %v", err)
	}

	if got != "sbx" {
		t.Fatalf("readSandboxToolConfig() = %q, want %q", got, "sbx")
	}

	configPath, err := dmrConfigPath()
	if err != nil {
		t.Fatalf("dmrConfigPath() error = %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", configPath, err)
	}

	want := "[sandbox]\ntool = \"sbx\"\n"
	if string(content) != want {
		t.Fatalf("config content = %q, want %q", string(content), want)
	}
}

func TestReadSandboxToolConfigMissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	got, err := readSandboxToolConfig()
	if err != nil {
		t.Fatalf("readSandboxToolConfig() error = %v", err)
	}

	if got != "" {
		t.Fatalf("readSandboxToolConfig() = %q, want empty string", got)
	}
}

func TestValidateSandboxToolAllowsSbx(t *testing.T) {
	if err := validateSandboxTool("sbx"); err != nil {
		t.Fatalf("validateSandboxTool() error = %v", err)
	}
}

func TestValidateSandboxToolRejectsUnsupportedTool(t *testing.T) {
	err := validateSandboxTool("firejail")
	if err == nil {
		t.Fatal("validateSandboxTool() error = nil, want error")
	}
}

func TestSandboxConfigCommandRejectsUnsupportedKey(t *testing.T) {
	cmd := newSandboxConfigCmd()
	cmd.SetArgs([]string{"unsupported.key", "sbx"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("config command error = nil, want error")
	}
}

func TestSandboxConfigCommandRejectsUnsupportedTool(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newSandboxConfigCmd()
	cmd.SetArgs([]string{"sandbox.tool", "firejail"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("config command error = nil, want error")
	}
}

func TestSandboxConfigCommandWritesConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newSandboxConfigCmd()
	cmd.SetArgs([]string{"sandbox.tool", "sbx"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config command error = %v", err)
	}

	got, err := readSandboxToolConfig()
	if err != nil {
		t.Fatalf("readSandboxToolConfig() error = %v", err)
	}

	if got != "sbx" {
		t.Fatalf("readSandboxToolConfig() = %q, want %q", got, "sbx")
	}
}

func TestLaunchCommandRequiresConfiguredSandboxTool(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newLaunchCmd()
	cmd.SetArgs([]string{"opencode"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("launch command error = nil, want error")
	}
}

func TestLaunchCommandUsesConfiguredSandboxTool(t *testing.T) {
	configDir := t.TempDir()
	binDir := t.TempDir()
	outputPath := filepath.Join(t.TempDir(), "output.txt")

	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("TEST_OUTPUT", outputPath)

	sbxPath := filepath.Join(binDir, "sbx")
	sbxScript := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_OUTPUT\"\n"

	if err := os.WriteFile(sbxPath, []byte(sbxScript), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", sbxPath, err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

	if err := writeSandboxToolConfig("sbx"); err != nil {
		t.Fatalf("writeSandboxToolConfig() error = %v", err)
	}

	cmd := newLaunchCmd()
	cmd.SetArgs([]string{"opencode", "--", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("launch command error = %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", outputPath, err)
	}

	want := "opencode\n--help\n"
	if string(content) != want {
		t.Fatalf("sandbox output = %q, want %q", string(content), want)
	}
}
