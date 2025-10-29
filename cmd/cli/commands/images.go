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
	// Validate that this is an image file using both content and extension
	if !isImageFileByContentAndExtension(buf, filePath) {
		return "", fmt.Errorf("invalid image type for file: %s", filePath)
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

// extractFileInclusions finds file paths in the prompt text using the @ symbol
// e.g., @filename.txt, @./path/to/file.txt, @/absolute/path/file.txt
func extractFileInclusions(prompt string) []string {
	// Regex to match @ followed by a file path
	// Pattern explanation:
	// - @ symbol before the path
	// - File path can be quoted (at least one char) or unquoted (at least one char)
	// - Supports relative (./, ../) and absolute paths
	regexPattern := `@(?:"([^"]+)"|'([^']+)'|([^\s"']+))`
	re := regexp.MustCompile(regexPattern)
	matches := re.FindAllStringSubmatch(prompt, -1)

	paths := []string{}
	for _, match := range matches {
		// match[0] is the full match
		// match[1] is double-quoted content
		// match[2] is single-quoted content
		// match[3] is unquoted content
		if len(match) >= 2 && match[1] != "" {
			paths = append(paths, match[1])
		} else if len(match) >= 3 && match[2] != "" {
			paths = append(paths, match[2])
		} else if len(match) >= 4 && match[3] != "" {
			paths = append(paths, match[3])
		}
	}

	return paths
}

// hasValidImageExtension checks if the file has a valid image extension
func hasValidImageExtension(filePath string) bool {
	filePathLower := strings.ToLower(filePath)
	return strings.HasSuffix(filePathLower, ".jpg") ||
		strings.HasSuffix(filePathLower, ".jpeg") ||
		strings.HasSuffix(filePathLower, ".png") ||
		strings.HasSuffix(filePathLower, ".webp")
}

// isImageFileByContentAndExtension checks if a file is an image using both file extension and content type detection
func isImageFileByContentAndExtension(content []byte, filePath string) bool {
	// First check content type
	contentType := http.DetectContentType(content)
	allowedTypes := []string{"image/jpeg", "image/jpg", "image/png", "image/webp"}
	if slices.Contains(allowedTypes, contentType) {
		return true
	}

	// Fallback to file extension check if content detection fails
	return hasValidImageExtension(filePath)
}

// processFileInclusions extracts files mentioned with @ symbol, reads their contents,
// and returns the prompt with file contents embedded
func processFileInclusions(prompt string) (string, error) {
	filePaths := extractFileInclusions(prompt)

	// Process each file inclusion in order
	for _, filePath := range filePaths {
		nfp := normalizeFilePath(filePath)

		// Read the file content
		content, err := os.ReadFile(nfp)
		if err != nil {
			// Skip non-existent files or files that can't be read
			continue
		}

		// Check if the file is an image to handle it appropriately
		// Only content is checked here, not file extension, to maintain original behavior
		if isImageFileByContentAndExtension(content, nfp) {
			// For image files, we keep the original file reference (@filePath) in the prompt
			// so that processImagesInPrompt can handle it properly
			continue
		}

		// For non-image files, replace the @filename with the file content
		// Try different variations to match how it appears in the prompt
		escapedPath := regexp.QuoteMeta(filePath) // Escape special regex chars
		quotedPath := regexp.QuoteMeta(nfp)       // Also try normalized path

		// Replace all occurrences of the file reference with its content
		contentStr := string(content)

		// Replace @ symbol usage with the file content - preserve the space or end by using word boundaries
		// Use capturing groups to preserve the space
		prompt = regexp.MustCompile(`@`+`"`+escapedPath+`"`).ReplaceAllString(prompt, contentStr)
		prompt = regexp.MustCompile(`@`+`'`+escapedPath+`'`).ReplaceAllString(prompt, contentStr)
		// For the unquoted version, we need to match @ + path and replace with content + the boundary character
		// Use a more specific pattern that captures the space or end
		prompt = regexp.MustCompile(`(@`+escapedPath+`)(\s|$)`).ReplaceAllString(prompt, contentStr+"$2")

		// Also try replacing with normalized path
		prompt = regexp.MustCompile(`@`+`"`+quotedPath+`"`).ReplaceAllString(prompt, contentStr)
		prompt = regexp.MustCompile(`@`+`'`+quotedPath+`'`).ReplaceAllString(prompt, contentStr)
		prompt = regexp.MustCompile(`(@`+quotedPath+`)(\s|$)`).ReplaceAllString(prompt, contentStr+"$2")
	}

	return prompt, nil
}

// For testing purposes - making the functions public so they can be tested
func ExtractFileInclusions(prompt string) []string {
	return extractFileInclusions(prompt)
}

func ProcessFileInclusions(prompt string) (string, error) {
	return processFileInclusions(prompt)
}
