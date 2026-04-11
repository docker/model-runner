package readline

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	defaultEditor = "vi"
	defaultShell  = "/bin/sh"
	windowsEditor = "notepad"
	windowsShell  = "cmd"
)

func openInEditor(fd uintptr, termios any, content string) (string, error) {
	if err := UnsetRawMode(fd, termios); err != nil {
		return content, err
	}

	edited, err := runEditor(content)

	if _, restoreErr := SetRawMode(fd); restoreErr != nil {
		return content, errors.Join(err, restoreErr)
	}

	if err != nil {
		return content, err
	}

	return edited, nil
}

func platformize(linux, windows string) string {
	if runtime.GOOS == "windows" {
		return windows
	}
	return linux
}

func defaultEnvShell() []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = platformize(defaultShell, windowsShell)
	}
	flag := "-c"
	if shell == windowsShell {
		flag = "/C"
	}
	return []string{shell, flag}
}

func resolveEditor() ([]string, bool) {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = platformize(defaultEditor, windowsEditor)
	}

	if !strings.Contains(editor, " ") {
		return []string{editor}, false
	}

	if !strings.ContainsAny(editor, "\"'\\") {
		return strings.Split(editor, " "), false
	}

	shell := defaultEnvShell()
	return append(shell, editor), true
}

func buildEditorCmd(filePath string) *exec.Cmd {
	args, shell := resolveEditor()

	if shell {
		// The editor string is the last element — append the file path to it
		safeFilePath := strings.ReplaceAll(filePath, "'", "'\\''")
		args[len(args)-1] = fmt.Sprintf("%s '%s'", args[len(args)-1], safeFilePath)
	} else {
		args = append(args, filePath)
	}

	//nolint:gosec // $EDITOR is a user-controlled local env var, same trust model as git/kubectl
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func runEditor(content string) (string, error) {
	tmpFile, err := os.CreateTemp("", "docker-model-prompt-*.txt")
	if err != nil {
		return content, err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return content, err
	}
	tmpFile.Close()

	cmd := buildEditorCmd(tmpFile.Name())
	if err := cmd.Run(); err != nil {
		return content, err
	}

	edited, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return content, err
	}

	return strings.TrimRight(string(edited), "\r\n"), nil
}
