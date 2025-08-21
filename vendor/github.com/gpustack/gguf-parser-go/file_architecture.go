package gguf_parser

import (
	"regexp"
	"strings"
)

// Types for the architecture metadata of a GGUF file.
type (
	// GGUFArchitecture represents the architecture metadata of a GGUF file.
	GGUFArchitecture struct {
		/* Basic */

		// Type describes the type of the file,
		// default is "model".
		Type string `json:"type"`
		// Architecture describes what architecture this model implements.
		//
		// All lowercase ASCII.
		Architecture string `json:"architecture"`
		// MaximumContextLength(n_ctx_train) is the maximum context length of the model.
		//
		// For most architectures, this is the hard limit on the length of the input.
		// Architectures, like RWKV,
		// that are not reliant on transformer-style attention may be able to handle larger inputs,
		// but this is not guaranteed.
		MaximumContextLength uint64 `json:"maximumContextLength,omitempty"`
		// EmbeddingLength(n_embd) is the length of the embedding layer.
		EmbeddingLength uint64 `json:"embeddingLength,omitempty"`
		// BlockCount(n_layer) is the number of blocks of attention and feed-forward layers,
		// i.e. the bulk of the LLM.
		// This does not include the input or embedding layers.
		BlockCount uint64 `json:"blockCount,omitempty"`
		// FeedForwardLength(n_ff) stores the length of each feed-forward layer.
		FeedForwardLength []uint64 `json:"feedForwardLength,omitempty"`
		// ExpertFeedForwardLength(expert_feed_forward_length) is the length of the feed-forward layer in the expert model.
		ExpertFeedForwardLength uint64 `json:"expertFeedForwardLength,omitempty"`
		// ExpertSharedFeedForwardLength(expert_shared_feed_forward_length) is the length of the shared feed-forward layer in the expert model.
		ExpertSharedFeedForwardLength uint64 `json:"expertSharedFeedForwardLength,omitempty"`
		// ExpertCount(n_expert) is the number of experts in MoE models.
		ExpertCount uint32 `json:"expertCount,omitempty"`
		// ExpertUsedCount(n_expert_used) is the number of experts used during each token evaluation in MoE models.
		ExpertUsedCount uint32 `json:"expertUsedCount,omitempty"`
		// AttentionHeadCount(n_head) is the number of attention heads.
		AttentionHeadCount uint64 `json:"attentionHeadCount,omitempty"`
		// AttentionHeadCountKV(n_head_kv) is the number of attention heads per group used in Grouped-Query-Attention.
		//
		// If not provided or equal to AttentionHeadCount,
		// the model does not use Grouped-Query-Attention.
		AttentionHeadCountKV uint64 `json:"attentionHeadCountKV,omitempty"`
		// AttentionMaxALiBIBias is the maximum bias to use for ALiBI.
		AttentionMaxALiBIBias float32 `json:"attentionMaxALiBIBias,omitempty"`
		// AttentionClampKQV describes a value `C`,
		// which is used to clamp the values of the `Q`, `K` and `V` tensors between `[-C, C]`.
		AttentionClampKQV float32 `json:"attentionClampKQV,omitempty"`
		// AttentionLayerNormEpsilon is the epsilon value used in the LayerNorm(Layer Normalization).
		AttentionLayerNormEpsilon float32 `json:"attentionLayerNormEpsilon,omitempty"`
		// AttentionLayerNormRMSEpsilon is the epsilon value used in the RMSNorm(root Mean Square Layer Normalization),
		// which is a simplification of the original LayerNorm.
		AttentionLayerNormRMSEpsilon float32 `json:"attentionLayerNormRMSEpsilon,omitempty"`
		// AttentionKeyLength(n_embd_head_k) is the size of a key head.
		//
		// Defaults to `EmbeddingLength / AttentionHeadCount`.
		AttentionKeyLength uint32 `json:"attentionKeyLength,omitempty"`
		// AttentionValueLength(n_embd_head_v) is the size of a value head.
		//
		// Defaults to `EmbeddingLength / AttentionHeadCount`.
		AttentionValueLength uint32 `json:"attentionValueLength,omitempty"`
		// AttentionCausal is true if the attention is causal.
		AttentionCausal bool `json:"attentionCausal,omitempty"`
		// RoPEDimensionCount is the number of dimensions in the RoPE(Rotary Positional Encoding).
		RoPEDimensionCount uint64 `json:"ropeDimensionCount,omitempty"`
		// RoPEFrequencyBase is the base frequency of the RoPE.
		RoPEFrequencyBase float32 `json:"ropeFrequencyBase,omitempty"`
		// RoPEFrequencyScale is the frequency scale of the RoPE.
		RoPEScalingType string `json:"ropeScalingType,omitempty"`
		// RoPEScalingFactor is the scaling factor of the RoPE.
		RoPEScalingFactor float32 `json:"ropeScalingFactor,omitempty"`
		// RoPEScalingOriginalContextLength is the original context length of the RoPE scaling.
		RoPEScalingOriginalContextLength uint64 `json:"ropeScalingOriginalContextLength,omitempty"`
		// RoPEScalingFinetuned is true if the RoPE scaling is fine-tuned.
		RoPEScalingFinetuned bool `json:"ropeScalingFinetuned,omitempty"`
		// SSMConvolutionKernel is the size of the convolution kernel used in the SSM(Selective State Space Model).
		SSMConvolutionKernel uint32 `json:"ssmConvolutionKernel,omitempty"`
		// SSMInnerSize is the embedding size of the state in SSM.
		SSMInnerSize uint32 `json:"ssmInnerSize,omitempty"`
		// SSMStateSize is the size of the recurrent state in SSM.
		SSMStateSize uint32 `json:"ssmStateSize,omitempty"`
		// SSMTimeStepRank is the rank of the time steps in SSM.
		SSMTimeStepRank uint32 `json:"ssmTimeStepRank,omitempty"`
		// VocabularyLength is the size of the vocabulary.
		//
		// VocabularyLength is the same as the tokenizer's token size.
		VocabularyLength uint64 `json:"vocabularyLength,omitempty"`

		/* Appendix */

		// EmbeddingGGQA is the GQA of the embedding layer.
		EmbeddingGQA uint64 `json:"embeddingGQA,omitempty"`
		// EmbeddingKeyGQA is the number of key GQA in the embedding layer.
		EmbeddingKeyGQA uint64 `json:"embeddingKeyGQA,omitempty"`
		// EmbeddingValueGQA is the number of value GQA in the embedding layer.
		EmbeddingValueGQA uint64 `json:"embeddingValueGQA,omitempty"`

		// ClipProjectorType is the type of the projector used in the clip model.
		//
		// Only used when Architecture is "clip".
		ClipProjectorType string `json:"clipProjectorType,omitempty"`
		// ClipHasLLaVAProjector indicates whether the clip model has LLaVA projector or not.
		//
		// Only used when Architecture is "clip".
		ClipHasLLaVAProjector bool `json:"clipHasLLaVAProjector,omitempty"`
		// ClipHasMiniCPMVProjector indicates whether the clip model has MiniCPMV projector or not.
		//
		// Only used when Architecture is "clip".
		ClipHasMiniCPMVProjector bool `json:"clipHasMiniCPMVProject,omitempty"`
		// ClipMiniCPMVVersion is the version of the MiniCPMV projector.
		//
		// Only used when Architecture is "clip" and ClipHasMiniCPMVProjector is true.
		ClipMiniCPMVVersion int32 `json:"clipMiniCPMVVersion,omitempty"`
		// ClipHasGLMProjector indicates whether the clip model has GLM projector or not.
		//
		// Only used when Architecture is "clip".
		ClipHasGLMProjector bool `json:"clipHasGLMProjector,omitempty"`
		// ClipHasQwen2VLMerger indicates whether the clip model has Qwen2VL merger or not.
		//
		// Only used when Architecture is "clip".
		ClipHasQwen2VLMerger bool `json:"clipHasQwen2VLMerger,omitempty"`
		// ClipHasTextEncoder indicates whether the clip model has text encoder or not.
		//
		// Only used when Architecture is "clip".
		ClipHasTextEncoder bool `json:"clipHasTextEncoder,omitempty"`
		// ClipHasVisionEncoder indicates whether the clip model has vision encoder or not.
		//
		// Only used when Architecture is "clip".
		ClipHasVisionEncoder bool `json:"clipHasVisionEncoder,omitempty"`
		// ClipVisionImageSize indicates the image size of vision encoder.
		//
		// Only used when Architecture is "clip" and ClipHasVisionEncoder is true.
		ClipVisionImageSize uint32 `json:"clipVisionImageSize,omitempty"`
		// ClipVisionPatchSize indicates the patch size of vision encoder.
		//
		// Only used when Architecture is "clip" and ClipHasVisionEncoder is true.
		ClipVisionPatchSize uint32 `json:"clipVisionPatchSize,omitempty"`
		// ClipVisionProjectionDim indicates the projection dimension of vision encoder.
		//
		// Only used when Architecture is "clip" and ClipHasVisionEncoder is true.
		ClipVisionProjectionDim uint32 `json:"clipVisionProjectionDim,omitempty"`
		// ClipVisionMMPatchMergeType indicates the merge type of the vision encoder.
		//
		// Only used when Architecture is "clip" and ClipHasVisionEncoder is true.
		ClipVisionMMPatchMergeType string `json:"clipVisionMMPatchMergeType,omitempty"`

		// AdapterType is the type of the adapter.
		//
		// Only used when Architecture is "adapter".
		AdapterType string `json:"adapterType,omitempty"`
		// AdapterLoRAAlpha is the alpha value of the LoRA adapter.
		//
		// Only used when AdapterType is "lora".
		AdapterLoRAAlpha float32 `json:"adapterLoRAAlpha,omitempty"`
		// AdapterControlVectorLayerCount is the number of layers in the control vector.
		//
		// Only used when Architecture is "control_vector".
		AdapterControlVectorLayerCount uint32 `json:"adapterControlVectorLayerCount,omitempty"`

		// DiffusionArchitecture is the actual architecture of the diffusion model.
		//
		// Only used when Architecture is "diffusion".
		DiffusionArchitecture string `json:"diffusionArchitecture,omitempty"`
		// DiffusionTransformer indicates whether the diffusion model is a diffusion transformer or not.
		//
		DiffusionTransformer bool `json:"diffusionTransformer,omitempty"`
		// DiffusionConditioners is the list of diffusion conditioners.
		//
		// Only used when Architecture is "diffusion".
		DiffusionConditioners GGUFArchitectureDiffusionConditioners `json:"diffusionConditioners,omitempty"`
		// DiffusionAutoencoder represents the autoencoder of the diffusion model.
		//
		// Only used when Architecture is "diffusion".
		DiffusionAutoencoder *GGUFArchitectureDiffusionAutoencoder `json:"diffusionAutoencoder,omitempty"`
	}

	// GGUFArchitectureDiffusionConditioners is the list of GGUFArchitectureDiffusionConditioner.
	GGUFArchitectureDiffusionConditioners []GGUFArchitectureDiffusionConditioner

	// GGUFArchitectureDiffusionConditioner represents the conditioner metadata of the diffusion architecture.
	GGUFArchitectureDiffusionConditioner struct {
		// Architecture is the architecture of the diffusion conditioner.
		Architecture string `json:"architecture"`

		// FileType describes the type of the majority of the tensors in the GGUF file.
		FileType GGUFFileType `json:"fileType"`
	}

	// GGUFArchitectureDiffusionAutoencoder represents the autoencoder metadata of the diffusion architecture.
	GGUFArchitectureDiffusionAutoencoder struct {
		// Architecture is the architecture of the diffusion autoencoder.
		//
		// Currently, only "VAE" is supported.
		Architecture string `json:"architecture"`

		// FileType describes the type of the majority of the tensors in the GGUF file.
		FileType GGUFFileType `json:"fileType"`
	}
)

// DiffusionHasConditioners returns true if the diffusion model has conditioners.
func (ga GGUFArchitecture) DiffusionHasConditioners() bool {
	return len(ga.DiffusionConditioners) > 0
}

// DiffusionHasAutoencoder returns true if the diffusion model has an autoencoder.
func (ga GGUFArchitecture) DiffusionHasAutoencoder() bool {
	return ga.DiffusionAutoencoder != nil && ga.DiffusionAutoencoder.Architecture != ""
}

func (gacs GGUFArchitectureDiffusionConditioners) String() string {
	var sb strings.Builder
	for i, gac := range gacs {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(gac.String())
	}
	return sb.String()
}

func (gac GGUFArchitectureDiffusionConditioner) String() string {
	return gac.Architecture + " (" + gac.FileType.String() + ")"
}

func (gaa GGUFArchitectureDiffusionAutoencoder) String() string {
	return gaa.Architecture + " (" + gaa.FileType.String() + ")"
}

// Architecture returns the architecture metadata of the GGUF file.
func (gf *GGUFFile) Architecture() (ga GGUFArchitecture) {
	if gf.TensorInfos.Match(regexp.MustCompile(`^model\.diffusion_model\..*`)) ||
		gf.TensorInfos.Match(regexp.MustCompile(`^double_blocks\..*`)) {
		return gf.diffuserArchitecture()
	}

	var (
		generalTypeKey         = "general.type"
		generalArchitectureKey = "general.architecture"

		controlVectorModelHintKey = "controlvector.model_hint"
	)
	m, _ := gf.Header.MetadataKV.Index([]string{
		generalTypeKey,
		generalArchitectureKey,
		controlVectorModelHintKey,
	})

	typ, arch := "model", "llama" // nolint: goconst
	{
		if v, ok := m[generalTypeKey]; ok {
			typ = v.ValueString()
		}
		if v, ok := m[generalArchitectureKey]; ok {
			arch = v.ValueString()
		}
	}

	switch {
	case arch == "clip":
		return gf.clipArchitecture()
	case arch == "controlvector":
		arch = "llama"
		if v, ok := m[controlVectorModelHintKey]; ok {
			arch = v.ValueString()
		}
		return gf.adapterArchitecture(arch)
	case typ == "adapter":
		return gf.adapterArchitecture(arch)
	}
	return gf.transformerArchitecture(arch)
}

func (gf *GGUFFile) diffuserArchitecture() (ga GGUFArchitecture) {
	const (
		// Diffusion

		sdKey               = "model.diffusion_model.output_blocks.11.1.transformer_blocks.0.attn2.to_v.weight" // SD 1.x/2.x
		sdXlKey             = "model.diffusion_model.output_blocks.5.1.transformer_blocks.1.attn1.to_v.weight"  // SD XL
		sdXlRefinerKey      = "model.diffusion_model.output_blocks.8.1.transformer_blocks.1.attn1.to_v.weight"  // SD XL Refiner
		sd3Key              = "model.diffusion_model.joint_blocks.23.x_block.attn.proj.weight"                  // SD 3.x
		sdInPaintFeatureKey = "model.diffusion_model.input_blocks.0.0.weight"                                   // SD in-paint feature

		fluxKey             = "model.diffusion_model.double_blocks.0.txt_attn.proj.weight" // FLUX.1
		fluxKey2            = "double_blocks.0.txt_attn.proj.weight"
		fluxFillFeatureKey  = "model.diffusion_model.img_in.weight" // FLUX.1 Fill feature
		fluxFillFeatureKey2 = "img_in.weight"

		// Conditioner

		openAiClipVitL14Key = "cond_stage_model.transformer.text_model.encoder.layers.11.self_attn.k_proj.weight"   // OpenAI CLIP ViT-L/14
		openClipVitH14Key   = "cond_stage_model.transformer.text_model.encoder.layers.22.self_attn.k_proj.weight"   // OpenCLIP ViT-H/14
		openClipVitG14Key   = "cond_stage_model.1.transformer.text_model.encoder.layers.31.self_attn.k_proj.weight" // OpenCLIP ViT-G/14
		t5xxlKey            = "cond_stage_model.1.transformer.encoder.block.23.layer.0.SelfAttention.k.weight"      // Google T5-xxl
		t5xxlKey2           = "cond_stage_model.2.transformer.encoder.block.23.layer.0.SelfAttention.k.weight"
	)

	tis, _ := gf.TensorInfos.Index([]string{
		sdKey,
		sdXlKey,
		sdXlRefinerKey,
		sd3Key,
		sdInPaintFeatureKey,

		fluxKey,
		fluxKey2,
		fluxFillFeatureKey,
		fluxFillFeatureKey2,

		openAiClipVitL14Key,
		openClipVitH14Key,
		openClipVitG14Key,
		t5xxlKey,
		t5xxlKey2,
	})

	ga.Type = "model"
	ga.Architecture = "diffusion"

	if ti, ok := tis[sdKey]; ok {
		ga.DiffusionArchitecture = "Stable Diffusion 1.x"
		if ti.Dimensions[0] == 1024 {
			ga.DiffusionArchitecture = "Stable Diffusion 2.x"
		}
		if ti, ok := tis[sdInPaintFeatureKey]; ok && ti.Dimensions[2] == 9 {
			ga.DiffusionArchitecture += " InPaint"
		}
	} else if _, ok := tis[sdXlKey]; ok {
		ga.DiffusionArchitecture = "Stable Diffusion XL"
		if _, ok = tis[sdXlRefinerKey]; ok {
			ga.DiffusionArchitecture = "Stable Diffusion XL Refiner"
		}
		if ti, ok := tis[sdInPaintFeatureKey]; ok && ti.Dimensions[2] == 9 {
			ga.DiffusionArchitecture += " InPaint"
		}
	} else if _, ok := tis[sd3Key]; ok {
		ga.DiffusionArchitecture = "Stable Diffusion 3.x"
		ga.DiffusionTransformer = true
	}
	if _, ok := tis[fluxKey]; ok {
		ga.DiffusionArchitecture = "FLUX.1"
		ga.DiffusionTransformer = true
		if ti, ok := tis[fluxFillFeatureKey]; ok && ti.Dimensions[0] == 384 {
			ga.DiffusionArchitecture += " Fill"
		}
	} else if _, ok := tis[fluxKey2]; ok {
		ga.DiffusionArchitecture = "FLUX.1"
		ga.DiffusionTransformer = true
		if ti, ok := tis[fluxFillFeatureKey2]; ok && ti.Dimensions[0] == 384 {
			ga.DiffusionArchitecture += " Fill"
		}
	}

	if ti, ok := tis[openAiClipVitL14Key]; ok {
		cond := GGUFArchitectureDiffusionConditioner{
			Architecture: "OpenAI CLIP ViT-L/14",
			FileType:     ti.GetFileType(),
		}
		if ti, ok = tis[openClipVitH14Key]; ok {
			cond = GGUFArchitectureDiffusionConditioner{
				Architecture: "OpenCLIP ViT-H/14",
				FileType:     ti.GetFileType(),
			}
		}
		ga.DiffusionConditioners = append(ga.DiffusionConditioners, cond)
	}
	if ti, ok := tis[openClipVitG14Key]; ok {
		ga.DiffusionConditioners = append(ga.DiffusionConditioners, GGUFArchitectureDiffusionConditioner{
			Architecture: "OpenCLIP ViT-G/14",
			FileType:     ti.GetFileType(),
		})
	}
	if ti, ok := tis[t5xxlKey]; ok {
		ga.DiffusionConditioners = append(ga.DiffusionConditioners, GGUFArchitectureDiffusionConditioner{
			Architecture: "Google T5-xxl",
			FileType:     ti.GetFileType(),
		})
	} else if ti, ok = tis[t5xxlKey2]; ok {
		ga.DiffusionConditioners = append(ga.DiffusionConditioners, GGUFArchitectureDiffusionConditioner{
			Architecture: "Google T5-xxl",
			FileType:     ti.GetFileType(),
		})
	}

	if tis := gf.TensorInfos.Search(regexp.MustCompile(`^first_stage_model\..*`)); len(tis) != 0 {
		ga.DiffusionAutoencoder = &GGUFArchitectureDiffusionAutoencoder{
			Architecture: ga.DiffusionArchitecture + " VAE",
			FileType:     GGUFTensorInfos(tis).GetFileType(),
		}
	}

	return ga
}

func (gf *GGUFFile) clipArchitecture() (ga GGUFArchitecture) {
	const (
		projectorTypeKey       = "clip.projector_type"
		hasLLaVAProjectorKey   = "clip.has_llava_projector"
		hasMiniCPMVProjector   = "clip.has_minicpmv_projector"
		miniCPMVVersionKey     = "clip.minicpmv_version"
		hasGLMProjectorKey     = "clip.has_glm_projector"
		hasQwen2VLMergerKey    = "clip.has_qwen2vl_merger"
		hasTextEncoderKey      = "clip.has_text_encoder"
		hasVisionEncoderKey    = "clip.has_vision_encoder"
		visionImageSizeKey     = "clip.vision.image_size"
		visionPatchSizeKey     = "clip.vision.patch_size"
		visionProjectionDim    = "clip.vision.projection_dim"
		visionMMPatchMergeType = "clip.vision.mm_patch_merge_type"

		textEmbeddingLengthKey              = "clip.text.embedding_length"
		textBlockCountKey                   = "clip.text.block_count"
		textFeedForwardLengthKey            = "clip.text.feed_forward_length"
		textAttentionHeadCountKey           = "clip.text.attention.head_count"
		textAttentionLayerNormRMSEpsilonKey = "clip.text.attention.layer_norm_epsilon"

		visionEmbeddingLengthKey              = "clip.vision.embedding_length"
		visionBlockCountKey                   = "clip.vision.block_count"
		visionFeedForwardLengthKey            = "clip.vision.feed_forward_length"
		visionAttentionHeadCountKey           = "clip.vision.attention.head_count"
		visionAttentionLayerNormRMSEpsilonKey = "clip.vision.attention.layer_norm_epsilon"
	)

	ga.Type = "projector"
	ga.Architecture = "clip"

	m, _ := gf.Header.MetadataKV.Index([]string{
		projectorTypeKey,
		hasLLaVAProjectorKey,
		hasMiniCPMVProjector,
		miniCPMVVersionKey,
		hasGLMProjectorKey,
		hasQwen2VLMergerKey,
		hasTextEncoderKey,
		hasVisionEncoderKey,
		visionImageSizeKey,
		visionPatchSizeKey,
		visionProjectionDim,
		visionMMPatchMergeType,
		textEmbeddingLengthKey,
		textBlockCountKey,
		textFeedForwardLengthKey,
		textAttentionHeadCountKey,
		textAttentionLayerNormRMSEpsilonKey,
		visionEmbeddingLengthKey,
		visionBlockCountKey,
		visionFeedForwardLengthKey,
		visionAttentionHeadCountKey,
		visionAttentionLayerNormRMSEpsilonKey,
	})

	if v, ok := m[projectorTypeKey]; ok {
		ga.ClipProjectorType = v.ValueString()
	} else {
		ga.ClipProjectorType = "mlp"
	}
	if v, ok := m[hasLLaVAProjectorKey]; ok {
		ga.ClipHasLLaVAProjector = v.ValueBool()
	}
	if v, ok := m[hasMiniCPMVProjector]; ok {
		ga.ClipHasMiniCPMVProjector = v.ValueBool()
	}
	if v, ok := m[miniCPMVVersionKey]; ok {
		ga.ClipMiniCPMVVersion = ValueNumeric[int32](v)
	}
	if v, ok := m[hasGLMProjectorKey]; ok {
		ga.ClipHasGLMProjector = v.ValueBool()
	}
	if v, ok := m[hasQwen2VLMergerKey]; ok {
		ga.ClipHasQwen2VLMerger = v.ValueBool()
	}
	if v, ok := m[hasTextEncoderKey]; ok {
		ga.ClipHasTextEncoder = v.ValueBool()
	}
	if v, ok := m[hasVisionEncoderKey]; ok {
		ga.ClipHasVisionEncoder = v.ValueBool()
	}
	if v, ok := m[visionImageSizeKey]; ok {
		ga.ClipVisionImageSize = ValueNumeric[uint32](v)
	}
	if v, ok := m[visionPatchSizeKey]; ok {
		ga.ClipVisionPatchSize = ValueNumeric[uint32](v)
	}
	if v, ok := m[visionProjectionDim]; ok {
		ga.ClipVisionProjectionDim = ValueNumeric[uint32](v)
	}
	ga.ClipVisionMMPatchMergeType = "flat"
	if v, ok := m[visionMMPatchMergeType]; ok {
		ga.ClipVisionMMPatchMergeType = v.ValueString()
	}

	if v, ok := m[textEmbeddingLengthKey]; ok {
		ga.EmbeddingLength = ValueNumeric[uint64](v)
	}
	if v, ok := m[textBlockCountKey]; ok {
		ga.BlockCount = ValueNumeric[uint64](v)
	}
	if v, ok := m[textFeedForwardLengthKey]; ok {
		if v.ValueType == GGUFMetadataValueTypeArray {
			ga.FeedForwardLength = ValuesNumeric[uint64](v.ValueArray())
		} else {
			vx := ValueNumeric[uint64](v)
			ga.FeedForwardLength = make([]uint64, ga.BlockCount)
			for i := range ga.FeedForwardLength {
				ga.FeedForwardLength[i] = vx
			}
		}
	}
	if v, ok := m[textAttentionHeadCountKey]; ok {
		ga.AttentionHeadCount = ValueNumeric[uint64](v)
	}
	if v, ok := m[textAttentionLayerNormRMSEpsilonKey]; ok {
		ga.AttentionLayerNormRMSEpsilon = ValueNumeric[float32](v)
	}

	if v, ok := m[visionEmbeddingLengthKey]; ok {
		ga.EmbeddingLength = ValueNumeric[uint64](v)
	}
	if v, ok := m[visionBlockCountKey]; ok {
		ga.BlockCount = ValueNumeric[uint64](v)
	}
	if v, ok := m[visionFeedForwardLengthKey]; ok {
		if v.ValueType == GGUFMetadataValueTypeArray {
			ga.FeedForwardLength = ValuesNumeric[uint64](v.ValueArray())
		} else {
			vx := ValueNumeric[uint64](v)
			ga.FeedForwardLength = make([]uint64, ga.BlockCount)
			for i := range ga.FeedForwardLength {
				ga.FeedForwardLength[i] = vx
			}
		}
	}
	if v, ok := m[visionAttentionHeadCountKey]; ok {
		ga.AttentionHeadCount = ValueNumeric[uint64](v)
	}
	if v, ok := m[visionAttentionLayerNormRMSEpsilonKey]; ok {
		ga.AttentionLayerNormRMSEpsilon = ValueNumeric[float32](v)
	}

	ga.AttentionHeadCountKV = ga.AttentionHeadCount

	{
		if ga.AttentionHeadCountKV > 0 {
			ga.EmbeddingGQA = ga.AttentionHeadCount / ga.AttentionHeadCountKV
		}
		if ga.AttentionHeadCount > 0 {
			ga.EmbeddingKeyGQA = uint64(ga.AttentionKeyLength) * ga.AttentionHeadCountKV
			ga.EmbeddingValueGQA = uint64(ga.AttentionValueLength) * ga.AttentionHeadCountKV
		}
		if ga.Architecture == "mamba" {
			ga.EmbeddingKeyGQA = uint64((ga.SSMConvolutionKernel - 1) * ga.SSMInnerSize)
			ga.EmbeddingValueGQA = uint64(ga.SSMStateSize * ga.SSMInnerSize)
		}
	}

	return ga
}

func (gf *GGUFFile) adapterArchitecture(arch string) (ga GGUFArchitecture) {
	var (
		typeKey = "adapter.type"

		loraAlphaKey = "adapter.lora.alpha"

		controlVectorLayerCountKey  = "adapter.control_vector.layer_count"
		controlVectorLayerCountKey2 = "control_vector.layer_count"
	)

	ga.Type = "adapter"
	ga.Architecture = arch

	m, _ := gf.Header.MetadataKV.Index([]string{
		typeKey,
		loraAlphaKey,
		controlVectorLayerCountKey,
		controlVectorLayerCountKey2,
	})

	if v, ok := m[typeKey]; ok {
		ga.AdapterType = v.ValueString()
	}
	if v, ok := m[loraAlphaKey]; ok {
		ga.AdapterLoRAAlpha = ValueNumeric[float32](v)
	}
	if v, ok := m[controlVectorLayerCountKey]; ok {
		ga.AdapterControlVectorLayerCount = ValueNumeric[uint32](v)
	} else if v, ok := m[controlVectorLayerCountKey2]; ok {
		ga.AdapterControlVectorLayerCount = ValueNumeric[uint32](v)
	}

	return ga
}

func (gf *GGUFFile) transformerArchitecture(arch string) (ga GGUFArchitecture) {
	var (
		contextLengthKey     = arch + ".context_length"
		embeddingLengthKey   = arch + ".embedding_length"
		blockCountKey        = arch + ".block_count"
		feedForwardLengthKey = arch + ".feed_forward_length"

		expertFeedForwardLengthKey       = arch + ".expert_feed_forward_length"
		expertSharedFeedForwardLengthKey = arch + ".expert_shared_feed_forward_length"
		expertCountKey                   = arch + ".expert_count"
		expertUsedCountKey               = arch + ".expert_used_count"

		attentionHeadCountKey           = arch + ".attention.head_count"
		attentionHeadCountKVKey         = arch + ".attention.head_count_kv"
		attentionMaxALiBIBiasKey        = arch + ".attention.max_alibi_bias"
		attentionMaxALiBIBiasKey2       = arch + ".attention.alibi_bias_max"
		attentionClampKQVKey            = arch + ".attention.clamp_kqv"
		attentionClampKQVKey2           = arch + ".attention.clip_kqv"
		attentionLayerNormEpsilonKey    = arch + ".attention.layer_norm_epsilon"
		attentionLayerNormRMSEpsilonKey = arch + ".attention.layer_norm_rms_epsilon"
		attentionKeyLengthKey           = arch + ".attention.key_length"
		attentionValueLengthKey         = arch + ".attention.value_length"
		attentionCausalKey              = arch + ".attention.causal"

		ropeDimensionCountKey         = arch + ".rope.dimension_count"
		ropeFrequencyBaseKey          = arch + ".rope.freq_base"
		ropeScaleLinearKey            = arch + ".rope.scale_linear"
		ropeScalingTypeKey            = arch + ".rope.scaling.type"
		ropeScalingFactorKey          = arch + ".rope.scaling.factor"
		ropeScalingOriginalContextKey = arch + ".rope.scaling.original_context_length" // uint32 maybe
		ropeScalingFinetunedKey       = arch + ".rope.scaling.finetuned"

		ssmConvolutionKernelKey = arch + ".ssm.conv_kernel"
		ssmInnerSizeKey         = arch + ".ssm.inner_size"
		ssmStateSizeKey         = arch + ".ssm.state_size"
		ssmTimeStepRankKey      = arch + ".ssm.time_step_rank"

		vocabularyLengthKey    = arch + ".vocab_size"
		tokenizerGGMLTokensKey = "tokenizer.ggml.tokens"
	)

	ga.Type = "model"
	ga.Architecture = arch

	m, _ := gf.Header.MetadataKV.Index([]string{
		contextLengthKey,
		embeddingLengthKey,
		blockCountKey,
		feedForwardLengthKey,
		expertFeedForwardLengthKey,
		expertSharedFeedForwardLengthKey,
		expertCountKey,
		expertUsedCountKey,
		attentionHeadCountKey,
		attentionHeadCountKVKey,
		attentionMaxALiBIBiasKey,
		attentionMaxALiBIBiasKey2,
		attentionClampKQVKey,
		attentionClampKQVKey2,
		attentionLayerNormEpsilonKey,
		attentionLayerNormRMSEpsilonKey,
		attentionKeyLengthKey,
		attentionValueLengthKey,
		attentionCausalKey,
		ropeDimensionCountKey,
		ropeFrequencyBaseKey,
		ropeScaleLinearKey,
		ropeScalingTypeKey,
		ropeScalingFactorKey,
		ropeScalingOriginalContextKey,
		ropeScalingFinetunedKey,
		ssmConvolutionKernelKey,
		ssmInnerSizeKey,
		ssmStateSizeKey,
		ssmTimeStepRankKey,
		vocabularyLengthKey,
		tokenizerGGMLTokensKey,
	})

	if v, ok := m[contextLengthKey]; ok {
		ga.MaximumContextLength = ValueNumeric[uint64](v)
	}
	if v, ok := m[embeddingLengthKey]; ok {
		ga.EmbeddingLength = ValueNumeric[uint64](v)
	}
	if v, ok := m[blockCountKey]; ok {
		ga.BlockCount = ValueNumeric[uint64](v)
	}
	if v, ok := m[feedForwardLengthKey]; ok {
		if v.ValueType == GGUFMetadataValueTypeArray {
			ga.FeedForwardLength = ValuesNumeric[uint64](v.ValueArray())
		} else {
			vx := ValueNumeric[uint64](v)
			ga.FeedForwardLength = make([]uint64, ga.BlockCount)
			for i := range ga.FeedForwardLength {
				ga.FeedForwardLength[i] = vx
			}
		}
	}

	if v, ok := m[expertCountKey]; ok {
		ga.ExpertCount = ValueNumeric[uint32](v)
	}
	if v, ok := m[expertUsedCountKey]; ok {
		ga.ExpertUsedCount = ValueNumeric[uint32](v)
	}
	if v, ok := m[expertFeedForwardLengthKey]; ok {
		ga.ExpertFeedForwardLength = ValueNumeric[uint64](v)
	}
	if v, ok := m[expertSharedFeedForwardLengthKey]; ok {
		ga.ExpertSharedFeedForwardLength = ValueNumeric[uint64](v)
	}

	if v, ok := m[attentionHeadCountKey]; ok {
		if v.ValueType == GGUFMetadataValueTypeArray {
			ga.AttentionHeadCount = ValuesNumeric[uint64](v.ValueArray())[0]
		} else {
			ga.AttentionHeadCount = ValueNumeric[uint64](v)
		}
	}
	if v, ok := m[attentionHeadCountKVKey]; ok {
		if v.ValueType == GGUFMetadataValueTypeArray {
			ga.AttentionHeadCountKV = ValuesNumeric[uint64](v.ValueArray())[0]
		} else {
			ga.AttentionHeadCountKV = ValueNumeric[uint64](v)
		}
	} else {
		ga.AttentionHeadCountKV = ga.AttentionHeadCount
	}
	if v, ok := m[attentionMaxALiBIBiasKey]; ok {
		ga.AttentionMaxALiBIBias = ValueNumeric[float32](v)
	} else if v, ok := m[attentionMaxALiBIBiasKey2]; ok {
		ga.AttentionMaxALiBIBias = ValueNumeric[float32](v)
	}
	if v, ok := m[attentionClampKQVKey]; ok {
		ga.AttentionClampKQV = ValueNumeric[float32](v)
	} else if v, ok := m[attentionClampKQVKey2]; ok {
		ga.AttentionClampKQV = ValueNumeric[float32](v)
	}
	if v, ok := m[attentionLayerNormEpsilonKey]; ok {
		ga.AttentionLayerNormEpsilon = ValueNumeric[float32](v)
	}
	if v, ok := m[attentionLayerNormRMSEpsilonKey]; ok {
		ga.AttentionLayerNormRMSEpsilon = ValueNumeric[float32](v)
	}
	if v, ok := m[attentionKeyLengthKey]; ok {
		ga.AttentionKeyLength = ValueNumeric[uint32](v)
	} else if ga.AttentionHeadCount != 0 {
		ga.AttentionKeyLength = uint32(ga.EmbeddingLength / ga.AttentionHeadCount)
	}
	if v, ok := m[attentionValueLengthKey]; ok {
		ga.AttentionValueLength = ValueNumeric[uint32](v)
	} else if ga.AttentionHeadCount != 0 {
		ga.AttentionValueLength = uint32(ga.EmbeddingLength / ga.AttentionHeadCount)
	}
	if v, ok := m[attentionCausalKey]; ok {
		ga.AttentionCausal = v.ValueBool()
	} else {
		ga.AttentionCausal = true
	}

	if v, ok := m[ropeDimensionCountKey]; ok {
		ga.RoPEDimensionCount = ValueNumeric[uint64](v)
	}
	if v, ok := m[ropeFrequencyBaseKey]; ok {
		ga.RoPEFrequencyBase = ValueNumeric[float32](v)
	}
	if v, ok := m[ropeScaleLinearKey]; ok {
		ga.RoPEScalingType = "linear"
		ga.RoPEScalingFactor = ValueNumeric[float32](v)
	}
	if v, ok := m[ropeScalingTypeKey]; ok {
		ga.RoPEScalingType = v.ValueString()
	}
	if v, ok := m[ropeScalingFactorKey]; ok {
		ga.RoPEScalingFactor = ValueNumeric[float32](v)
	}
	if v, ok := m[ropeScalingOriginalContextKey]; ok {
		ga.RoPEScalingOriginalContextLength = ValueNumeric[uint64](v)
	}
	if v, ok := m[ropeScalingFinetunedKey]; ok {
		ga.RoPEScalingFinetuned = v.ValueBool()
	}

	if v, ok := m[ssmConvolutionKernelKey]; ok {
		ga.SSMConvolutionKernel = ValueNumeric[uint32](v)
	}
	if v, ok := m[ssmInnerSizeKey]; ok {
		ga.SSMInnerSize = ValueNumeric[uint32](v)
	}
	if v, ok := m[ssmStateSizeKey]; ok {
		ga.SSMStateSize = ValueNumeric[uint32](v)
	}
	if v, ok := m[ssmTimeStepRankKey]; ok {
		ga.SSMTimeStepRank = ValueNumeric[uint32](v)
	}

	if v, ok := m[vocabularyLengthKey]; ok {
		ga.VocabularyLength = ValueNumeric[uint64](v)
	} else if v, ok := m[tokenizerGGMLTokensKey]; ok {
		ga.VocabularyLength = v.ValueArray().Len
	}

	{
		if ga.AttentionHeadCountKV > 0 {
			ga.EmbeddingGQA = ga.AttentionHeadCount / ga.AttentionHeadCountKV
		}
		if ga.AttentionHeadCount > 0 {
			ga.EmbeddingKeyGQA = uint64(ga.AttentionKeyLength) * ga.AttentionHeadCountKV
			ga.EmbeddingValueGQA = uint64(ga.AttentionValueLength) * ga.AttentionHeadCountKV
		}
		if ga.Architecture == "mamba" {
			ga.EmbeddingKeyGQA = uint64((ga.SSMConvolutionKernel - 1) * ga.SSMInnerSize)
			ga.EmbeddingValueGQA = uint64(ga.SSMStateSize * ga.SSMInnerSize)
		}
	}

	return ga
}
