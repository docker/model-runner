// Package utils provides utility functions for internal use.
package utils

import (
	"strings"
	"unicode"
)

// SanitizeForLog sanitizes a string for safe logging by removing or escaping
// control characters that could cause log injection attacks.
// The optional maxLength parameter controls truncation (default: 100).
// Pass 0 or negative to disable truncation.
// TODO: Consider migrating to structured logging which
// handles sanitization automatically through field encoding.
func SanitizeForLog(s string, maxLength ...int) string {
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

	// Default maxLength is 100, or use provided value.
	// Pass 0 or negative to disable truncation.
	maxLen := 100
	if len(maxLength) > 0 {
		maxLen = maxLength[0]
	}

	if maxLen > 0 && result.Len() > maxLen {
		return result.String()[:maxLen] + "...[truncated]"
	}

	return result.String()
}
