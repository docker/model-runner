package utils

import (
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
