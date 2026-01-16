package diffusers

import (
	"fmt"
	"regexp"
	"strings"
)

// pythonErrorPatterns contains regex patterns to extract meaningful error messages
// from Python tracebacks. The patterns are tried in order, and the first match wins.
var pythonErrorPatterns = []*regexp.Regexp{
	// Custom error marker from our Python server (highest priority)
	regexp.MustCompile(`(?m)^DIFFUSERS_ERROR:\s*(.+)$`),
	// Python RuntimeError, ValueError, etc.
	regexp.MustCompile(`(?m)^(RuntimeError|ValueError|TypeError|OSError|ImportError|ModuleNotFoundError):\s*(.+)$`),
	// CUDA/GPU related errors
	regexp.MustCompile(`(?mi)(CUDA|GPU|out of memory|OOM|No GPU found)[^.]*\.?`),
	// Generic Python Exception with message
	regexp.MustCompile(`(?m)^(\w+Error):\s*(.+)$`),
}

// ExtractPythonError attempts to extract a meaningful error message from Python output.
// It looks for common error patterns and returns a cleaner, more user-friendly message.
// If no recognizable pattern is found, it returns the original output.
func ExtractPythonError(output string) string {
	// Try each pattern in order
	for i, pattern := range pythonErrorPatterns {
		matches := pattern.FindStringSubmatch(output)
		if len(matches) > 0 {
			switch i {
			case 0:
				// Custom error marker: return just the message
				return strings.TrimSpace(matches[1])
			case 1:
				// Standard Python errors: "ErrorType: message"
				return fmt.Sprintf("%s: %s", matches[1], strings.TrimSpace(matches[2]))
			case 2:
				// GPU/CUDA related errors
				return strings.TrimSpace(matches[0])
			case 3:
				// Generic Python errors
				return fmt.Sprintf("%s: %s", matches[1], strings.TrimSpace(matches[2]))
			}
		}
	}

	// No pattern matched - return original but try to trim some noise
	// Take only the last few meaningful lines
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > 5 {
		// Return the last 5 non-empty lines
		var meaningful []string
		for i := len(lines) - 1; i >= 0 && len(meaningful) < 5; i-- {
			line := strings.TrimSpace(lines[i])
			if line != "" && !strings.HasPrefix(line, "  ") {
				meaningful = append([]string{line}, meaningful...)
			}
		}
		if len(meaningful) > 0 {
			return strings.Join(meaningful, "\n")
		}
	}

	return output
}
