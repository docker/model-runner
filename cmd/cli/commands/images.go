package commands

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
)

// MaxImageSizeBytes is the maximum allowed size for image files (100MB)
const MaxImageSizeBytes int64 = 100 * 1024 * 1024

// extractImagePaths finds image file paths in the input string using regex
// Matches paths like: /path/to/file.jpg, ./image.png, C:\photos\pic.webp
// Also handles quoted paths with spaces: "/path/to/my file.jpg" or '/path/with spaces/image.png'
// Unquoted paths with spaces are also supported for better UX
func extractImagePaths(input string) []string {
	// Regex to match file paths:
	// - Quoted paths (double or single quotes) - can contain spaces
	// - Unix absolute: /path/to/file.jpg (with or without spaces)
	// - Unix relative: ./path/to/file.jpg (with or without spaces)
	// - Windows absolute: C:\path\to\file.jpg or C:/path/to/file.jpg (with or without spaces)
	// - Windows relative: \path\to\file.jpg (with or without spaces)
	// Pattern explanation:
	//   1. "([^"]+?\.(?i:jpg|jpeg|png|webp))" - Double-quoted paths
	//   2. '([^']+?\.(?i:jpg|jpeg|png|webp))' - Single-quoted paths
	//   3. (?:[a-zA-Z]:[/\\]|[/\\]|\./)[^\n"']*?\.(?i:jpg|jpeg|png|webp)\b - Unquoted paths
	//      - Matches from path start to image extension
	//      - [^\n"'] allows spaces but stops at newlines or quotes
	//      - Non-greedy *? ensures we stop at first valid extension
	//      - \b word boundary ensures clean extension match
	regexPattern := `"([^"]+?\.(?i:jpg|jpeg|png|webp))"|'([^']+?\.(?i:jpg|jpeg|png|webp))'|(?:[a-zA-Z]:[/\\]|[/\\]|\./)[^\n"']*?\.(?i:jpg|jpeg|png|webp)\b`
	re := regexp.MustCompile(regexPattern)
	matches := re.FindAllStringSubmatch(input, -1)

	var paths []string
	for _, match := range matches {
		// match[0] is the full match
		// match[1] is the double-quoted path (if matched)
		// match[2] is the single-quoted path (if matched)
		// If neither capture group matched, match[0] is the unquoted path
		if match[1] != "" {
			paths = append(paths, match[1]) // double-quoted path
		} else if match[2] != "" {
			paths = append(paths, match[2]) // single-quoted path
		} else {
			paths = append(paths, match[0]) // unquoted path
		}
	}

	return paths
}

// normalizeFilePath handles escaped characters in file paths
// Converts escaped spaces, parentheses, brackets, etc. to their literal form
func normalizeFilePath(filePath string) string {
	return strings.NewReplacer(
		"\\ ", " ", // Escaped space
		"\\(", "(", // Escaped left parenthesis
		"\\)", ")", // Escaped right parenthesis
		"\\[", "[", // Escaped left square bracket
		"\\]", "]", // Escaped right square bracket
		"\\{", "{", // Escaped left curly brace
		"\\}", "}", // Escaped right curly brace
		"\\$", "$", // Escaped dollar sign
		"\\&", "&", // Escaped ampersand
		"\\;", ";", // Escaped semicolon
		"\\'", "'", // Escaped single quote
		"\\\\", "\\", // Escaped backslash
		"\\*", "*", // Escaped asterisk
		"\\?", "?", // Escaped question mark
		"\\~", "~", // Escaped tilde
	).Replace(filePath)
}

// encodeImageToDataURL reads an image file, validates it, and encodes it to a base64 data URL
// Returns a data URL like: data:image/jpeg;base64,/9j/4AAQ...
func encodeImageToDataURL(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read first 512 bytes to detect content type
	buf := make([]byte, 512)
	_, err = file.Read(buf)
	if err != nil {
		return "", err
	}

	contentType := http.DetectContentType(buf)
	allowedTypes := []string{"image/jpeg", "image/png", "image/webp"}
	if !slices.Contains(allowedTypes, contentType) {
		return "", fmt.Errorf("invalid image type: %s", contentType)
	}

	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	// Check if the file size exceeds the maximum limit
	if info.Size() > MaxImageSizeBytes {
		return "", fmt.Errorf("file size exceeds maximum limit (%d MB)", MaxImageSizeBytes/(1024*1024))
	}

	// Read entire file
	buf = make([]byte, info.Size())
	_, err = file.Seek(0, 0)
	if err != nil {
		return "", err
	}

	_, err = io.ReadFull(file, buf)
	if err != nil {
		return "", err
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(buf)

	// Create data URL
	dataURL := fmt.Sprintf("data:%s;base64,%s", contentType, encoded)

	return dataURL, nil
}

// processImagesInPrompt extracts images from the prompt, encodes them to data URLs,
// and returns the cleaned prompt text and list of image data URLs
func processImagesInPrompt(prompt string) (string, []string, error) {
	imagePaths := extractImagePaths(prompt)
	var imageDataURLs []string

	for _, filePath := range imagePaths {
		nfp := normalizeFilePath(filePath)
		dataURL, err := encodeImageToDataURL(nfp)
		if errors.Is(err, os.ErrNotExist) {
			// Skip non-existent files (might be false positive from regex)
			continue
		} else if err != nil {
			return "", nil, fmt.Errorf("couldn't process image %q: %w", nfp, err)
		}

		// Remove the image path from the prompt text
		prompt = strings.ReplaceAll(prompt, "'"+nfp+"'", "")
		prompt = strings.ReplaceAll(prompt, "'"+filePath+"'", "")
		prompt = strings.ReplaceAll(prompt, nfp, "")
		prompt = strings.ReplaceAll(prompt, filePath, "")

		imageDataURLs = append(imageDataURLs, dataURL)
	}

	return strings.TrimSpace(prompt), imageDataURLs, nil
}
