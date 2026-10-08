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
	tool, err := validateSandboxTool("sbx")
	if err != nil {
		t.Fatalf("validateSandboxTool() error = %v", err)
	}

	if tool != "sbx" {
		t.Fatalf("validateSandboxTool() = %q, want %q", tool, "sbx")
	}
}

func TestValidateSandboxToolRejectsUnsupportedTool(t *testing.T) {
	_, err := validateSandboxTool("firejail")
	if err == nil {
		t.Fatal("validateSandboxTool() error = nil, want error")
	}
}

func TestConfigCommandRejectsUnsupportedKey(t *testing.T) {
	cmd := newConfigCmd()
	cmd.SetArgs([]string{"unsupported.key", "sbx"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("config command error = nil, want error")
	}
}

func TestConfigCommandRejectsUnsupportedTool(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newConfigCmd()
	cmd.SetArgs([]string{"sandbox.tool", "firejail"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("config command error = nil, want error")
	}
}

func TestConfigCommandWritesSandboxToolConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newConfigCmd()
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

func TestConfiguredSandboxToolMissingConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	got, err := configuredSandboxTool()
	if err != nil {
		t.Fatalf("configuredSandboxTool() error = %v", err)
	}

	if got != "" {
		t.Fatalf("configuredSandboxTool() = %q, want empty string", got)
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
