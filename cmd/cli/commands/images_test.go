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
		{
			name:     "path with spaces in double quotes",
			input:    "Describe this image \"/path/to/my file.jpg\"",
			expected: []string{"/path/to/my file.jpg"},
		},
		{
			name:     "path with spaces in single quotes",
			input:    "What's in '/Users/photos/vacation 2023/photo.png'?",
			expected: []string{"/Users/photos/vacation 2023/photo.png"},
		},
		{
			name:     "multiple paths with spaces",
			input:    "Compare \"/path/my file.jpg\" and '/another path/image.png'",
			expected: []string{"/path/my file.jpg", "/another path/image.png"},
		},
		{
			name:     "Windows path with spaces",
			input:    "Analyze \"C:\\Users\\My Documents\\photo.jpg\"",
			expected: []string{"C:\\Users\\My Documents\\photo.jpg"},
		},
		{
			name:     "mixed quoted and unquoted paths",
			input:    "Compare /simple/path.jpg with \"/path with spaces/image.png\"",
			expected: []string{"/simple/path.jpg", "/path with spaces/image.png"},
		},
		{
			name:     "unquoted path with spaces",
			input:    "What's in this image? /Users/ilopezluna/Documents/some thing.jpg",
			expected: []string{"/Users/ilopezluna/Documents/some thing.jpg"},
		},
		{
			name:     "unquoted Windows path with spaces",
			input:    "Analyze C:\\Users\\My Documents\\photo.jpg",
			expected: []string{"C:\\Users\\My Documents\\photo.jpg"},
		},
		{
			name:     "multiple unquoted paths with spaces",
			input:    "Compare /path/my file.jpg and C:\\Users\\My Photos\\image.png",
			expected: []string{"/path/my file.jpg", "C:\\Users\\My Photos\\image.png"},
		},
		{
			name:     "path followed by period",
			input:    "Look at /Users/test/some image.jpg. What do you see?",
			expected: []string{"/Users/test/some image.jpg"},
		},
		{
			name:     "path followed by comma",
			input:    "Check /path/my photo.png, it's interesting",
			expected: []string{"/path/my photo.png"},
		},
		{
			name:     "path followed by exclamation",
			input:    "Amazing shot at /photos/vacation 2024/sunset.jpg!",
			expected: []string{"/photos/vacation 2024/sunset.jpg"},
		},
		{
			name:     "path followed by question mark",
			input:    "What's in /docs/my file.jpeg?",
			expected: []string{"/docs/my file.jpeg"},
		},
		{
			name:     "path in middle of sentence",
			input:    "I found /path/test image.jpg and it looks great",
			expected: []string{"/path/test image.jpg"},
		},
		{
			name:     "path with newline after",
			input:    "Check this /path/photo.jpg\nAnd tell me what you think",
			expected: []string{"/path/photo.jpg"},
		},
		{
			name:     "multiple extensions in text",
			input:    "The file.jpg format is used in /path/image.jpg here",
			expected: []string{"/path/image.jpg"},
		},
		{
			name:     "path with dots in filename",
			input:    "Check /path/my image.v2.final.jpg for details",
			expected: []string{"/path/my image.v2.final.jpg"},
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

func TestPromptCleaning(t *testing.T) {
	// Create temporary test images
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

	img1Path := filepath.Join(tmpDir, "img1.jpg")
	img2Path := filepath.Join(tmpDir, "img2.png")
	img3Path := filepath.Join(tmpDir, "img3.webp")

	for _, path := range []string{img1Path, img2Path, img3Path} {
		if err := os.WriteFile(path, jpegData, 0644); err != nil {
			t.Fatalf("failed to create test image %s: %v", path, err)
		}
	}

	tests := []struct {
		name           string
		input          string
		expectedPrompt string
		expectedImages int
	}{
		{
			name:           "single image at end",
			input:          fmt.Sprintf("Describe this image %s", img1Path),
			expectedPrompt: "Describe this image",
			expectedImages: 1,
		},
		{
			name:           "single image at beginning",
			input:          fmt.Sprintf("%s What do you see?", img1Path),
			expectedPrompt: "What do you see?",
			expectedImages: 1,
		},
		{
			name:           "single image in middle",
			input:          fmt.Sprintf("Look at %s and describe it", img1Path),
			expectedPrompt: "Look at  and describe it",
			expectedImages: 1,
		},
		{
			name:           "multiple images",
			input:          fmt.Sprintf("Compare %s and %s", img1Path, img2Path),
			expectedPrompt: "Compare  and",
			expectedImages: 2,
		},
		{
			name:           "three images with text",
			input:          fmt.Sprintf("Analyze %s, %s, and %s carefully", img1Path, img2Path, img3Path),
			expectedPrompt: "Analyze , , and  carefully",
			expectedImages: 3,
		},
		{
			name:           "image with quotes",
			input:          fmt.Sprintf("What's in '%s'?", img1Path),
			expectedPrompt: "What's in ?",
			expectedImages: 1,
		},
		{
			name:           "only image path",
			input:          img1Path,
			expectedPrompt: "",
			expectedImages: 1,
		},
		{
			name:           "image at start and end",
			input:          fmt.Sprintf("%s Compare these %s", img1Path, img2Path),
			expectedPrompt: "Compare these",
			expectedImages: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanedPrompt, imageURLs, err := processImagesInPrompt(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cleanedPrompt != tt.expectedPrompt {
				t.Errorf("expected cleaned prompt %q, got %q", tt.expectedPrompt, cleanedPrompt)
			}

			if len(imageURLs) != tt.expectedImages {
				t.Errorf("expected %d images, got %d", tt.expectedImages, len(imageURLs))
			}

			// Verify all image URLs are valid data URLs
			for i, url := range imageURLs {
				if !strings.HasPrefix(url, "data:image/") {
					t.Errorf("image URL %d doesn't start with 'data:image/', got: %s", i, url[:30])
				}
			}
		})
	}
}
