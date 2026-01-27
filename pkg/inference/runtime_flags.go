package inference

import (
	"fmt"
	"strings"
)

// ValidateRuntimeFlags validates runtime flags against the backend's allowlist
// and checks for path characters as defense-in-depth.
func ValidateRuntimeFlags(backendName string, flags []string) error {
	// Get allowlist for this backend
	allowedFlags := GetAllowedFlags(backendName)

	// Check each flag against allowlist
	for _, flag := range flags {
		flagKey := ParseFlagKey(flag)
		if flagKey == "" {
			continue // Skip values, only validate flag keys
		}
		if !allowedFlags[flagKey] {
			return fmt.Errorf("runtime flag %q is not allowed for backend %q", flagKey, backendName)
		}
	}

	// still check for path characters in values
	return validatePathSafety(flags)
}

// validatePathSafety ensures runtime flags don't contain paths (forward slash "/" or backslash "\")
// to prevent malicious users from overwriting host files via arguments like
// --log-file /some/path, --output-file /etc/passwd, or --log-file C:\Windows\file.
//
// This validation rejects any flag or value containing "/" or "\" to block:
// - Unix/Linux/macOS absolute paths: /var/log/file, /etc/passwd
// - Unix/Linux/macOS relative paths: ../file.txt, ./config
// - Windows absolute paths: C:\Users\file, D:\data\file
// - Windows relative paths: ..\file.txt, .\config
// - UNC paths: \\network\share\file
//
// Returns an error if any flag contains a forward slash or backslash.
func validatePathSafety(flags []string) error {
	for _, flag := range flags {
		if strings.Contains(flag, "/") || strings.Contains(flag, "\\") {
			return fmt.Errorf("invalid runtime flag %q: paths are not allowed (contains '/' or '\\\\')", flag)
		}
	}
	return nil
}
