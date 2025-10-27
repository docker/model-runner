package utils

import (
	"regexp"
	"strings"
	"unicode"
)

// SanitizeForLog sanitizes a string for safe logging by removing or escaping
// control characters that could cause log injection attacks.
// TODO: Consider migrating to structured logging which
// handles sanitization automatically through field encoding.
func SanitizeForLog(s string) string {
	if s == "" {
		return ""
	}

	var result strings.Builder
	result.Grow(len(s))

	for _, r := range s {
		switch {
		// Replace newlines and carriage returns with escaped versions.
		case r == '\n':
			result.WriteString("\\n")
		case r == '\r':
			result.WriteString("\\r")
		case r == '\t':
			result.WriteString("\\t")
		// Remove other control characters (0x00-0x1F, 0x7F).
		case unicode.IsControl(r):
			// Skip control characters or replace with placeholder.
			result.WriteString("?")
		// Escape backslashes to prevent escape sequence injection.
		case r == '\\':
			result.WriteString("\\\\")
		// Keep printable characters.
		case unicode.IsPrint(r):
			result.WriteRune(r)
		default:
			// Replace non-printable characters with placeholder.
			result.WriteString("?")
		}
	}

	const maxLength = 100
	if result.Len() > maxLength {
		return result.String()[:maxLength] + "...[truncated]"
	}

	return result.String()
}
// SanitizeModelNameForCommand sanitizes a model name to be safe for use in command arguments.
// This prevents command injection by allowing only safe characters in model names.
func SanitizeModelNameForCommand(modelName string) string {
	if modelName == "" {
		return ""
	}
	
	// Only allow alphanumeric characters, dots, hyphens, underscores, and forward slashes
	// This covers common model name formats like "user/model-name", "model.v1", etc.
	re := regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)
	if !re.MatchString(modelName) {
		// If the model name contains unsafe characters, clean it
		// Replace unsafe characters with underscores
		safeModelName := regexp.MustCompile(`[^a-zA-Z0-9._/-]`).ReplaceAllString(modelName, "_")
		// Ensure it doesn't start or end with unsafe sequences like ".." that could be path traversal
		safeModelName = strings.ReplaceAll(safeModelName, "..", "_")
		return safeModelName
	}
	
	return modelName
}