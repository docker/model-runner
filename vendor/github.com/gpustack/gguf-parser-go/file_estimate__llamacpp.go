package gguf_parser

import (
	"regexp"
	"slices"
	"strings"

	"github.com/gpustack/gguf-parser-go/util/anyx"
	"github.com/gpustack/gguf-parser-go/util/ptr"
	"github.com/gpustack/gguf-parser-go/util/slicex"
)

// Types for LLaMACpp estimation.
type (
	// LLaMACppRunEstimate represents the estimated result of loading the GGUF file in llama.cpp.
	LLaMACppRunEstimate struct {
		// Type describes what type this GGUF file is.
		Type string `json:"type"`
		// Architecture describes what architecture this GGUF file implements.
		//
		// All lowercase ASCII.
		Architecture string `json:"architecture"`
		// ClipProjectorType is the type of the projector used in the clip model.
		//
		// Only used when Architecture is "clip".
		ClipProjectorType string `json:"clipProjectorType,omitempty"`
		// AdapterType is the type of the adapter.
		//
		// Only used when Architecture is "adapter".
		AdapterType string `json:"adapterType,omitempty"`
		// FlashAttention is the flag to indicate whether enable the flash attention,
		// true for enable.
		FlashAttention bool `json:"flashAttention"`
		// ContextSize is the size of the context.
		ContextSize uint64 `json:"contextSize"`
		// OffloadLayers is the number of offloaded layers.
		OffloadLayers uint64 `json:"offloadLayers"`
		// FullOffloaded is the flag to indicate whether the layers are fully offloaded,
		// false for partial offloaded or zero offloaded.
		FullOffloaded bool `json:"fullOffloaded"`
		// NoMMap is the flag to indicate whether support the mmap,
		// true for support.
		NoMMap bool `json:"noMMap"`
		// EmbeddingOnly is the flag to indicate whether the model is used for embedding only,
		// true for embedding only.
		EmbeddingOnly bool `json:"embeddingOnly"`
		// Reranking is the flag to indicate whether the model is used for reranking,
		// true for reranking.
		//
		// Only available when EmbeddingOnly is true.
		Reranking bool `json:"reranking"`
		// Distributable is the flag to indicate whether the model is distributable,
		// true for distributable.
		Distributable bool `json:"distributable"`
		// LogicalBatchSize is the logical batch size.
		LogicalBatchSize int32 `json:"logicalBatchSize"`
		// PhysicalBatchSize is the physical batch size.
		PhysicalBatchSize int32 `json:"physicalBatchSize"`
		// Devices represents the usage for running the GGUF file,
		// the first device is the CPU, and the rest are GPUs.
		Devices []LLaMACppRunDeviceUsage `json:"devices"`
		// Drafter is the estimated result of drafter.
		Drafter *LLaMACppRunEstimate `json:"drafter,omitempty"`
		// Projector is the estimated result of multimodal projector.
		Projector *LLaMACppRunEstimate `json:"projector,omitempty"`
		// Adapters is the estimated result of adapters.
		Adapters []LLaMACppRunEstimate `json:"adapters,omitempty"`
		// MaximumTokensPerSecond represents the maximum tokens per second for running the GGUF file.
		MaximumTokensPerSecond *GGUFTokensPerSecondScalar `json:"maximumTokensPerSecond,omitempty"`
	}

	// LLaMACppRunDeviceUsage represents the usage for running the GGUF file in llama.cpp.
	LLaMACppRunDeviceUsage struct {
		// HandleLayers is the number of layers that the device can handle.
		HandleLayers uint64 `json:"handleLayers"`
		// HandleLastLayer is the index of the last layer the device can handle.
		HandleLastLayer int `json:"handleLastLayer"`
		// HandleOutputLayer is the flag to indicate whether the device can handle the output layer,
		// true for handle.
		HandleOutputLayer bool `json:"handleOutputLayer"`
		// Remote is the flag to indicate whether the device is remote,
		// true for remote.
		Remote bool `json:"remote"`
		// Position is the relative position of the device,
		// starts from 0.
		//
		// If Remote is true, Position is the position of the remote devices,
		// Otherwise, Position is the position of the device in the local devices.
		Position int `json:"position"`
		// Footprint is the memory footprint for bootstrapping.
		Footprint GGUFBytesScalar `json:"footprint"`
		// Parameter is the running parameters that the device processes.
		Parameter LLaMACppParameterUsage `json:"parameter"`
		// Weight is the memory usage of weights that the device loads.
		Weight LLaMACppWeightMemoryUsage `json:"weight"`
		// KVCache is the memory usage of kv that the device caches.
		KVCache LLaMACppKVCacheMemoryUsage `json:"kvCache"`
		// Computation is the memory usage of computation that the device processes.
		Computation LLaMACppComputationMemoryUsage `json:"computation"`
	}

	// LLaMACppParameterUsage represents the parameter usage for running the GGUF file in llama.cpp.
	LLaMACppParameterUsage struct {
		// KVCache is the parameter usage for caching previous KV.
		KVCache GGUFParametersScalar `json:"kvCache"`
		// Input is the parameter usage for input tensors.
		Input GGUFParametersScalar `json:"input"`
		// Compute is the parameter usage for compute tensors.
		Compute GGUFParametersScalar `json:"compute"`
		// Output is the parameter usage for output tensors.
		Output GGUFParametersScalar `json:"output"`
	}

	// LLaMACppWeightMemoryUsage represents the memory usage of loading weights in llama.cpp.
	LLaMACppWeightMemoryUsage struct {
		// Input is the memory usage for loading input tensors.
		Input GGUFBytesScalar `json:"input"`
		// Compute is the memory usage for loading compute tensors.
		Compute GGUFBytesScalar `json:"compute"`
		// Output is the memory usage for loading output tensors.
		Output GGUFBytesScalar `json:"output"`
	}

	// LLaMACppKVCacheMemoryUsage represents the memory usage of caching previous KV in llama.cpp.
	LLaMACppKVCacheMemoryUsage struct {
		// Key is the memory usage for caching previous keys.
		Key GGUFBytesScalar `json:"key"`
		// Value is the memory usage for caching previous values.
		Value GGUFBytesScalar `json:"value"`
	}

	// LLaMACppComputationMemoryUsage represents the memory usage of computation in llama.cpp.
	LLaMACppComputationMemoryUsage struct {
		// Footprint is the memory footprint for computation.
		Footprint GGUFBytesScalar `json:"footprint"`
		// Input is the memory usage for input.
		Input GGUFBytesScalar `json:"input"`
		// Compute is the memory usage for computation.
		Compute GGUFBytesScalar `json:"graph"`
		// Output is the memory usage for output.
		Output GGUFBytesScalar `json:"output"`
	}
)

// EstimateLLaMACppRun returns the inference estimated result of the GGUF file.
func (gf *GGUFFile) EstimateLLaMACppRun(opts ...GGUFRunEstimateOption) (e LLaMACppRunEstimate) {
	// Options
	var o _GGUFRunEstimateOptions
	for _, opt := range opts {
		opt(&o)
	}
	switch {
	case o.TensorSplitFraction == nil:
		o.TensorSplitFraction = []float64{1}
		o.MainGPUIndex = 0
	case o.MainGPUIndex < 0 || o.MainGPUIndex >= len(o.TensorSplitFraction):
		panic("main device index must be range of 0 to the length of tensor split fraction")
	}
	if len(o.DeviceMetrics) > 0 {
		for i, j := 0, len(o.DeviceMetrics)-1; i < len(o.TensorSplitFraction)-j; i++ {
			o.DeviceMetrics = append(o.DeviceMetrics, o.DeviceMetrics[j])
		}
		o.DeviceMetrics = o.DeviceMetrics[:len(o.TensorSplitFraction)+1]
	}
	if o.LMCCacheKeyType == nil {
		o.LMCCacheKeyType = ptr.To(GGMLTypeF16)
	}
	if o.LMCCacheValueType == nil {
		o.LMCCacheValueType = ptr.To(GGMLTypeF16)
	}
	if o.LMCOffloadKVCache == nil {
		o.LMCOffloadKVCache = ptr.To(true)
	}
	if o.LMCLogicalBatchSize == nil {
		o.LMCLogicalBatchSize = ptr.To(int32(2048))
	} else {
		// See https://github.com/ggerganov/llama.cpp/blob/0bf16de07b0692e7df26b9a633e232bbd66e0360/src/llama.cpp#L16519-L16525.
		o.LMCLogicalBatchSize = ptr.To(max(32, *o.LMCLogicalBatchSize))
	}
	if o.LMCPhysicalBatchSize == nil {
		o.LMCPhysicalBatchSize = ptr.To(int32(512))
	}
	if *o.LMCPhysicalBatchSize > *o.LMCLogicalBatchSize {
		panic("physical batch size must be less than or equal to logical batch size")
	}
	if o.LMCSplitMode >= _LLAMACppSplitModeMax {
		panic("split mode must be less than max")
	}

	// Devices.
	e.Devices = make([]LLaMACppRunDeviceUsage, len(o.TensorSplitFraction)+1)
	for i := range e.Devices {
		e.Devices[i].HandleLastLayer = -1
	}
	for j := range e.Devices[1:] {
		e.Devices[j+1].Remote = j < len(o.RPCServers)
		if e.Devices[j+1].Remote {
			e.Devices[j+1].Position = j
		} else {
			e.Devices[j+1].Position = j - len(o.RPCServers)
		}
	}

	// Metadata.
	a := gf.Architecture()
	e.Type = a.Type
	e.Architecture = a.Architecture
	e.ClipProjectorType = a.ClipProjectorType
	e.AdapterType = a.AdapterType

	switch a.Type {
	case "model":
		t := gf.Tokenizer()
		gf.estimateLLaMACppRunInModel(&o, &a, &t, &e)
	case "projector":
		// For projector model,
		// see https://github.com/ggerganov/llama.cpp/blob/148ec970b62c3c5ae0a8bfdaad2fc237aaae350d/examples/llava/clip.cpp#L994-L1008.
		if ptr.Deref(o.LMCOffloadLayers, a.BlockCount) != 0 {
			// None model means full offload.
			o.LMCOffloadLayers = ptr.To(a.BlockCount)
		} else {
			// None model means zero offload.
			o.LMCOffloadLayers = ptr.To[uint64](0)
		}
		gf.estimateLLaMACppRunInProjector(&o, &a, &e)
	case "adapter":
		gf.estimateLLaMaCppRunInAdapter(&o, &a, &e)
	}

	return e
}

// estimateLLaMACppRunInModel estimates the inference result of the GGUF file in llama.cpp for model type,
// including the usages of footprint, weight, KV cache, and computation.
func (gf *GGUFFile) estimateLLaMACppRunInModel(o *_GGUFRunEstimateOptions, a *GGUFArchitecture, t *GGUFTokenizer, e *LLaMACppRunEstimate) {
	ls := gf.Layers()
	ioLs, tfLs, _ := ls.Cut([]string{
		"position_*",
		"token_*",
		"cls.*",
		"output.*",
		"output_*",
		"rope_factors_*",
	})
	ipLs, opLs, _ := ioLs.Cut([]string{
		"position_*",
		"token_*",
	})

	if a.BlockCount == 0 {
		a.BlockCount = uint64(len(tfLs))
	}

	// Full offload: nLoadLayers == 0 && isOffloadOutputLayer
	// Zero offload: nOffloadLayers == 0
	// Partial offload: !Full offload && !Zero offload
	var (
		nOffloadLayers       uint64
		nActualOffloadLayers uint64
		nLoadLayers          = a.BlockCount
		idxOutputDevice      int

		fullOffload, zeroOffload bool
	)
	{
		var isOffloadOutputLayer bool

		switch v := o.LMCOffloadLayers; {
		case v == nil:
			o.LMCOffloadLayers = ptr.To(a.BlockCount)
			nOffloadLayers = a.BlockCount
			isOffloadOutputLayer = true
		case *v != 0:
			nOffloadLayers = *v
			if nOffloadLayers > a.BlockCount {
				isOffloadOutputLayer = true
				nOffloadLayers = a.BlockCount
			}
		}
		nActualOffloadLayers = nOffloadLayers
		if isOffloadOutputLayer {
			nActualOffloadLayers += 1
		}
		nLoadLayers -= nOffloadLayers

		fullOffload = nLoadLayers == 0 && isOffloadOutputLayer
		zeroOffload = nOffloadLayers == 0

		e.FullOffloaded = fullOffload
		e.OffloadLayers = nOffloadLayers

		for i, j, offloadStart := 0, 0, len(tfLs)-int(nOffloadLayers); i < len(tfLs); i++ {
			switch {
			case i < int(nLoadLayers):
				e.Devices[0].HandleLayers += 1
				e.Devices[0].HandleLastLayer = i
			case i >= offloadStart:
				x := float64(i-offloadStart) / float64(nActualOffloadLayers)
				j = slicex.UpperBound(o.TensorSplitFraction, x)
				e.Devices[j+1].HandleLayers += 1
				e.Devices[j+1].HandleLastLayer = i
				if fullOffload && i == len(tfLs)-1 {
					idxOutputDevice = j + 1
				}
			}
		}

		e.Devices[idxOutputDevice].HandleOutputLayer = true
	}

	// Flash attention.
	{
		// Grok is not compatible with flash attention,
		// see https://github.com/ggerganov/llama.cpp/blob/19d3c8293b1f61acbe2dab1d49a17950fd788a4a/src/llama.cpp#L9566-L9569.
		if a.Architecture == "grok" {
			o.FlashAttention = false
		}
		// Attention key length must be equal to attention value length,
		// see https://github.com/ggerganov/llama.cpp/blob/19d3c8293b1f61acbe2dab1d49a17950fd788a4a/src/llama.cpp#L9571-L9574.
		if a.AttentionKeyLength != a.AttentionValueLength {
			o.FlashAttention = false
		}
		// Fallback to FP16 if the value type is quantized when disabling flash attention,
		// see https://github.com/ggerganov/llama.cpp/blob/19d3c8293b1f61acbe2dab1d49a17950fd788a4a/src/llama.cpp#L9576-L9579.
		if o.LMCCacheValueType.IsQuantized() && !o.FlashAttention {
			o.LMCCacheValueType = ptr.To(GGMLTypeF16)
		}

		e.FlashAttention = o.FlashAttention
	}

	// Embedding.
	if !a.AttentionCausal {
		e.EmbeddingOnly = true
		// Set context size/physical batch size/logical batch size to the training context size.
		o.LMCContextSize = ptr.To(min(int32(a.MaximumContextLength), ptr.Deref(o.LMCContextSize, int32(a.MaximumContextLength))))
		o.LMCLogicalBatchSize = o.LMCContextSize
		o.LMCPhysicalBatchSize = o.LMCLogicalBatchSize
		// Reranking.
		if _, found := gf.TensorInfos.Index([]string{"cls.bias", "cls.weight"}); found > 0 {
			e.Reranking = true
		}
	}

	// Distributable,
	// fix by https://github.com/ggerganov/llama.cpp/pull/11047.
	e.Distributable = true

	// Batch size.
	e.LogicalBatchSize = *o.LMCLogicalBatchSize
	e.PhysicalBatchSize = *o.LMCPhysicalBatchSize

	// Init hyperparameters,
	// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L6957-L7000.
	var (
		nContext  uint64
		nTokens   uint64
		nBatch    uint64
		nOutputs  uint64
		nParallel uint64
		nKV       uint64
	)
	{
		nContext = a.MaximumContextLength
		if o.LMCContextSize != nil {
			nContext = uint64(*o.LMCContextSize)
		}
		if o.LMCInMaxContextSize {
			nContext = min(nContext, a.MaximumContextLength)
		}
		// Padding context size,
		// see https://github.com/ggerganov/llama.cpp/blob/278d0e18469aacf505be18ce790a63c7cc31be26/src/llama.cpp#L19001-L19002.
		if o.FlashAttention {
			nContext = GGMLPadding(nContext, 256)
		} else {
			nContext = GGMLPadding(nContext, 32)
		}
		// Correct token size,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L12221-L12224.
		nTokens = min(nContext, uint64(*o.LMCPhysicalBatchSize))
		nBatch = nTokens
		nOutputs = nTokens
		nParallel = uint64(ptr.Deref(o.ParallelSize, 1))
		nKV = nContext

		// For mamba,
		// see https://github.com/ggerganov/llama.cpp/blob/7672adeec7a79ea271058c63106c142ba84f951a/llama.cpp#L16122-L16129.
		if a.Architecture == "mamba" {
			nKV = nParallel
			o.LMCCacheKeyType = ptr.To(GGMLTypeF32)
			o.LMCCacheValueType = ptr.To(GGMLTypeF32)
		}

		e.ContextSize = nContext
	}

	// Footprint.
	{
		// Bootstrap.
		e.Devices[0].Footprint = GGUFBytesScalar(5*1024*1024) /* model load */ + (gf.Size - gf.ModelSize) /* metadata */

		// Tokens,
		// https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L6380-L6384.
		fp := t.TokensLength * (4 /* token type */ + 4 /* token score*/)
		if t.Model == "gpt2" {
			fp += t.MergesLength * (48 /* key type */ + 56 /* value type */)
		}
		fp += t.TokensLength * (32 /* id to token vector */ + (24 + 32) /* token to id map*/)
		e.Devices[0].Footprint += GGUFBytesScalar(fp)

		// Output buffer,
		// see https://github.com/ggerganov/llama.cpp/blob/7672adeec7a79ea271058c63106c142ba84f951a/llama.cpp#L11940-L12003.
		ob := 4 /* float32 size */ * (a.VocabularyLength + a.EmbeddingLength) * nParallel
		if fullOffload {
			e.Devices[idxOutputDevice].Footprint += GGUFBytesScalar(ob)
		} else {
			e.Devices[0].Footprint += GGUFBytesScalar(ob)
		}
	}

	// Weight & Parameter.
	{
		// Compute.
		for i, j, offloadStart := 0, 0, len(tfLs)-int(nOffloadLayers); i < len(tfLs); i++ {
			idx := 0
			if i >= offloadStart {
				x := float64(i-offloadStart) / float64(nActualOffloadLayers)
				j = slicex.UpperBound(o.TensorSplitFraction, x)
				idx = j + 1
			}
			e.Devices[idx].Weight.Compute += GGUFBytesScalar(tfLs[i].Bytes())
			e.Devices[idx].Parameter.Compute += GGUFParametersScalar(tfLs[i].Elements())
		}

		// IO,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L4930-L5002.
		e.Devices[0].Weight.Input = GGUFBytesScalar(ipLs.Bytes())
		e.Devices[0].Parameter.Input = GGUFParametersScalar(ipLs.Elements())
		var (
			wg GGUFBytesScalar
			ps GGUFParametersScalar
		)
		if _, ok := opLs.Get("output.weight"); ok {
			wg = GGUFBytesScalar(opLs.Bytes())
			ps = GGUFParametersScalar(opLs.Elements())
		} else if a.AttentionCausal {
			wg = GGUFBytesScalar(opLs.Bytes()) + e.Devices[0].Weight.Input /* duplicate the input layer */
			ps = GGUFParametersScalar(opLs.Elements() + ipLs.Elements())
		}
		e.Devices[0].Weight.Output = wg
		if fullOffload {
			e.Devices[idxOutputDevice].Weight.Output = wg
			e.Devices[idxOutputDevice].Parameter.Output = ps
		} else {
			e.Devices[0].Parameter.Output = ps
		}
	}

	// KV cache,
	// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L2479-L2501.
	{
		kps, vps := a.EmbeddingKeyGQA*nKV, a.EmbeddingValueGQA*nKV
		krs, vrs := o.LMCCacheKeyType.RowSizeOf([]uint64{kps}), o.LMCCacheValueType.RowSizeOf([]uint64{vps})

		e.Devices[0].KVCache.Key = GGUFBytesScalar(krs * nLoadLayers)
		e.Devices[0].KVCache.Value = GGUFBytesScalar(vrs * nLoadLayers)
		e.Devices[0].Parameter.KVCache = GGUFParametersScalar((kps + vps) * nLoadLayers)
		if !*o.LMCOffloadKVCache {
			e.Devices[0].KVCache.Key += GGUFBytesScalar(krs * nOffloadLayers)
			e.Devices[0].KVCache.Value += GGUFBytesScalar(vrs * nOffloadLayers)
			e.Devices[0].Parameter.KVCache += GGUFParametersScalar((kps + vps) * nOffloadLayers)
		} else if !zeroOffload {
			for i, d := range e.Devices[1:] {
				e.Devices[i+1].KVCache.Key = GGUFBytesScalar(krs * d.HandleLayers)
				e.Devices[i+1].KVCache.Value = GGUFBytesScalar(vrs * d.HandleLayers)
				e.Devices[i+1].Parameter.KVCache = GGUFParametersScalar((kps + vps) * d.HandleLayers)
			}
		}
	}

	// Computation.
	{
		// Bootstrap, compute metadata,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L16135-L16136.
		cm := GGMLTensorOverhead()*GGMLComputationGraphNodesMaximum +
			GGMLComputationGraphOverhead(GGMLComputationGraphNodesMaximum, false)
		e.Devices[0].Computation.Footprint = GGUFBytesScalar(cm)

		// Scheduler overhead,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L16149.
		e.Devices[0].Computation.Footprint += GGUFBytesScalar(4 * 1024 * 1024)

		// GGML context,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L5015-L5036.
		gc := 2 /* buffer count */ * GGMLTensorOverhead() * (uint64(len(gf.TensorInfos)) + 1 + a.BlockCount*3)
		e.Devices[0].Computation.Footprint += GGUFBytesScalar(gc)

		// Tensor usage,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L16149.
		//
		// First, get the usage of input layer,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L2279-L2290.
		var (
			inpTokens = GGMLTypeI32.RowSizeOf([]uint64{nBatch})                    // I32 [n_batch]
			inpEmbd   = GGMLTypeF32.RowSizeOf([]uint64{a.EmbeddingLength, nBatch}) // F32 [n_embd, n_batch]
			inpPos    = GGMLTypeI32.RowSizeOf([]uint64{nBatch})                    // I32 [n_batch]
			inpOutIds = GGMLTypeI32.RowSizeOf([]uint64{nOutputs})                  // I32 [n_outputs],
			inpKQMask = GGMLTypeF32.RowSizeOf([]uint64{nKV, nBatch})               // F32 [n_kv, n_batch]
			inpSMask  = GGMLTypeF32.RowSizeOf([]uint64{1, nKV})                    // F32 [1, n_kv]
			inpSSeq   = GGMLTypeI32.RowSizeOf([]uint64{nKV, nBatch})               // I32 [n_kv, n_batch]
		)
		switch {
		case a.Architecture == "mamba":
			e.Devices[0].Computation.Input = GGUFBytesScalar(inpTokens + inpEmbd + inpSMask + inpSSeq + inpOutIds)
		default:
			e.Devices[0].Computation.Input = GGUFBytesScalar(inpTokens + inpEmbd + inpPos + inpKQMask + inpOutIds)
		}
		if !zeroOffload {
			var v GGUFBytesScalar
			switch {
			case a.Architecture == "mamba":
				v = GGUFBytesScalar(inpEmbd + inpSMask + inpSSeq)
			default:
				v = GGUFBytesScalar(inpEmbd + inpPos + inpKQMask)
			}
			if len(o.RPCServers) == 0 && len(o.TensorSplitFraction) > 1 {
				if a.ExpertCount > 0 {
					v *= 2
				} else {
					v *= 4
				}
			}
			for i := range e.Devices[1:] {
				e.Devices[i+1].Computation.Input += v
			}
		}
		// Since the steps between transformer layers are serial,
		// the allocated memory can be reused for the next layer.
		// So, we only consider the usage of the largest layer,
		// which is the last layer by default.
		switch {
		case a.Architecture == "mamba":
			convInc := GGMLTypeF32.RowSizeOf([]uint64{a.EmbeddingKeyGQA, nKV}) // F32 [n_embd_key_gqa, n_kv] reshape
			for _, l := range tfLs[len(tfLs)-1].Search(regexp.MustCompile(`.*\.\d+\.(attn_norm|ssm_in|ssm_conv1d)\.weight`)) {
				if !strings.HasSuffix(l.Name, ".ssm_conv1d.weight") {
					rs := GGMLTypeF32.RowSizeOf([]uint64{l.Dimensions[l.NDimensions-1], nTokens})
					convInc += rs
					continue
				}
				// https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L10379.
				rs := GGMLTypeF32.RowSizeOf([]uint64{uint64(a.SSMInnerSize)*nTokens + uint64(a.SSMConvolutionKernel)*uint64(a.SSMInnerSize)*nKV})
				convInc += rs
			}
			ssmInc := uint64(0)
			for _, l := range tfLs[len(tfLs)-1].Search(regexp.MustCompile(`.*\.\d+\.ssm_(dt\.weight|a)`)) {
				if !strings.HasSuffix(l.Name, ".ssm_a") {
					rs := GGMLTypeF32.RowSizeOf([]uint64{l.Dimensions[l.NDimensions-1], nTokens})
					ssmInc += rs
					continue
				}
				// https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L10413.
				rs := GGMLTypeF32.RowSizeOf([]uint64{uint64(a.SSMInnerSize)*nTokens + uint64(a.SSMStateSize)*uint64(a.SSMInnerSize)*nKV})
				ssmInc += rs
			}
			cp := GGUFBytesScalar(convInc + ssmInc)
			for i := range e.Devices[1:] {
				e.Devices[i+1].Computation.Compute = cp
			}
		default:
			loadAttnInc, offloadAttnInc := uint64(0), uint64(0)
			{
				rs := o.LMCCacheKeyType.RowSizeOf([]uint64{uint64(a.AttentionKeyLength), nKV, a.AttentionHeadCountKV})
				loadAttnInc = rs // k-?
				rs = o.LMCCacheValueType.RowSizeOf([]uint64{uint64(a.AttentionValueLength), nKV, a.AttentionHeadCountKV})
				loadAttnInc += rs // v-?
			}
			if o.FlashAttention {
				// https://github.com/ggerganov/llama.cpp/blob/172c8256840ffd882ab9992ecedbb587d9b21f15/llama.cpp#L7387.
				offloadAttnInc = GGMLTypeF16.RowSizeOf([]uint64{nKV, nTokens})
				for _, l := range tfLs[len(tfLs)-1].Search(regexp.MustCompile(`.*\.\d+\.attn_(norm|q|qkv)\.weight`)) {
					if strings.HasSuffix(l.Name, ".attn_norm.weight") {
						rs := GGMLTypeF32.RowSizeOf([]uint64{l.Dimensions[l.NDimensions-1], nTokens})
						offloadAttnInc += rs
						continue
					}
					rs := l.Bytes()
					offloadAttnInc += rs
				}
				// https://github.com/ggerganov/llama.cpp/blob/172c8256840ffd882ab9992ecedbb587d9b21f15/llama.cpp#L6986-L6992.
				rs := o.LMCCacheKeyType.RowSizeOf([]uint64{uint64(a.AttentionKeyLength), nKV, a.AttentionHeadCountKV})
				offloadAttnInc += rs
				// https://github.com/ggerganov/llama.cpp/blob/172c8256840ffd882ab9992ecedbb587d9b21f15/llama.cpp#L7000-L7007.
				rs = o.LMCCacheValueType.RowSizeOf([]uint64{uint64(a.AttentionValueLength), nKV, a.AttentionHeadCountKV})
				offloadAttnInc += rs
			} else {
				offloadAttnInc = uint64(0)
				for _, l := range tfLs[len(tfLs)-1].Search(regexp.MustCompile(`.*\.\d+\.attn_(norm|q|qkv|q_b)\.weight`)) {
					var rs uint64
					switch {
					default: // norm.
						rs = GGMLTypeF32.RowSizeOf([]uint64{l.Dimensions[l.NDimensions-1], nTokens})
						offloadAttnInc += rs
					case strings.HasSuffix(l.Name, ".attn_q.weight"):
						rs = GGMLTypeF32.RowSizeOf([]uint64{l.Dimensions[0], nTokens})
						offloadAttnInc += rs * 2 // Qcur.
						rs = GGMLTypeF32.RowSizeOf([]uint64{nKV, nTokens, a.AttentionHeadCount})
						offloadAttnInc += rs // kq.
						if !zeroOffload && !fullOffload {
							offloadAttnInc += loadAttnInc
						}
					case strings.HasSuffix(l.Name, ".attn_qkv.weight"):
						rs = GGMLTypeF32.RowSizeOf([]uint64{l.Dimensions[0], nTokens})
						offloadAttnInc += rs * 2 // Qcur.
						rs = GGMLTypeF32.RowSizeOf([]uint64{nKV, nTokens, a.AttentionHeadCount})
						offloadAttnInc += rs // kq.
						rs = GGMLTypeF32.RowSizeOf([]uint64{a.EmbeddingLength, a.EmbeddingLength * 3})
						offloadAttnInc += rs // wqkv.
						if !zeroOffload && !fullOffload {
							offloadAttnInc += loadAttnInc
						}
					case strings.HasSuffix(l.Name, ".attn_q_b.weight"):
						rs = GGMLTypeF32.RowSizeOf([]uint64{l.Dimensions[l.NDimensions-1], nTokens})
						offloadAttnInc += rs * 2 // q-?
						rs = GGMLTypeF32.RowSizeOf([]uint64{nKV, nTokens, a.AttentionHeadCount})
						offloadAttnInc += rs // kq.
					}
				}
			}
			ffnInc := uint64(0)
			for _, l := range tfLs[len(tfLs)-1].Search(regexp.MustCompile(`.*\.\d+\.(attn_norm|ffn_norm|ffn_gate|ffn_up)\.weight`)) {
				rs := GGMLTypeF32.RowSizeOf([]uint64{l.Dimensions[l.NDimensions-1], nTokens})
				ffnInc += rs
			}
			if a.ExpertCount > 0 || a.ExpertUsedCount > 0 {
				rs := GGMLTypeF32.RowSizeOf([]uint64{uint64(a.ExpertCount), a.EmbeddingLength})
				ffnInc += rs // ffn_gate_input
				rs = GGMLTypeF32.RowSizeOf([]uint64{uint64(a.ExpertCount), nTokens})
				ffnInc += rs // ffn_moe_logits
				rs = GGMLTypeF32.RowSizeOf([]uint64{a.EmbeddingLength, uint64(a.ExpertUsedCount), nTokens})
				ffnInc += rs // ffn_moe_down
			}
			if !zeroOffload {
				e.Devices[0].Computation.Compute = GGUFBytesScalar(loadAttnInc + ffnInc)
			} else {
				e.Devices[0].Computation.Compute = GGUFBytesScalar(loadAttnInc)
			}
			if !zeroOffload {
				cp := GGUFBytesScalar(max(offloadAttnInc, ffnInc))
				for i := range e.Devices[1:] {
					e.Devices[i+1].Computation.Compute = cp
				}
				if nLoadLayers > 1 {
					for i := range e.Devices[1:] {
						if e.Devices[i+1].Remote {
							continue
						}
						e.Devices[i+1].Computation.Compute += GGUFBytesScalar(loadAttnInc)
						break
					}
				}
			}
		}
		// Finally, get the usage of output layer.
		if a.AttentionCausal {
			var outInc uint64
			if a.Architecture == "mamba" {
				outInc += inpSMask + inpSSeq
			}
			if l, ok := opLs.Get("output.weight"); ok {
				rs := GGMLTypeF32.RowSizeOf([]uint64{l.Dimensions[l.NDimensions-1], nTokens})
				outInc += rs
			} else if l, ok := ipLs.Get("token_embd.weight"); ok {
				rs := GGMLTypeF32.RowSizeOf([]uint64{l.Dimensions[l.NDimensions-1], nTokens})
				outInc += rs
			}
			e.Devices[idxOutputDevice].Computation.Output += GGUFBytesScalar(outInc)
		}
	}

	// Drafter.
	e.Drafter = o.LMCDrafter

	// Projector.
	e.Projector = o.LMCProjector

	// Adapters.
	e.Adapters = o.LMCAdapters

	// Maximum tokens per second.
	if ds, dmss := e.Devices, o.DeviceMetrics; len(dmss) != 0 {
		ltss := make([]float64, len(dmss))
		bs := anyx.Number[float64](*o.LMCLogicalBatchSize) / float64(nBatch)
		for i, dm := range dmss {
			fl, upbw, dwbw := float64(max(dm.FLOPS, 1)), float64(max(dm.UpBandwidth, 1)), float64(max(dm.DownBandwidth, 1))
			cmpops := float64(ds[i].Parameter.Compute)*2 /* FMA */ *bs + float64(ds[i].Parameter.Input) + float64(ds[i].Parameter.Output)
			cmps := float64(ds[i].Weight.Sum())
			cmplat := max(cmpops/fl, cmps/upbw)
			kvcops := float64(ds[i].Parameter.KVCache) * 2 /* FMA */ * bs
			kvcs := float64(ds[i].KVCache.Sum()) * bs
			kvclat := max(kvcops/fl, kvcs/upbw)
			ffs := float64(GGMLTypeF32.RowSizeOf([]uint64{a.EmbeddingLength, nBatch}))
			ffslat := ffs / dwbw
			lays := float64(ds[i].HandleLayers)
			if ds[i].HandleOutputLayer {
				lays += 1
			}
			ltss[i] = (cmplat + kvclat + ffslat) * lays / float64(a.BlockCount+2)
		}
		lt := float64(0)
		ltmax := slices.Max(ltss)
		for i := range ltss {
			lt += ltss[i] / ltmax * ltss[i]
		}
		e.MaximumTokensPerSecond = ptr.To(GGUFTokensPerSecondScalar(1 / lt))
	}
}

func (gf *GGUFFile) estimateLLaMACppRunInProjector(o *_GGUFRunEstimateOptions, a *GGUFArchitecture, e *LLaMACppRunEstimate) {
	ls := gf.Layers()
	ioLs, tfLs, _ := ls.Cut([]string{
		"v.patch_embd.*",
		"v.class_embd",
		"v.position_embd.*",
		"v.pre_ln.*",
		"model.*",
		"v.post_ln.*",
		"mm.*",
		"resampler.*",
	})
	ipLs, opLs, _ := ioLs.Cut([]string{
		"v.patch_embd.*",
		"v.class_embd",
		"v.position_embd.*",
		"v.pre_ln.*",
		"model.*",
	})

	if a.BlockCount == 0 {
		a.BlockCount = uint64(len(tfLs))
	}

	e.FullOffloaded = *o.LMCOffloadLayers == a.BlockCount
	e.OffloadLayers = *o.LMCOffloadLayers

	// Init hyperparameters,
	// see https://github.com/ggerganov/llama.cpp/blob/0827b2c1da299805288abbd556d869318f2b121e/examples/llava/clip.cpp#L599-L636.
	var (
		imgHeightSize     uint64
		imgWidthSize      uint64
		imgPatchSize      uint64
		nPatchesHeight    uint64
		nPatchesWidth     uint64
		nPatches          uint64
		imgPatchesMaxSize uint64
		imgPatches        uint64
		projectionDim     uint64 // NB(thxCode): do not sure if there is the correct name.
	)
	{
		// See https://github.com/ggerganov/llama.cpp/blob/0827b2c1da299805288abbd556d869318f2b121e/examples/llava/llava.cpp#L397-L411,
		//     https://github.com/ggerganov/llama.cpp/blob/0827b2c1da299805288abbd556d869318f2b121e/examples/llava/clip.cpp#L2323-L2345,
		//     https://github.com/ggerganov/llama.cpp/blob/0827b2c1da299805288abbd556d869318f2b121e/examples/llava/clip.cpp#L2767-L2794.
		imgHeightSize = uint64(a.ClipVisionImageSize)
		imgWidthSize = imgHeightSize
		imgPatchSize = uint64(a.ClipVisionPatchSize)
		if a.ClipHasQwen2VLMerger {
			imgHeightSize = uint64(ptr.Deref(o.LMCVisualMaxImageSize, 224))
			imgWidthSize = imgHeightSize
		}
		nPatchesHeight = imgHeightSize / imgPatchSize
		nPatchesWidth = imgWidthSize / imgPatchSize
		nPatches = nPatchesHeight * nPatchesWidth
		imgPatchesMaxSize = 1
		imgPatches = nPatches
		switch {
		case a.ClipHasLLaVAProjector:
			// LLaVA 1.6 uses up to 6 patches
			if a.ClipVisionMMPatchMergeType != "flat" {
				imgPatchesMaxSize = 6
			}
		case a.ClipHasMiniCPMVProjector:
			// MiniCPM-V uses up to 10 patches
			imgPatchesMaxSize = 10
		case a.ClipProjectorType == "adapter":
			// Granite vision uses up to 10 patches + base patch
			imgPatchesMaxSize = 11
		}
		switch a.ClipProjectorType {
		case "ldp":
			imgPatches /= 4
			if ti, ok := gf.TensorInfos.Get("mm.model.mb_block.1.block.2.1.bias"); ok {
				projectionDim = ti.Dimensions[0]
			}
		case "ldpv2":
			imgPatches /= 4
			if ti, ok := gf.TensorInfos.Get("mm.model.peg.0.bias"); ok {
				projectionDim = ti.Dimensions[0]
			}
		case "mlp":
			if ti, ok := gf.TensorInfos.Get("mm.2.bias"); ok {
				projectionDim = ti.Dimensions[0]
			}
		case "mlp_norm":
			if ti, ok := gf.TensorInfos.Get("mm.3.bias"); ok {
				projectionDim = ti.Dimensions[0]
			}
		case "resampler":
			if ti, ok := gf.TensorInfos.Get("resampler.query"); ok {
				imgPatches = ti.Dimensions[1]
				projectionDim = ti.Dimensions[0]
			}
		case "adapter":
			if ti, ok := gf.TensorInfos.Get("adapter.linear.dense_4h_to_h.weight"); ok {
				projectionDim = ti.Dimensions[1]
			}
		case "qwen2vl_merger":
			nSizePatch := uint64(a.ClipVisionPatchSize * 2)
			imgHeightPatchSize := imgHeightSize / nSizePatch
			if imgHeightSize%nSizePatch > 0 {
				imgHeightPatchSize++
			}
			imgWidthPatchSize := imgWidthSize / nSizePatch
			if imgWidthSize%nSizePatch > 0 {
				imgWidthPatchSize++
			}
			imgPatches = imgHeightPatchSize * imgWidthPatchSize
			if ti, ok := gf.TensorInfos.Get("mm.2.bias"); ok {
				projectionDim = ti.Dimensions[0]
			}
		case "gemma3":
			if ti, ok := gf.TensorInfos.Get("mm.input_projection.weight"); ok {
				imgPatches = 256
				projectionDim = ti.Dimensions[0]
			}
		}
	}

	// Footprint.
	{
		// Bootstrap.
		e.Devices[0].Footprint = GGUFBytesScalar(5*1024*1024) /* model load */ + (gf.Size - gf.ModelSize) /* metadata */

		// Image Embed,
		// see https://github.com/ggerganov/llama.cpp/blob/0827b2c1da299805288abbd556d869318f2b121e/examples/llava/llava.cpp#L401-L407.
		e.Devices[0].Footprint += GGUFBytesScalar(imgPatchesMaxSize * imgPatches * projectionDim * 4 /* float32 size */)
	}

	idx := 0 // Default to the main host's RAM.
	if *o.LMCOffloadLayers != 0 {
		for i := 1; i < len(e.Devices); i++ {
			if !e.Devices[i].Remote {
				idx = i
				break
			}
		}
	}

	// Weight & Parameter.
	{
		// Compute.
		e.Devices[idx].HandleLayers = *o.LMCOffloadLayers
		e.Devices[idx].HandleLastLayer = int(e.Devices[idx].HandleLayers - 1)
		e.Devices[idx].Weight.Compute = GGUFBytesScalar(tfLs.Bytes())
		e.Devices[idx].Parameter.Compute = GGUFParametersScalar(tfLs.Elements())

		// IO.
		e.Devices[idx].Weight.Input = GGUFBytesScalar(ipLs.Bytes())
		e.Devices[idx].Parameter.Input = GGUFParametersScalar(ipLs.Elements())
		e.Devices[idx].Weight.Output = GGUFBytesScalar(opLs.Bytes())
		e.Devices[idx].Parameter.Output = GGUFParametersScalar(opLs.Elements())
	}

	// Computation.
	{
		// Bootstrap, compute metadata,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L16135-L16136.
		cm := GGMLTensorOverhead()*GGMLComputationGraphNodesMaximum +
			GGMLComputationGraphOverhead(GGMLComputationGraphNodesMaximum, false)
		e.Devices[0].Computation.Footprint = GGUFBytesScalar(cm)

		// Scheduler overhead,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L16149.
		e.Devices[0].Computation.Footprint += GGUFBytesScalar(4 * 1024 * 1024)

		// GGML context,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L5015-L5036.
		gc := 2 /* buffer count */ * GGMLTensorOverhead() * (uint64(len(gf.TensorInfos)) + 1 + a.BlockCount*3)
		e.Devices[0].Computation.Footprint += GGUFBytesScalar(gc)

		// Tensor usage.
		var (
			hasClassEmbd bool
			nPositions   uint64
			nPositionIDs uint64
			nBatch       uint64
			nEmbd        uint64
			nHead        uint64
		)
		{
			_, hasClassEmbd = ipLs.Get("v.class_embd")
			nPositions = nPatches
			if hasClassEmbd {
				nPositions += 1
			}
			nPositionIDs = nPositions
			if a.ClipHasQwen2VLMerger {
				nPositionIDs *= 4
			}
			nBatch = 1
			nEmbd = a.EmbeddingLength
			nHead = a.AttentionHeadCount
		}
		// First, get the usage of input layer.
		var (
			inpRaw     = GGMLTypeF32.RowSizeOf([]uint64{imgWidthSize, imgHeightSize, 3, nBatch})                // F32 [img_width, img_height, 3, n_batch]
			inpRawCnt  = GGMLTypeF32.RowSizeOf([]uint64{nPatches, nEmbd, nBatch})                               // I32 [n_patches, n_embd, n_batch]
			inpEmbd    = GGMLTypeF32.RowSizeOf([]uint64{nEmbd, nPositions, nBatch})                             // F32 [n_embd, n_positions, n_batch]
			inpPosEmbd = GGMLTypeF32.RowSizeOf([]uint64{projectionDim, nPatchesHeight * nPatchesWidth, nBatch}) // F32 [mmproj, pos_h * pos_w, n_batch]
			inpPos     = GGMLTypeI32.RowSizeOf([]uint64{nPositionIDs})                                          // I32 [n_positions]
			inpPatches = GGMLTypeI32.RowSizeOf([]uint64{nPatches})                                              // I32 [n_patches]
		)
		{
			e.Devices[idx].Computation.Input = GGUFBytesScalar(inpRaw + inpRawCnt + inpPos + inpPatches)
			if a.ClipHasMiniCPMVProjector {
				e.Devices[idx].Computation.Input += GGUFBytesScalar(inpPosEmbd)
			}
			if hasClassEmbd {
				e.Devices[idx].Computation.Input += GGUFBytesScalar(inpEmbd)
			}
		}
		// Since the steps between transformer layers are serial,
		// the allocated memory can be reused for the next layer.
		// So, we only consider the usage of a certain layer.
		{
			compNorm := GGMLTypeF32.RowSizeOf([]uint64{nEmbd, nPositions}) * 2
			compVcur := GGMLTypeF32.RowSizeOf([]uint64{nEmbd, nPositions})
			compKcur := GGMLTypeF32.RowSizeOf([]uint64{nEmbd, nPositions})
			compKQcur := GGMLTypeF32.RowSizeOf([]uint64{nPositions, nPositions, nHead})
			e.Devices[idx].Computation.Compute = GGUFBytesScalar(compNorm + compVcur + compKcur + compKQcur)
		}
	}
}

func (gf *GGUFFile) estimateLLaMaCppRunInAdapter(o *_GGUFRunEstimateOptions, a *GGUFArchitecture, e *LLaMACppRunEstimate) {
	ls := gf.Layers()
	ioLs, tfLs, _ := ls.Cut([]string{
		"position_*",
		"token_*",
		"cls.*",
		"output.*",
		"output_*",
	})
	ipLs, opLs, _ := ioLs.Cut([]string{
		"position_*",
		"token_*",
	})

	if a.BlockCount == 0 {
		a.BlockCount = uint64(len(tfLs))
	}

	// Full offload: nLoadLayers == 0 && isOffloadOutputLayer
	// Zero offload: nOffloadLayers == 0
	// Partial offload: !Full offload && !Zero offload
	var (
		nOffloadLayers       uint64
		nActualOffloadLayers uint64
		nLoadLayers          = a.BlockCount
		idxOutputDevice      int

		fullOffload bool
	)
	{
		var isOffloadOutputLayer bool

		switch v := o.LMCOffloadLayers; {
		case v == nil:
			o.LMCOffloadLayers = ptr.To(a.BlockCount)
			nOffloadLayers = a.BlockCount
			isOffloadOutputLayer = true
		case *v != 0:
			nOffloadLayers = *v
			if nOffloadLayers > a.BlockCount {
				isOffloadOutputLayer = true
				nOffloadLayers = a.BlockCount
			}
		}
		nActualOffloadLayers = nOffloadLayers
		if isOffloadOutputLayer {
			nActualOffloadLayers += 1
		}
		nLoadLayers -= nOffloadLayers

		fullOffload = nLoadLayers == 0 && isOffloadOutputLayer

		e.FullOffloaded = fullOffload
		e.OffloadLayers = nOffloadLayers

		for i, j, offloadStart := 0, 0, len(tfLs)-int(nOffloadLayers); i < len(tfLs); i++ {
			switch {
			case i < int(nLoadLayers):
				e.Devices[0].HandleLayers += 1
				e.Devices[0].HandleLastLayer = i
			case i >= offloadStart:
				x := float64(i-offloadStart) / float64(nActualOffloadLayers)
				j = slicex.UpperBound(o.TensorSplitFraction, x)
				e.Devices[j+1].HandleLayers += 1
				e.Devices[j+1].HandleLastLayer = i
				if fullOffload && i == len(tfLs)-1 {
					idxOutputDevice = j + 1
				}
			}
		}

		e.Devices[idxOutputDevice].HandleOutputLayer = true
	}

	// Distributable.
	e.Distributable = false

	// Footprint.
	{
		// Bootstrap.
		e.Devices[0].Footprint = GGUFBytesScalar(5*1024*1024) /* model load */ + (gf.Size - gf.ModelSize) /* metadata */
	}

	// Weight & Parameter.
	{
		// Compute.
		for i, j, offloadStart := 0, 0, len(tfLs)-int(nOffloadLayers); i < len(tfLs); i++ {
			idx := 0
			if i >= offloadStart {
				x := float64(i-offloadStart) / float64(nActualOffloadLayers)
				j = slicex.UpperBound(o.TensorSplitFraction, x)
				idx = j + 1
			}
			e.Devices[idx].Weight.Compute += GGUFBytesScalar(tfLs[i].Bytes())
			e.Devices[idx].Parameter.Compute += GGUFParametersScalar(tfLs[i].Elements())
		}

		// IO,
		// see https://github.com/ggerganov/llama.cpp/blob/d6ef0e77dd25f54fb5856af47e3926cf6f36c281/llama.cpp#L4930-L5002.
		e.Devices[0].Weight.Input = GGUFBytesScalar(ipLs.Bytes())
		e.Devices[0].Parameter.Input = GGUFParametersScalar(ipLs.Elements())
		var (
			wg GGUFBytesScalar
			ps GGUFParametersScalar
		)
		if _, ok := opLs.Get("output.weight"); ok {
			wg = GGUFBytesScalar(opLs.Bytes())
			ps = GGUFParametersScalar(opLs.Elements())
		} else if a.AttentionCausal {
			wg = GGUFBytesScalar(opLs.Bytes()) + e.Devices[0].Weight.Input /* duplicate the input layer */
			ps = GGUFParametersScalar(opLs.Elements() + ipLs.Elements())
		}
		e.Devices[0].Weight.Output = wg
		if fullOffload {
			e.Devices[idxOutputDevice].Weight.Output = wg
			e.Devices[idxOutputDevice].Parameter.Output = ps
		} else {
			e.Devices[0].Parameter.Output = ps
		}
	}
}

// Types for LLaMACpp estimated summary.
type (
	// LLaMACppRunEstimateSummary represents the summary of the usage for loading the GGUF file in llama.cpp.
	LLaMACppRunEstimateSummary struct {
		/* Basic */

		// Items
		Items []LLaMACppRunEstimateSummaryItem `json:"items"`

		/* Appendix */

		// Type describes what type this GGUF file is.
		Type string `json:"type"`
		// Architecture describes what architecture this GGUF file implements.
		//
		// All lowercase ASCII.
		Architecture string `json:"architecture"`
		// ClipProjectorType is the type of the projector used in the clip model.
		//
		// Only used when Architecture is "clip".
		ClipProjectorType string `json:"clipProjectorType,omitempty"`
		// AdapterType is the type of the adapter.
		//
		// Only used when Architecture is "adapter".
		AdapterType string `json:"adapterType,omitempty"`
		// ContextSize is the size of the context.
		ContextSize uint64 `json:"contextSize"`
		// FlashAttention is the flag to indicate whether enable the flash attention,
		// true for enable.
		FlashAttention bool `json:"flashAttention"`
		// NoMMap is the flag to indicate whether the file must be loaded without mmap,
		// true for total loaded.
		NoMMap bool `json:"noMMap"`
		// EmbeddingOnly is the flag to indicate whether the model is used for embedding only,
		// true for embedding only.
		EmbeddingOnly bool `json:"embeddingOnly"`
		// Reranking is the flag to indicate whether the model is used for reranking,
		// true for reranking.
		//
		// Only available when EmbeddingOnly is true.
		Reranking bool `json:"reranking"`
		// Distributable is the flag to indicate whether the model is distributable,
		// true for distributable.
		Distributable bool `json:"distributable"`
		// LogicalBatchSize is the logical batch size.
		LogicalBatchSize int32 `json:"logicalBatchSize"`
		// PhysicalBatchSize is the physical batch size.
		PhysicalBatchSize int32 `json:"physicalBatchSize"`
	}

	// LLaMACppRunEstimateSummaryItem represents one summary item for loading the GGUF file in llama.cpp.
	LLaMACppRunEstimateSummaryItem struct {
		// OffloadLayers is the number of offloaded layers.
		OffloadLayers uint64 `json:"offloadLayers"`
		// FullOffloaded is the flag to indicate whether the layers are fully offloaded,
		// false for partial offloaded or zero offloaded.
		FullOffloaded bool `json:"fullOffloaded"`
		// MaximumTokensPerSecond is the maximum tokens per second for running the GGUF file.
		MaximumTokensPerSecond *GGUFTokensPerSecondScalar `json:"maximumTokensPerSecond,omitempty"`
		// RAM is the memory usage for loading the GGUF file in RAM.
		RAM LLaMACppRunEstimateMemory `json:"ram"`
		// VRAMs is the memory usage for loading the GGUF file in VRAM per device.
		VRAMs []LLaMACppRunEstimateMemory `json:"vrams"`
	}

	// LLaMACppRunEstimateMemory represents the memory usage for loading the GGUF file in llama.cpp.
	LLaMACppRunEstimateMemory struct {
		// HandleLayers is the number of layers that the device can handle.
		HandleLayers uint64 `json:"handleLayers"`
		// HandleLastLayer is the index of the last layer the device can handle.
		HandleLastLayer int `json:"handleLastLayer"`
		// HandleOutputLayer is the flag to indicate whether the device can handle the output layer,
		// true for handle.
		HandleOutputLayer bool `json:"handleOutputLayer"`
		// Remote is the flag to indicate whether the device is remote,
		// true for remote.
		Remote bool `json:"remote"`
		// Position is the relative position of the device,
		// starts from 0.
		//
		// If Remote is true, Position is the position of the remote devices,
		// Otherwise, Position is the position of the device in the local devices.
		Position int `json:"position"`
		// UMA represents the usage of Unified Memory Architecture.
		UMA GGUFBytesScalar `json:"uma"`
		// NonUMA represents the usage of Non-Unified Memory Architecture.
		NonUMA GGUFBytesScalar `json:"nonuma"`
	}
)

// SummarizeItem returns the corresponding LLaMACppRunEstimateSummaryItem with the given options.
func (e LLaMACppRunEstimate) SummarizeItem(mmap bool, nonUMARamFootprint, nonUMAVramFootprint uint64) (emi LLaMACppRunEstimateSummaryItem) {
	emi.OffloadLayers, emi.FullOffloaded = e.OffloadLayers, e.FullOffloaded
	if emi.FullOffloaded {
		emi.OffloadLayers++ // The output layer is offloaded.
	}
	emi.MaximumTokensPerSecond = e.MaximumTokensPerSecond

	// RAM.
	{
		fp := e.Devices[0].Footprint
		wg := e.Devices[0].Weight.Sum()
		kv := e.Devices[0].KVCache.Sum()
		cp := e.Devices[0].Computation.Sum()

		emi.RAM.HandleLayers = e.Devices[0].HandleLayers
		emi.RAM.HandleLastLayer = e.Devices[0].HandleLastLayer
		emi.RAM.HandleOutputLayer = e.Devices[0].HandleOutputLayer

		// UMA.
		emi.RAM.UMA = fp + wg + kv + cp
		if !e.NoMMap && (mmap || e.FullOffloaded) {
			emi.RAM.UMA -= wg
			if !mmap {
				emi.RAM.UMA += e.Devices[0].Weight.Output
			}
		}

		// NonUMA.
		emi.RAM.NonUMA = GGUFBytesScalar(nonUMARamFootprint) + emi.RAM.UMA
	}

	// VRAMs.
	emi.VRAMs = make([]LLaMACppRunEstimateMemory, len(e.Devices)-1)
	{
		for i, d := range e.Devices[1:] {
			fp := d.Footprint
			wg := d.Weight.Sum()
			kv := d.KVCache.Sum()
			cp := d.Computation.Sum()

			emi.VRAMs[i].HandleLayers = d.HandleLayers
			emi.VRAMs[i].HandleLastLayer = d.HandleLastLayer
			emi.VRAMs[i].HandleOutputLayer = d.HandleOutputLayer
			emi.VRAMs[i].Remote = d.Remote
			emi.VRAMs[i].Position = d.Position

			// UMA.
			emi.VRAMs[i].UMA = fp + wg + kv + /* cp */ 0
			if !e.NoMMap && mmap {
				emi.VRAMs[i].UMA -= wg
				if d.Remote || d.Position > 0 && d.HandleLastLayer >= 0 || e.Type == "projector" {
					emi.VRAMs[i].UMA += wg
				}
			}

			// NonUMA.
			emi.VRAMs[i].NonUMA = GGUFBytesScalar(nonUMAVramFootprint) + fp + wg + kv + cp
			if !d.Remote && d.Position > 0 && d.HandleLastLayer < 0 {
				emi.VRAMs[i].NonUMA -= wg + cp
			}
		}
	}

	// Add drafter's usage.
	if e.Drafter != nil {
		demi := e.Drafter.SummarizeItem(mmap, 0, 0)
		emi.RAM.UMA += demi.RAM.UMA
		emi.RAM.NonUMA += demi.RAM.NonUMA
		for i, v := range demi.VRAMs {
			emi.VRAMs[i].UMA += v.UMA
			emi.VRAMs[i].NonUMA += v.NonUMA
		}
	}

	// Add projector's usage.
	if e.Projector != nil {
		pemi := e.Projector.SummarizeItem(mmap, 0, 0)
		emi.RAM.UMA += pemi.RAM.UMA
		emi.RAM.NonUMA += pemi.RAM.NonUMA
		for i, v := range pemi.VRAMs {
			emi.VRAMs[i].UMA += v.UMA
			emi.VRAMs[i].NonUMA += v.NonUMA
		}
	}

	// Add adapters' usage.
	for i := range e.Adapters {
		aemi := e.Adapters[i].SummarizeItem(false, 0, 0)
		emi.RAM.UMA += aemi.RAM.UMA
		emi.RAM.NonUMA += aemi.RAM.NonUMA
		for j, v := range aemi.VRAMs {
			emi.VRAMs[j].UMA += v.UMA
			emi.VRAMs[j].NonUMA += v.NonUMA
		}
	}

	return emi
}

// Summarize returns the corresponding LLaMACppRunEstimateSummary with the given options.
func (e LLaMACppRunEstimate) Summarize(mmap bool, nonUMARamFootprint, nonUMAVramFootprint uint64) (es LLaMACppRunEstimateSummary) {
	// Items.
	es.Items = []LLaMACppRunEstimateSummaryItem{
		e.SummarizeItem(mmap, nonUMARamFootprint, nonUMAVramFootprint),
	}

	// Just copy from the original estimate.
	es.Type = e.Type
	es.Architecture = e.Architecture
	es.ClipProjectorType = e.ClipProjectorType
	es.AdapterType = e.AdapterType
	es.ContextSize = e.ContextSize
	es.FlashAttention = e.FlashAttention
	es.NoMMap = e.NoMMap
	es.EmbeddingOnly = e.EmbeddingOnly
	es.Reranking = e.Reranking
	es.LogicalBatchSize = e.LogicalBatchSize
	es.PhysicalBatchSize = e.PhysicalBatchSize
	es.Distributable = e.Distributable

	return es
}

func (u LLaMACppWeightMemoryUsage) Sum() GGUFBytesScalar {
	return u.Input + u.Compute + u.Output
}

func (u LLaMACppKVCacheMemoryUsage) Sum() GGUFBytesScalar {
	return u.Key + u.Value
}

func (u LLaMACppComputationMemoryUsage) Sum() GGUFBytesScalar {
	return u.Footprint + u.Input + max(u.Compute, u.Output)
}
