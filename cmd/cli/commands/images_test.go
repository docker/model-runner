package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractImagePaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single absolute path",
			input:    "Describe this image /path/to/image.jpg",
			expected: []string{"/path/to/image.jpg"},
		},
		{
			name:     "multiple images",
			input:    "Compare /path/to/first.png and /path/to/second.jpeg",
			expected: []string{"/path/to/first.png", "/path/to/second.jpeg"},
		},
		{
			name:     "relative path",
			input:    "What's in ./photo.webp?",
			expected: []string{"./photo.webp"},
		},
		{
			name:     "Windows path",
			input:    "Analyze C:\\Users\\photos\\pic.jpg",
			expected: []string{"C:\\Users\\photos\\pic.jpg"},
		},
		{
			name:     "no images",
			input:    "Just a regular prompt without images",
			expected: nil,
		},
		{
			name:     "mixed case extensions",
			input:    "Check /path/image.JPG and /path/photo.Png",
			expected: []string{"/path/image.JPG", "/path/photo.Png"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractImagePaths(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d paths, got %d", len(tt.expected), len(result))
				return
			}
			for i, path := range result {
				if path != tt.expected[i] {
					t.Errorf("expected path %q, got %q", tt.expected[i], path)
				}
			}
		})
	}
}

func TestNormalizeFilePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "escaped space",
			input:    "/path/to/my\\ file.jpg",
			expected: "/path/to/my file.jpg",
		},
		{
			name:     "escaped parentheses",
			input:    "/path/to/file\\(1\\).jpg",
			expected: "/path/to/file(1).jpg",
		},
		{
			name:     "multiple escaped chars",
			input:    "/path/to/my\\ file\\(2\\).jpg",
			expected: "/path/to/my file(2).jpg",
		},
		{
			name:     "no escapes",
			input:    "/path/to/file.jpg",
			expected: "/path/to/file.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeFilePath(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestEncodeImageToDataURL(t *testing.T) {
	// Create a temporary test image file
	tmpDir := t.TempDir()

	// Create a minimal valid JPEG (1x1 pixel)
	jpegData := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
		0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
		0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08,
		0x07, 0x07, 0x07, 0x09, 0x09, 0x08, 0x0A, 0x0C,
		0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D,
		0x1A, 0x1C, 0x1C, 0x20, 0x24, 0x2E, 0x27, 0x20,
		0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27,
		0x39, 0x3D, 0x38, 0x32, 0x3C, 0x2E, 0x33, 0x34,
		0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4,
		0x00, 0x14, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x03, 0xFF, 0xC4, 0x00, 0x14,
		0x10, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
		0x00, 0x00, 0x3F, 0x00, 0x37, 0xFF, 0xD9,
	}

	jpegPath := filepath.Join(tmpDir, "test.jpg")
	err := os.WriteFile(jpegPath, jpegData, 0644)
	if err != nil {
		t.Fatalf("failed to create test image: %v", err)
	}

	t.Run("valid jpeg", func(t *testing.T) {
		dataURL, err := encodeImageToDataURL(jpegPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(dataURL, "data:image/jpeg;base64,") {
			t.Errorf("expected data URL to start with 'data:image/jpeg;base64,', got %s", dataURL[:30])
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := encodeImageToDataURL("/non/existent/file.jpg")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("invalid file type", func(t *testing.T) {
		txtPath := filepath.Join(tmpDir, "test.txt")
		err := os.WriteFile(txtPath, []byte("not an image"), 0644)
		if err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		_, err = encodeImageToDataURL(txtPath)
		if err == nil {
			t.Error("expected error for invalid file type")
		}
		if !strings.Contains(err.Error(), "invalid image type") {
			t.Errorf("expected 'invalid image type' error, got: %v", err)
		}
	})
}

func TestProcessImagesInPrompt(t *testing.T) {
	// Create a temporary test image
	tmpDir := t.TempDir()
	jpegData := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
		0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
		0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08,
		0x07, 0x07, 0x07, 0x09, 0x09, 0x08, 0x0A, 0x0C,
		0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D,
		0x1A, 0x1C, 0x1C, 0x20, 0x24, 0x2E, 0x27, 0x20,
		0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27,
		0x39, 0x3D, 0x38, 0x32, 0x3C, 0x2E, 0x33, 0x34,
		0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4,
		0x00, 0x14, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x03, 0xFF, 0xC4, 0x00, 0x14,
		0x10, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
		0x00, 0x00, 0x3F, 0x00, 0x37, 0xFF, 0xD9,
	}

	jpegPath := filepath.Join(tmpDir, "test.jpg")
	err := os.WriteFile(jpegPath, jpegData, 0644)
	if err != nil {
		t.Fatalf("failed to create test image: %v", err)
	}

	t.Run("prompt with valid image", func(t *testing.T) {
		prompt := fmt.Sprintf("Describe this image %s", jpegPath)
		cleanedPrompt, imageURLs, err := processImagesInPrompt(prompt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cleanedPrompt != "Describe this image" {
			t.Errorf("expected cleaned prompt 'Describe this image', got %q", cleanedPrompt)
		}

		if len(imageURLs) != 1 {
			t.Errorf("expected 1 image URL, got %d", len(imageURLs))
		}

		if len(imageURLs) > 0 && !strings.HasPrefix(imageURLs[0], "data:image/jpeg;base64,") {
			t.Errorf("expected data URL to start with 'data:image/jpeg;base64,'")
		}
	})

	t.Run("prompt without images", func(t *testing.T) {
		prompt := "Just a regular prompt"
		cleanedPrompt, imageURLs, err := processImagesInPrompt(prompt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cleanedPrompt != prompt {
			t.Errorf("expected prompt unchanged, got %q", cleanedPrompt)
		}

		if len(imageURLs) != 0 {
			t.Errorf("expected 0 image URLs, got %d", len(imageURLs))
		}
	})

	t.Run("prompt with non-existent image", func(t *testing.T) {
		prompt := "Describe this image /non/existent/image.jpg"
		cleanedPrompt, imageURLs, err := processImagesInPrompt(prompt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Non-existent files should be skipped
		if len(imageURLs) != 0 {
			t.Errorf("expected 0 image URLs for non-existent file, got %d", len(imageURLs))
		}

		// Prompt should still contain the path since file wasn't found
		if !strings.Contains(cleanedPrompt, "/non/existent/image.jpg") {
			t.Errorf("expected prompt to still contain non-existent path")
		}
	})
}
