package markdown

import (
	"fmt"
	"github.com/docker/model-cards/tools/build-tables/internal/utils"
	"os"
	"regexp"
	"strings"

	"github.com/docker/model-cards/tools/build-tables/internal/domain"
)

// Updater implements the domain.MarkdownUpdater interface
type Updater struct{}

// UpdateModelTable updates the "Available model variants" table in a markdown file
func (u *Updater) UpdateModelTable(filePath string, variants []domain.ModelVariant) error {
	// Read the markdown file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read markdown file: %v", err)
	}

	// Find the "Available model variants" section
	sectionRegex := regexp.MustCompile(`(?m)^## Available model variants\s*$`)
	sectionMatch := sectionRegex.FindIndex(content)
	if sectionMatch == nil {
		return fmt.Errorf("could not find the 'Available model variants' section")
	}

	// Extract the content before the table section
	beforeTable := content[:sectionMatch[1]]

	// Generate the new table
	var latestTag string
	var tableBuilder strings.Builder
	tableBuilder.WriteString("\n")
	tableBuilder.WriteString("| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |\n")
	tableBuilder.WriteString("|---------------|------------|--------------|----------------|------|-------|\n")

	// First, find and add the latest variant if it exists
	for _, variant := range variants {
		if variant.IsLatest() {
			latestTag = variant.GetLatestTag()
			modelVariant := fmt.Sprintf("`%s:latest`<br><br>`%s:%s`", variant.RepoName, variant.RepoName, latestTag)
			row := u.getRow(variant, modelVariant)
			tableBuilder.WriteString(row)
			break
		}
	}

	// Then add the rest of the variants
	for _, variant := range variants {
		if variant.IsLatest() {
			continue
		}
		// For non-latest variants, show all their tags
		modelVariant := fmt.Sprintf("`%s:%s`", variant.RepoName, variant.Tags[0])
		row := u.getRow(variant, modelVariant)
		tableBuilder.WriteString(row)
	}

	// Add the footnote for VRAM estimation
	tableBuilder.WriteString("\n¹: VRAM estimated based on model characteristics.\n")

	// Add the latest tag mapping note if we found a match
	if latestTag != "" {
		tableBuilder.WriteString(fmt.Sprintf("\n> `latest` → `%s`\n\n", latestTag))
	}

	// Find the next section (any ## heading)
	nextSectionRegex := regexp.MustCompile(`(?m)^##\s+[^#]`)
	nextSectionMatch := nextSectionRegex.FindIndex(content[sectionMatch[1]:])

	var afterTable []byte
	if nextSectionMatch != nil {
		// Make a copy of the content to avoid modifying the original
		afterTable = make([]byte, len(content[sectionMatch[1]+nextSectionMatch[0]:]))
		copy(afterTable, content[sectionMatch[1]+nextSectionMatch[0]:])
	} else {
		// Make a copy of the content to avoid modifying the original
		afterTable = make([]byte, len(content[sectionMatch[1]:]))
		copy(afterTable, content[sectionMatch[1]:])
	}

	// Combine the parts with proper spacing
	newContent := append(beforeTable, []byte(tableBuilder.String())...)
	newContent = append(newContent, afterTable...)

	// Write the updated content back to the file
	err = os.WriteFile(filePath, newContent, 0644)
	if err != nil {
		return fmt.Errorf("failed to write updated markdown file: %v", err)
	}

	return nil
}

func (u *Updater) getRow(variant domain.ModelVariant, modelVariant string) string {
	parameters := utils.FormatParameters(variant.Parameters)
	contextWindow := utils.FormatContextLength(variant.ContextLength)
	size := utils.FormatSize(variant.Size)
	vram := utils.FormatVRAM(variant.VRAM)
	row := fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
		modelVariant,
		parameters,
		variant.Quantization,
		contextWindow,
		vram,
		size)
	return row
}
