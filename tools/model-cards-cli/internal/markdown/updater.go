package markdown

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/model-cards/tools/build-tables/internal/utils"

	"github.com/docker/model-cards/tools/build-tables/internal/domain"
)

// Updater implements the domain.MarkdownUpdater interface
type Updater struct{}

// Define the sort order for each quantization type
var quantizationOrder = map[string]int{
	"Q2_K": 0,
	"Q3_K": 1,
	"Q4_0": 2,
	"Q4_1": 3,
	"Q4_K": 4,
	"Q5_0": 5,
	"Q5_1": 6,
	"Q5_K": 7,
	"Q6_K": 8,
	"Q8_0": 9,
	"I16":  10,
	"I32":  11,
	"I64":  12,
	"F16":  13,
	"BF16": 14,
	"F32":  15,
	"F64":  16,
}

// Sort suffixes (if needed, you can customize this)
var suffixOrder = map[string]int{
	"":   0, // no suffix
	"_S": 1,
	"_M": 2,
	"_G": 3,
}

// parseWeight converts a weight string (e.g., "12B", "7M") to a float64
func (u *Updater) parseWeight(weight string) (float64, error) {
	// Remove any non-numeric characters except decimal point
	toParse := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' {
			return r
		}
		return -1
	}, weight)

	// Parse the number
	value, err := strconv.ParseFloat(toParse, 64)
	if err != nil {
		return 0, err
	}

	// Convert to actual number based on unit
	if strings.Contains(strings.ToUpper(weight), "B") {
		value *= 1e9 // billions
	} else if strings.Contains(strings.ToUpper(weight), "M") {
		value *= 1e6 // millions
	}

	return value, nil
}

func getSortKey(tag string) int {
	re := regexp.MustCompile(`^([A-Z0-9_]+?)(_[A-Z])?$`)
	matches := re.FindStringSubmatch(tag)

	if len(matches) == 3 {
		base := matches[1]
		suffix := matches[2] // may be empty

		baseRank, baseExists := quantizationOrder[base]
		suffixRank, suffixExists := suffixOrder[suffix]

		if baseExists {
			if !suffixExists {
				suffixRank = 99 // unknown suffix gets low priority
			}
			return baseRank*10 + suffixRank
		}
	}

	return 1000 // completely unknown tag
}

// sortVariants sorts variants by weights and quantization
func (u *Updater) sortVariants(variants []domain.ModelVariant) []domain.ModelVariant {
	// Create a copy of the variants slice to avoid modifying the original
	sortedVariants := make([]domain.ModelVariant, len(variants))
	copy(sortedVariants, variants)

	sort.Slice(sortedVariants, func(i, j int) bool {
		// Get the first tag for each variant
		tagI := sortedVariants[i].Tags[0]
		tagJ := sortedVariants[j].Tags[0]

		// Split tags into weights and quantization
		partsI := strings.Split(tagI, "-")
		partsJ := strings.Split(tagJ, "-")

		if len(partsI) != 2 || len(partsJ) != 2 {
			return tagI < tagJ // Fallback to string comparison if format is unexpected
		}

		// Compare weights
		weightI, errI := u.parseWeight(partsI[0])
		weightJ, errJ := u.parseWeight(partsJ[0])

		if errI != nil || errJ != nil {
			return tagI < tagJ // Fallback to string comparison if parsing fails
		}

		if weightI != weightJ {
			return weightI < weightJ // Sort by weights ascending
		}

		// If weights are equal, sort by quantization
		quantI := getSortKey(partsI[1])
		quantJ := getSortKey(partsJ[1])
		return quantI < quantJ // Sort by quantization ascending
	})

	return sortedVariants
}

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

	// Sort all variants first
	sortedVariants := u.sortVariants(variants)

	// Generate the new table

	// Compute display repository using default namespace "ai" and file basename
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	displayRepo := fmt.Sprintf("ai/%s", base)

	var latestVariant *domain.ModelVariant
	var latestTag string
	var tableBuilder strings.Builder
	tableBuilder.WriteString("\n")
	tableBuilder.WriteString("| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |\n")
	tableBuilder.WriteString("|---------------|------------|--------------|----------------|------|-------|\n")

	// First, find and add the latest variant if it exists
	for i, variant := range variants {
		if variant.IsLatest() {
			latestVariant = &variants[i]
			// Build the model variant string with ALL tags
			var modelVariantStr string
			for j, tag := range variant.Tags {
				if j == 0 {
					modelVariantStr = fmt.Sprintf("`%s:%s`", displayRepo, tag)
				} else {
					modelVariantStr += fmt.Sprintf("<br><br>`%s:%s`", displayRepo, tag)
				}
			}
			row := u.getRow(variant, modelVariantStr)
			tableBuilder.WriteString(row)

			// Get the first non-latest tag for the mapping note
			latestTag = variant.GetLatestTag()
			break
		}
	}

	// Then add the rest of the variants in sorted order (excluding the entire latest variant)
	for _, variant := range sortedVariants {
		// Skip if this is the same variant as the latest one (compare all properties)
		if latestVariant != nil && variant.IsLatest() {
			continue
		}
		modelVariant := fmt.Sprintf("`%s:%s`", displayRepo, variant.Tags[0])
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
