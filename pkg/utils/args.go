package utils

import (
	"github.com/mattn/go-shellwords"
)

// SplitArgs splits a string into arguments, respecting quoted strings.
// This is a wrapper around shellwords.Parse for convenience.
func SplitArgs(s string) []string {
	args, err := shellwords.Parse(s)
	if err != nil {
		// If parsing fails, return empty slice
		// The caller can check for empty result
		return []string{}
	}
	return args
}
