package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// modelfileAliases maps Modelfile instruction aliases to their canonical names.
var modelfileAliases = map[string]string{
	"SAFETENSORS-DIR": "SAFETENSORS_DIR",
	"CHAT-TEMPLATE":   "CHAT_TEMPLATE",
	"MM-PROJ":         "MMPROJ",
	"CTX":             "CONTEXT",
	"CONTEXT-SIZE":    "CONTEXT",
}

// modelfilePathInstructions is the set of instructions whose value is a file or directory path.
var modelfilePathInstructions = map[string]struct{}{
	"GGUF":            {},
	"SAFETENSORS_DIR": {},
	"DDUF":            {},
	"LICENSE":         {},
	"CHAT_TEMPLATE":   {},
	"MMPROJ":          {},
}

// applyModelfile reads opts.modelfile and applies its directives to opts.
// CLI flags take precedence over Modelfile values.
func applyModelfile(opts *packageOptions) error {
	if opts.modelfile == "" {
		return nil
	}

	absModelfile, err := filepath.Abs(opts.modelfile)
	if err != nil {
		return fmt.Errorf("resolve Modelfile path %q: %w", opts.modelfile, err)
	}
	baseDir := filepath.Dir(absModelfile)

	f, err := os.Open(absModelfile)
	if err != nil {
		return fmt.Errorf("open Modelfile %q: %w", opts.modelfile, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			return fmt.Errorf("Modelfile line %d: expected an instruction and a value, got: %q", lineNum, line)
		}

		instruction := strings.ToUpper(fields[0])
		if canonical, ok := modelfileAliases[instruction]; ok {
			instruction = canonical
		}

		value := strings.Join(fields[1:], " ")

		var absPath string
		if _, isPath := modelfilePathInstructions[instruction]; isPath {
			absPath, err = modelfileResolvePath(value, baseDir)
			if err != nil {
				return fmt.Errorf("Modelfile line %d: invalid path for %s: %w", lineNum, instruction, err)
			}

			info, statErr := os.Stat(absPath)
			if statErr != nil {
				return fmt.Errorf("Modelfile line %d: path for %s not found: %q", lineNum, instruction, absPath)
			}

			switch instruction {
			case "SAFETENSORS_DIR":
				if !info.IsDir() {
					return fmt.Errorf("Modelfile line %d: SAFETENSORS_DIR must be a directory: %q", lineNum, absPath)
				}
			case "GGUF", "DDUF", "LICENSE", "CHAT_TEMPLATE", "MMPROJ":
				if info.IsDir() {
					return fmt.Errorf("Modelfile line %d: %s must be a file, not a directory: %q", lineNum, instruction, absPath)
				}
			}
		}

		switch instruction {
		// Model sources
		case "FROM":
			if opts.fromModel == "" {
				if strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || filepath.IsAbs(value) {
					return fmt.Errorf("Modelfile line %d: FROM takes a model reference, not a file path; use GGUF or SAFETENSORS_DIR instead", lineNum)
				}
				opts.fromModel = value
			}

		case "GGUF":
			if opts.ggufPath == "" {
				opts.ggufPath = absPath
			}

		case "SAFETENSORS_DIR":
			if opts.safetensorsDir == "" {
				opts.safetensorsDir = absPath
			}

		case "DDUF":
			if opts.ddufPath == "" {
				opts.ddufPath = absPath
			}

		// Optional assets
		case "LICENSE":
			if !slices.Contains(opts.licensePaths, absPath) {
				opts.licensePaths = append(opts.licensePaths, absPath)
			}

		case "CHAT_TEMPLATE":
			if opts.chatTemplatePath == "" {
				opts.chatTemplatePath = absPath
			}

		case "MMPROJ":
			if opts.mmprojPath == "" {
				opts.mmprojPath = absPath
			}

		// Parameters
		case "CONTEXT":
			if opts.contextSize == 0 {
				v, parseErr := strconv.ParseUint(value, 10, 64)
				if parseErr != nil || v == 0 {
					return fmt.Errorf("Modelfile line %d: invalid CONTEXT value %q: must be a positive integer", lineNum, value)
				}
				opts.contextSize = v
				opts.contextSizeSet = true
			}

		default:
			return fmt.Errorf("Modelfile line %d: unknown instruction %q", lineNum, instruction)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read Modelfile %q: %w", opts.modelfile, err)
	}

	return nil
}

// modelfileResolvePath returns path as an absolute cleaned path, resolved
// relative to baseDir when path is not already absolute.
func modelfileResolvePath(path, baseDir string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
