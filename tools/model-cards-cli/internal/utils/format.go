package utils

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// FormatParameters formats the parameters to match the table format
func FormatParameters(params string) string {
	// If already formatted with M or B suffix, return as is
	if strings.HasSuffix(params, "M") || strings.HasSuffix(params, "B") {
		return params
	}

	// Try to parse as a number
	num, err := strconv.ParseFloat(params, 64)
	if err != nil {
		return params
	}

	// Format based on size
	if num >= 1000000000 {
		return fmt.Sprintf("%.1fB", num/1000000000)
	} else if num >= 1000000 {
		return fmt.Sprintf("%.0fM", num/1000000)
	}

	return params
}

// FormatVRAM converts bytes to GB and returns a formatted string
// The value is rounded to 2 decimal places
func FormatVRAM(bytes float64) string {
	// Convert bytes to GB (1 GB = 1024^3 bytes)
	gb := bytes / (1024 * 1024 * 1024)

	// Round to 2 decimal places
	rounded := math.Round(gb*100) / 100

	return fmt.Sprintf("%.2f GB", rounded)
}

// FormatContextLength formats a token count with K/M/B suffixes
// For example: 1000 -> "1K", 1500 -> "1.5K", 1000000 -> "1M"
func FormatContextLength(tokens uint32) string {
	const (
		K = 1000
		M = K * 1000
		B = M * 1000
	)

	switch {
	case tokens >= B:
		return fmt.Sprintf("%dB", int(math.Round(float64(tokens)/float64(B))))
	case tokens >= M:
		return fmt.Sprintf("%dM tokens", int(math.Round(float64(tokens)/float64(M))))
	case tokens >= K:
		return fmt.Sprintf("%dK tokens", int(math.Round(float64(tokens)/float64(K))))
	default:
		return fmt.Sprintf("%d tokens", tokens)
	}
}

// FormatSize converts bytes to GB or MB and returns a formatted string
// The value is rounded to 2 decimal places
func FormatSize(bytes uint64) string {
	const (
		MB = 1024 * 1024
		GB = MB * 1024
	)

	// Convert to GB if size is large enough
	if bytes >= GB {
		gb := float64(bytes) / float64(GB)
		rounded := math.Round(gb*100) / 100
		return fmt.Sprintf("%.2f GB", rounded)
	}

	// Otherwise convert to MB
	mb := float64(bytes) / float64(MB)
	rounded := math.Round(mb*100) / 100
	return fmt.Sprintf("%.2f MB", rounded)
}
