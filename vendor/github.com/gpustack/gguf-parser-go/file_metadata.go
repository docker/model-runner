package gguf_parser

import (
	"regexp"
	"sort"
	"strings"

	"golang.org/x/exp/maps"
)

// GGUFMetadata represents the model metadata of a GGUF file.
type GGUFMetadata struct {
	/* Basic */

	// Type describes what type this GGUF file is,
	// default is "model".
	Type string `json:"type"`
	// Architecture describes what architecture this GGUF file implements.
	//
	// All lowercase ASCII.
	Architecture string `json:"architecture"`
	// QuantizationVersion describes the version of the quantization format.
	//
	// Not required if the model is not quantized (i.e. no tensors are quantized).
	// If any tensors are quantized, this must be present.
	// This is separate to the quantization scheme of the tensors itself,
	// the quantization version may change without changing the scheme's name,
	// e.g. the quantization scheme is Q5_K, and the QuantizationVersion is 4.
	QuantizationVersion uint32 `json:"quantizationVersion,omitempty"`
	// Alignment describes the alignment of the GGUF file.
	//
	// This can vary to allow for different alignment schemes, but it must be a multiple of 8.
	// Some writers may not write the alignment.
	//
	// Default is 32.
	Alignment uint32 `json:"alignment"`
	// Name to the model.
	//
	// This should be a human-readable name that can be used to identify the GGUF file.
	// It should be unique within the community that the model is defined in.
	Name string `json:"name,omitempty"`
	// Author to the model.
	Author string `json:"author,omitempty"`
	// URL to the model's homepage.
	//
	// This can be a GitHub repo, a paper, etc.
	URL string `json:"url,omitempty"`
	// Description to the model.
	Description string `json:"description,omitempty"`
	// License to the model.
	//
	// This is expressed as a SPDX license expression, e.g. "MIT OR Apache-2.0".
	License string `json:"license,omitempty"`
	// FileType describes the type of the majority of the tensors in the GGUF file.
	FileType GGUFFileType `json:"fileType"`

	/* Appendix */

	// LittleEndian is true if the GGUF file is little-endian,
	// and false for big-endian.
	LittleEndian bool `json:"littleEndian"`
	// FileSize is the size of the GGUF file in bytes.
	FileSize GGUFBytesScalar `json:"fileSize"`
	// Size is the model size.
	Size GGUFBytesScalar `json:"size"`
	// Parameters is the parameters of the GGUF file.
	Parameters GGUFParametersScalar `json:"parameters"`
	// BitsPerWeight is the bits per weight of the GGUF file.
	BitsPerWeight GGUFBitsPerWeightScalar `json:"bitsPerWeight"`
}

// GGUFFileType is a type of GGUF file,
// see https://github.com/ggerganov/llama.cpp/blob/278d0e18469aacf505be18ce790a63c7cc31be26/ggml/include/ggml.h#L404-L433.
type GGUFFileType uint32

// GGUFFileType constants.
//
// GGUFFileTypeMostlyQ4_2, GGUFFileTypeMostlyQ4_3 are deprecated.
//
// GGUFFileTypeMostlyQ4_1_F16 is a special case where the majority of the tensors are Q4_1,
// but 'token_embd.weight' and 'output.weight' tensors are F16.
const (
	GGUFFileTypeAllF32           GGUFFileType = iota // F32
	GGUFFileTypeMostlyF16                            // F16
	GGUFFileTypeMostlyQ4_0                           // Q4_0
	GGUFFileTypeMostlyQ4_1                           // Q4_1
	GGUFFileTypeMostlyQ4_1_F16                       // Q4_1_F16
	GGUFFileTypeMostlyQ4_2                           // Q4_2
	GGUFFileTypeMostlyQ4_3                           // Q4_3
	GGUFFileTypeMostlyQ8_0                           // Q8_0
	GGUFFileTypeMostlyQ5_0                           // Q5_0
	GGUFFileTypeMostlyQ5_1                           // Q5_1
	GGUFFileTypeMostlyQ2_K                           // Q2_K
	GGUFFileTypeMostlyQ3_K                           // Q3_K/Q3_K_S
	GGUFFileTypeMostlyQ4_K                           // Q4_K/Q3_K_M
	GGUFFileTypeMostlyQ5_K                           // Q5_K/Q3_K_L
	GGUFFileTypeMostlyQ6_K                           // Q6_K/Q4_K_S
	GGUFFileTypeMostlyIQ2_XXS                        // IQ2_XXS/Q4_K_M
	GGUFFileTypeMostlyIQ2_XS                         // IQ2_XS/Q5_K_S
	GGUFFileTypeMostlyIQ3_XXS                        // IQ3_XXS/Q5_K_M
	GGUFFileTypeMostlyIQ1_S                          // IQ1_S/Q6_K
	GGUFFileTypeMostlyIQ4_NL                         // IQ4_NL
	GGUFFileTypeMostlyIQ3_S                          // IQ3_S
	GGUFFileTypeMostlyIQ2_S                          // IQ2_S
	GGUFFileTypeMostlyIQ4_XS                         // IQ4_XS
	GGUFFileTypeMostlyIQ1_M                          // IQ1_M
	GGUFFileTypeMostlyBF16                           // BF16
	GGUFFileTypeMostlyQ4_0_4_4                       // Q4_0_4x4
	GGUFFileTypeMostlyQ4_0_4_8                       // Q4_0_4x8
	GGUFFileTypeMostlyQ4_0_8_8                       // Q4_0_8x8
	GGUFFileTypeMostlyTQ1_0                          // TQ1_0
	GGUFFileTypeMostlyTQ2_0                          // TQ2_0
	GGUFFileTypeMostlyIQ4_NL_4_4                     // IQ4_NL_4x4
	GGUFFileTypeMostlyIQ4_NL_4_8                     // IQ4_NL_4x8
	GGUFFileTypeMostlyIQ4_NL_8_8                     // IQ4_NL_8x8
	_GGUFFileTypeCount                               // Unknown
)

// Metadata returns the metadata of the GGUF file.
func (gf *GGUFFile) Metadata() (gm GGUFMetadata) {
	const (
		typeKey         = "general.type"
		architectureKey = "general.architecture"
		quantizationKey = "general.quantization_version"
		alignmentKey    = "general.alignment"
		nameKey         = "general.name"
		authorKey       = "general.author"
		urlKey          = "general.url"
		descriptionKey  = "general.description"
		licenseKey      = "general.license"
		fileTypeKey     = "general.file_type"

		controlVectorModelHintKey = "controlvector.model_hint"
	)

	gm.FileType = _GGUFFileTypeCount

	m, _ := gf.Header.MetadataKV.Index([]string{
		typeKey,
		architectureKey,
		quantizationKey,
		alignmentKey,
		nameKey,
		authorKey,
		urlKey,
		descriptionKey,
		licenseKey,
		fileTypeKey,
		controlVectorModelHintKey,
	})

	if v, ok := m[typeKey]; ok {
		gm.Type = v.ValueString()
	} else if _, ok = m[controlVectorModelHintKey]; ok {
		gm.Type = "adapter"
	} else {
		gm.Type = "model"
	}
	if v, ok := m[controlVectorModelHintKey]; ok {
		gm.Architecture = v.ValueString()
	} else if v, ok = m[architectureKey]; ok {
		gm.Architecture = v.ValueString()
		if gm.Architecture == "clip" {
			gm.Type = "projector"
		}
	} else {
		if gf.TensorInfos.Match(regexp.MustCompile(`^model\.diffusion_model\..*`)) ||
			gf.TensorInfos.Match(regexp.MustCompile(`^double_blocks\..*`)) {
			gm.Architecture = "diffusion"
		} else {
			gm.Architecture = "llama"
		}
	}
	if v, ok := m[quantizationKey]; ok {
		gm.QuantizationVersion = ValueNumeric[uint32](v)
	}
	if v, ok := m[alignmentKey]; ok {
		gm.Alignment = ValueNumeric[uint32](v)
	} else {
		gm.Alignment = 32
	}
	if v, ok := m[nameKey]; ok {
		gm.Name = v.ValueString()
	}
	if v, ok := m[authorKey]; ok {
		gm.Author = v.ValueString()
	}
	if v, ok := m[urlKey]; ok {
		gm.URL = v.ValueString()
	}
	if v, ok := m[descriptionKey]; ok {
		gm.Description = v.ValueString()
	}
	if v, ok := m[licenseKey]; ok {
		gm.License = v.ValueString()
	}
	if v, ok := m[fileTypeKey]; ok {
		gm.FileType = GGUFFileType(ValueNumeric[uint32](v))
	}

	if gm.FileType >= _GGUFFileTypeCount {
		gm.FileType = gf.guessFileType(gm.Architecture)
	}

	gm.LittleEndian = gf.Header.Version < GGUFVersionV3 || gf.Header.Magic == GGUFMagicGGUFLe
	gm.FileSize = gf.Size
	gm.Size = gf.ModelSize
	gm.Parameters = gf.ModelParameters
	gm.BitsPerWeight = gf.ModelBitsPerWeight

	return gm
}

// GGMLType returns the GGMLType of the GGUFFileType,
// which is inspired by
// https://github.com/ggerganov/ggml/blob/a10a8b880c059b3b29356eb9a9f8df72f03cdb6a/src/ggml.c#L2730-L2763.
func (t GGUFFileType) GGMLType() GGMLType {
	switch t {
	case GGUFFileTypeAllF32:
		return GGMLTypeF32
	case GGUFFileTypeMostlyF16:
		return GGMLTypeF16
	case GGUFFileTypeMostlyQ4_0:
		return GGMLTypeQ4_0
	case GGUFFileTypeMostlyQ4_1:
		return GGMLTypeQ4_1
	case GGUFFileTypeMostlyQ4_2:
		return GGMLTypeQ4_2
	case GGUFFileTypeMostlyQ4_3:
		return GGMLTypeQ4_3
	case GGUFFileTypeMostlyQ8_0:
		return GGMLTypeQ8_0
	case GGUFFileTypeMostlyQ5_0:
		return GGMLTypeQ5_0
	case GGUFFileTypeMostlyQ5_1:
		return GGMLTypeQ5_1
	case GGUFFileTypeMostlyQ2_K:
		return GGMLTypeQ2_K
	case GGUFFileTypeMostlyQ3_K:
		return GGMLTypeQ3_K
	case GGUFFileTypeMostlyQ4_K:
		return GGMLTypeQ4_K
	case GGUFFileTypeMostlyQ5_K:
		return GGMLTypeQ5_K
	case GGUFFileTypeMostlyQ6_K:
		return GGMLTypeQ6_K
	case GGUFFileTypeMostlyIQ2_XXS:
		return GGMLTypeIQ2_XXS
	case GGUFFileTypeMostlyIQ2_XS:
		return GGMLTypeIQ2_XS
	case GGUFFileTypeMostlyIQ3_XXS:
		return GGMLTypeIQ3_XXS
	case GGUFFileTypeMostlyIQ1_S:
		return GGMLTypeIQ1_S
	case GGUFFileTypeMostlyIQ4_NL:
		return GGMLTypeIQ4_NL
	case GGUFFileTypeMostlyIQ3_S:
		return GGMLTypeIQ3_S
	case GGUFFileTypeMostlyIQ2_S:
		return GGMLTypeIQ2_S
	case GGUFFileTypeMostlyIQ4_XS:
		return GGMLTypeIQ4_XS
	case GGUFFileTypeMostlyIQ1_M:
		return GGMLTypeIQ1_M
	case GGUFFileTypeMostlyBF16:
		return GGMLTypeBF16
	case GGUFFileTypeMostlyQ4_0_4_4:
		return GGMLTypeQ4_0_4_4
	case GGUFFileTypeMostlyQ4_0_4_8:
		return GGMLTypeQ4_0_4_8
	case GGUFFileTypeMostlyQ4_0_8_8:
		return GGMLTypeQ4_0_8_8
	case GGUFFileTypeMostlyTQ1_0:
		return GGMLTypeTQ1_0
	case GGUFFileTypeMostlyTQ2_0:
		return GGMLTypeTQ2_0
	case GGUFFileTypeMostlyIQ4_NL_4_4:
		return GGMLTypeIQ4_NL_4_4
	case GGUFFileTypeMostlyIQ4_NL_4_8:
		return GGMLTypeIQ4_NL_4_8
	case GGUFFileTypeMostlyIQ4_NL_8_8:
		return GGMLTypeIQ4_NL_8_8
	default:
	}
	return _GGMLTypeCount
}

// guessFileType guesses the GGUF file type by
// statistically analyzing the tensor types,
// which is inspired by
// https://huggingface.co/TheBloke/Llama-2-13B-chat-GGML#provided-files.
func (gf *GGUFFile) guessFileType(arch string) GGUFFileType {
	if len(gf.TensorInfos) == 0 {
		return _GGUFFileTypeCount
	}

	// Count.
	cm := make(map[GGMLType]int)
	for i := range gf.TensorInfos {
		switch {
		case arch != "diffusion" && !strings.HasPrefix(gf.TensorInfos[i].Name, "blk."):
			continue
		case arch == "diffusion" && !strings.HasSuffix(gf.TensorInfos[i].Name, ".weight"):
			continue
		}
		cm[gf.TensorInfos[i].Type]++
	}

	return GetFileType(cm)
}

// GetFileType returns the GGUFFileType represented the mostly GGMLType of the given tensors counter.
//
// The input `cm` is a map of GGMLType to the count of tensors of that type.
func GetFileType(cm map[GGMLType]int) GGUFFileType {
	if len(cm) == 0 {
		return _GGUFFileTypeCount
	}

	// Sort.
	ts := maps.Keys(cm)
	sort.Slice(ts, func(i, j int) bool {
		return cm[ts[i]] > cm[ts[j]]
	})

	// Guess.
	if ts[0] == GGMLTypeF32 {
		if len(ts) == 1 {
			return GGUFFileTypeAllF32
		}
		ts[0] = ts[1]
	}
	switch ts[0] {
	case GGMLTypeF16:
		return GGUFFileTypeMostlyF16
	case GGMLTypeQ4_0:
		return GGUFFileTypeMostlyQ4_0
	case GGMLTypeQ4_1:
		return GGUFFileTypeMostlyQ4_1
	case GGMLTypeQ4_2:
		return GGUFFileTypeMostlyQ4_2
	case GGMLTypeQ4_3:
		return GGUFFileTypeMostlyQ4_3
	case GGMLTypeQ5_0:
		return GGUFFileTypeMostlyQ5_0
	case GGMLTypeQ5_1:
		return GGUFFileTypeMostlyQ5_1
	case GGMLTypeQ8_0:
		return GGUFFileTypeMostlyQ8_0
	case GGMLTypeQ2_K:
		return GGUFFileTypeMostlyQ2_K
	case GGMLTypeQ3_K:
		switch ts[1] {
		case GGMLTypeQ4_K: // Legacy, Q3_K_M.
			return GGUFFileTypeMostlyQ4_K
		case GGMLTypeQ5_K: // Legacy, Q3_K_L.
			return GGUFFileTypeMostlyQ5_K
		default: // Legacy. Q3_K_S
			return GGUFFileTypeMostlyQ3_K
		}
	case GGMLTypeQ4_K:
		if len(ts) > 2 && ts[2] == GGMLTypeQ6_K { // Legacy, Q4_K_M.
			return GGUFFileTypeMostlyIQ2_XXS
		}
		return GGUFFileTypeMostlyQ6_K // Legacy. Q4_K_S
	case GGMLTypeQ5_K:
		if len(ts) > 2 && ts[2] == GGMLTypeQ6_K { // Legacy, Q5_K_M.
			return GGUFFileTypeMostlyIQ3_XXS
		}
		return GGUFFileTypeMostlyIQ2_XS // Legacy. Q5_K_S
	case GGMLTypeQ6_K:
		return GGUFFileTypeMostlyIQ1_S // Legacy. Q6_K
	case GGMLTypeIQ2_XXS:
		return GGUFFileTypeMostlyIQ2_XXS
	case GGMLTypeIQ2_XS:
		return GGUFFileTypeMostlyIQ2_XS
	case GGMLTypeIQ3_XXS:
		return GGUFFileTypeMostlyIQ3_XXS
	case GGMLTypeIQ1_S:
		return GGUFFileTypeMostlyIQ1_S
	case GGMLTypeIQ4_NL:
		return GGUFFileTypeMostlyIQ4_NL
	case GGMLTypeIQ3_S:
		return GGUFFileTypeMostlyIQ3_S
	case GGMLTypeIQ2_S:
		return GGUFFileTypeMostlyIQ2_S
	case GGMLTypeIQ4_XS:
		return GGUFFileTypeMostlyIQ4_XS
	case GGMLTypeIQ1_M:
		return GGUFFileTypeMostlyIQ1_M
	case GGMLTypeBF16:
		return GGUFFileTypeMostlyBF16
	case GGMLTypeQ4_0_4_4:
		return GGUFFileTypeMostlyQ4_0_4_4
	case GGMLTypeQ4_0_4_8:
		return GGUFFileTypeMostlyQ4_0_4_8
	case GGMLTypeQ4_0_8_8:
		return GGUFFileTypeMostlyQ4_0_8_8
	case GGMLTypeTQ1_0:
		return GGUFFileTypeMostlyTQ1_0
	case GGMLTypeTQ2_0:
		return GGUFFileTypeMostlyTQ2_0
	case GGMLTypeIQ4_NL_4_4:
		return GGUFFileTypeMostlyIQ4_NL_4_4
	case GGMLTypeIQ4_NL_4_8:
		return GGUFFileTypeMostlyIQ4_NL_4_8
	case GGMLTypeIQ4_NL_8_8:
		return GGUFFileTypeMostlyIQ4_NL_8_8
	default:
	}
	return _GGUFFileTypeCount
}
