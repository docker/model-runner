// Package iniconfig implements reading and writing of INI-style config files.
// The format uses sections, optional subsections, and key=value pairs:
//
//	[section]
//		key = value
//	[section "subsection"]
//		key = value
//
// Key names are of the form "section.key" or "section.subsection.key".
// Section names and variable names are case-insensitive; subsection names are
// case-sensitive.
package iniconfig

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// maxConfigLineBytes is the hard cap on a single config line length (1 MiB).
// This guards against unbounded memory use on pathological inputs while still
// accommodating large values such as certificates or long tokens.
const maxConfigLineBytes = 1 << 20

// Entry is a single key/value pair from a config file.
type Entry struct {
	// Key is the canonical dotted key: "section.variable" or
	// "section.subsection.variable". Section and variable are lowercased;
	// subsection preserves its original case.
	Key   string
	Value string
}

// File represents a parsed config file and the path it was read from.
type File struct {
	path    string
	entries []Entry
}

// Path returns the file path associated with this File.
func (f *File) Path() string { return f.path }

// Entries returns all key/value pairs in file order.
func (f *File) Entries() []Entry { return f.entries }

// ----------------------------------------------------------------------------
// Reading
// ----------------------------------------------------------------------------

// Load reads the config file at path. If the file does not exist an empty File
// is returned without error.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{path: path}, nil
		}
		return nil, err
	}
	entries, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &File{path: path, entries: entries}, nil
}

// parse parses INI bytes into a slice of Entries.
func parse(data []byte) ([]Entry, error) {
	// Strip UTF-8 BOM if present.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	var entries []Entry
	var section, subsection string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), maxConfigLineBytes)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Empty line or comment.
		if trimmed == "" || trimmed[0] == '#' || trimmed[0] == ';' {
			continue
		}

		if trimmed[0] == '[' {
			// Section header.
			var err error
			section, subsection, err = parseSectionHeader(trimmed)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			continue
		}

		// Key-value (or boolean key).
		if section == "" {
			return nil, fmt.Errorf("line %d: key outside of section", lineNum)
		}
		key, value, err := parseKeyValue(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		canonical := canonicalKey(section, subsection, key)
		entries = append(entries, Entry{Key: canonical, Value: value})
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, fmt.Errorf("line %d: config line too long (max %d bytes)", lineNum+1, maxConfigLineBytes)
		}
		return nil, err
	}
	return entries, nil
}

// parseSectionHeader parses "[section]" or `[section "subsection"]`.
// Anything after the closing ']' (e.g. inline comments) is ignored.
// section is returned lowercased; subsection preserves case.
func parseSectionHeader(line string) (section, subsection string, err error) {
	// Find the closing bracket; ignore anything that follows (inline comment).
	closeIdx := strings.IndexByte(line, ']')
	if closeIdx < 0 {
		return "", "", fmt.Errorf("invalid section header: %q", line)
	}
	inner := line[1:closeIdx]

	// Check for subsection: section "subsection"
	if idx := strings.Index(inner, "\""); idx >= 0 {
		rawSection := strings.TrimRight(inner[:idx], " \t")
		rawSubsection := inner[idx:]
		if !strings.HasPrefix(rawSubsection, "\"") || !strings.HasSuffix(rawSubsection, "\"") || len(rawSubsection) < 2 {
			return "", "", fmt.Errorf("invalid section header: %q", line)
		}
		sub, err2 := unescapeSubsection(rawSubsection[1 : len(rawSubsection)-1])
		if err2 != nil {
			return "", "", fmt.Errorf("invalid subsection in %q: %w", line, err2)
		}
		return strings.ToLower(rawSection), sub, nil
	}

	return strings.ToLower(strings.TrimSpace(inner)), "", nil
}

// unescapeSubsection handles backslash escapes inside subsection names.
// \\ and \" are unescaped; a lone \ followed by any other char is silently
// dropped (the character after it is kept).
func unescapeSubsection(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			default:
				// Unknown escape: drop the backslash, keep the character.
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String(), nil
}

// parseKeyValue parses a line of the form "   key = value  # comment" or a
// boolean "   key". Supports line continuation with trailing backslash.
func parseKeyValue(line string) (key, value string, err error) {
	trimmed := strings.TrimLeft(line, " \t")

	eqIdx := strings.IndexByte(trimmed, '=')
	if eqIdx < 0 {
		// Boolean key: no "=", value is implicitly "true".
		key = strings.TrimRight(trimmed, " \t")
		if err2 := validateVarName(key); err2 != nil {
			return "", "", err2
		}
		return strings.ToLower(key), "true", nil
	}

	key = strings.TrimRight(trimmed[:eqIdx], " \t")
	if err2 := validateVarName(key); err2 != nil {
		return "", "", err2
	}

	raw := strings.TrimLeft(trimmed[eqIdx+1:], " \t")
	val, err2 := parseValue(raw)
	if err2 != nil {
		return "", "", err2
	}
	return strings.ToLower(key), val, nil
}

// parseValue decodes the value portion of a key-value line, handling quoting,
// escape sequences, and inline comments.
func parseValue(raw string) (string, error) {
	var b strings.Builder
	inQuotes := false
	i := 0
	for i < len(raw) {
		c := raw[i]
		switch {
		case !inQuotes && (c == '#' || c == ';'):
			// Inline comment — stop.
			goto done
		case !inQuotes && c == '"':
			inQuotes = true
			i++
		case inQuotes && c == '"':
			inQuotes = false
			i++
		case c == '\\':
			if i+1 >= len(raw) {
				// Trailing backslash = line continuation (we don't handle
				// multi-line here; treat as end of value).
				goto done
			}
			i++
			switch raw[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'b':
				b.WriteByte('\b')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			default:
				return "", fmt.Errorf("unknown escape sequence \\%c", raw[i])
			}
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
done:
	if inQuotes {
		return "", fmt.Errorf("unterminated quoted string")
	}
	result := b.String()
	if !inQuotes {
		result = strings.TrimRight(result, " \t")
	}
	return result, nil
}

// validateVarName ensures a variable name contains only [A-Za-z0-9-] and
// starts with a letter.
func validateVarName(name string) error {
	if name == "" {
		return fmt.Errorf("empty variable name")
	}
	if !unicode.IsLetter(rune(name[0])) {
		return fmt.Errorf("variable name %q must start with a letter", name)
	}
	for _, c := range name {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' {
			return fmt.Errorf("invalid character %q in variable name %q", c, name)
		}
	}
	return nil
}

// canonicalKey assembles the canonical dotted key.
func canonicalKey(section, subsection, variable string) string {
	section = strings.ToLower(section)
	variable = strings.ToLower(variable)
	if subsection == "" {
		return section + "." + variable
	}
	return section + "." + subsection + "." + variable
}

// ----------------------------------------------------------------------------
// Key parsing (for CLI inputs)
// ----------------------------------------------------------------------------

// ParseKey splits a dotted key "section.variable" or
// "section.subsection.variable" into its components. Section and variable are
// lowercased; subsection preserves case. The split point is the last dot.
func ParseKey(key string) (section, subsection, variable string, err error) {
	// The last dot separates the variable from the section[.subsection] part.
	lastDot := strings.LastIndex(key, ".")
	if lastDot < 0 {
		return "", "", "", fmt.Errorf("invalid key %q: must contain at least one dot", key)
	}
	variable = strings.ToLower(key[lastDot+1:])
	prefix := key[:lastDot]

	// The first dot (if any) separates section from subsection.
	firstDot := strings.Index(prefix, ".")
	if firstDot < 0 {
		section = strings.ToLower(prefix)
		subsection = ""
	} else {
		section = strings.ToLower(prefix[:firstDot])
		subsection = prefix[firstDot+1:] // subsection preserves case
	}

	if section == "" {
		return "", "", "", fmt.Errorf("invalid key %q: empty section", key)
	}
	if variable == "" {
		return "", "", "", fmt.Errorf("invalid key %q: empty variable", key)
	}
	if err2 := validateVarName(variable); err2 != nil {
		return "", "", "", fmt.Errorf("invalid key %q: %w", key, err2)
	}
	return section, subsection, variable, nil
}

// ----------------------------------------------------------------------------
// Querying
// ----------------------------------------------------------------------------

// Get returns the last value for the given canonical key. The second return
// value is false if the key is not present.
func (f *File) Get(key string) (string, bool) {
	section, subsection, variable, err := ParseKey(key)
	if err != nil {
		return "", false
	}
	canonical := canonicalKey(section, subsection, variable)
	found := false
	last := ""
	for _, e := range f.entries {
		if e.Key == canonical {
			last = e.Value
			found = true
		}
	}
	return last, found
}

// GetAll returns all values for the given canonical key.
func (f *File) GetAll(key string) []string {
	section, subsection, variable, err := ParseKey(key)
	if err != nil {
		return nil
	}
	canonical := canonicalKey(section, subsection, variable)
	var vals []string
	for _, e := range f.entries {
		if e.Key == canonical {
			vals = append(vals, e.Value)
		}
	}
	return vals
}

// ----------------------------------------------------------------------------
// Writing
// ----------------------------------------------------------------------------

// Set writes key=value to the file, replacing the last existing occurrence or
// appending if absent. The file is written atomically via a lock file.
func (f *File) Set(key, value string) error {
	section, subsection, variable, err := ParseKey(key)
	if err != nil {
		return err
	}
	canonical := canonicalKey(section, subsection, variable)
	return f.writeAtomic(func(entries []Entry) []Entry {
		replaced := false
		for i := len(entries) - 1; i >= 0; i-- {
			if entries[i].Key == canonical {
				entries[i].Value = value
				replaced = true
				break
			}
		}
		if !replaced {
			entries = append(entries, Entry{Key: canonical, Value: value})
		}
		return entries
	})
}

// Unset removes all occurrences of key from the file.
func (f *File) Unset(key string) error {
	section, subsection, variable, err := ParseKey(key)
	if err != nil {
		return err
	}
	canonical := canonicalKey(section, subsection, variable)
	return f.writeAtomic(func(entries []Entry) []Entry {
		out := entries[:0]
		for _, e := range entries {
			if e.Key != canonical {
				out = append(out, e)
			}
		}
		return out
	})
}

// writeAtomic applies transform to the in-memory entries, serialises the
// result to disk atomically (write to .lock → rename), and updates f.entries.
func (f *File) writeAtomic(transform func([]Entry) []Entry) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	lockPath := f.path + ".lock"

	// Preserve the existing file's permissions; default to 0600 for new files
	// so that config files containing sensitive values are not world-readable.
	mode := os.FileMode(0o600)
	if info, err := os.Stat(f.path); err == nil {
		mode = info.Mode()
	}

	newEntries := transform(append([]Entry(nil), f.entries...))

	data := serialise(newEntries)
	if err := os.WriteFile(lockPath, data, mode); err != nil {
		return err
	}
	if err := os.Rename(lockPath, f.path); err != nil {
		_ = os.Remove(lockPath)
		return err
	}
	f.entries = newEntries
	return nil
}

// serialise converts a slice of Entries to INI-format bytes.
// Sections are grouped; within each group, key-value lines are tab-indented.
func serialise(entries []Entry) []byte {
	var buf bytes.Buffer

	type sectionKey struct {
		section    string
		subsection string
	}

	// Preserve insertion order of sections while grouping entries.
	type sectionEntry struct {
		key   sectionKey
		items []Entry
	}

	var order []sectionKey
	groups := map[sectionKey]*sectionEntry{}

	for _, e := range entries {
		sec, sub, _, _ := splitCanonical(e.Key)
		sk := sectionKey{sec, sub}
		if _, ok := groups[sk]; !ok {
			order = append(order, sk)
			groups[sk] = &sectionEntry{key: sk}
		}
		groups[sk].items = append(groups[sk].items, e)
	}

	for _, sk := range order {
		g := groups[sk]
		buf.WriteString(formatSectionHeader(g.key.section, g.key.subsection))
		for _, e := range g.items {
			_, _, variable, _ := splitCanonical(e.Key)
			buf.WriteString(formatKeyValue(variable, e.Value))
		}
	}

	return buf.Bytes()
}

// splitCanonical splits a canonical key "section[.subsection].variable" into
// its three parts using the same last-dot logic as ParseKey.
func splitCanonical(canonical string) (section, subsection, variable string, err error) {
	lastDot := strings.LastIndex(canonical, ".")
	if lastDot < 0 {
		return "", "", "", fmt.Errorf("bad canonical key %q", canonical)
	}
	variable = canonical[lastDot+1:]
	prefix := canonical[:lastDot]
	firstDot := strings.Index(prefix, ".")
	if firstDot < 0 {
		section = prefix
	} else {
		section = prefix[:firstDot]
		subsection = prefix[firstDot+1:]
	}
	return section, subsection, variable, nil
}

// formatSectionHeader formats a section header line.
func formatSectionHeader(section, subsection string) string {
	if subsection == "" {
		return fmt.Sprintf("[%s]\n", section)
	}
	return "[" + section + ` "` + escapeSubsection(subsection) + "\"]\n"
}

// escapeSubsection escapes backslashes and double-quotes in a subsection name.
func escapeSubsection(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// formatKeyValue formats a key = value line with proper quoting.
func formatKeyValue(variable, value string) string {
	return fmt.Sprintf("\t%s = %s\n", variable, quoteValue(value))
}

// quoteValue wraps a value in double-quotes when it contains characters that
// would be misinterpreted by the parser (leading/trailing space, #, ;, \r).
// It also applies backslash escaping inside quoted strings.
func quoteValue(v string) string {
	needsQuote := v != "" && (v[0] == ' ' || v[0] == '\t' || v[len(v)-1] == ' ' || v[len(v)-1] == '\t')
	for _, c := range v {
		if c == '#' || c == ';' || c == '\r' || c == '\n' || c == '\\' || c == '"' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return v
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range v {
		switch c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// ----------------------------------------------------------------------------
// Listing
// ----------------------------------------------------------------------------

// List writes all key=value pairs to w, one per line.
func (f *File) List(w io.Writer) error {
	for _, e := range f.entries {
		if _, err := fmt.Fprintf(w, "%s=%s\n", e.Key, e.Value); err != nil {
			return err
		}
	}
	return nil
}
