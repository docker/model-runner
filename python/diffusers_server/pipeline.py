"""Diffusers pipeline wrapper for image generation."""

import logging
import os
from typing import List, Optional

import torch
from PIL import Image

logger = logging.getLogger(__name__)


class DiffusersPipeline:
    """Wrapper for diffusers pipelines supporting various image generation models."""

    def __init__(
        self,
        model_path: str,
        device: str = "auto",
        precision: str = "auto",
        enable_attention_slicing: bool = False,
    ):
        """Initialize the diffusers pipeline.

        Args:
            model_path: Path to the model directory
            device: Device to use (auto, cpu, cuda, mps)
            precision: Precision to use (auto, fp16, bf16, fp32)
            enable_attention_slicing: Enable attention slicing for memory efficiency
        """
        self.model_path = model_path
        self.device = self._resolve_device(device)
        self.dtype = self._resolve_dtype(precision)
        self.enable_attention_slicing = enable_attention_slicing

        logger.info(f"Using device: {self.device}, dtype: {self.dtype}")

        self.pipeline = self._load_pipeline()

    def _resolve_device(self, device: str) -> str:
        """Resolve the device to use."""
        if device != "auto":
            return device

        if torch.cuda.is_available():
            return "cuda"
        elif hasattr(torch.backends, "mps") and torch.backends.mps.is_available():
            return "mps"
        else:
            return "cpu"

    def _resolve_dtype(self, precision: str) -> torch.dtype:
        """Resolve the dtype to use."""
        if precision == "fp16":
            return torch.float16
        elif precision == "bf16":
            return torch.bfloat16
        elif precision == "fp32":
            return torch.float32
        elif precision == "auto":
            # Use fp16 on GPU, fp32 on CPU
            if self.device in ("cuda", "mps"):
                return torch.float16
            else:
                return torch.float32
        else:
            return torch.float32

    def _load_pipeline(self):
        """Load the appropriate diffusers pipeline based on model type."""
        from diffusers import (
            DiffusionPipeline,
            StableDiffusionPipeline,
            StableDiffusionXLPipeline,
        )

        # Check for model_index.json to determine pipeline type
        model_index_path = os.path.join(self.model_path, "model_index.json")

        try:
            if os.path.exists(model_index_path):
                # Use auto-detection via DiffusionPipeline
                logger.info("Loading pipeline using DiffusionPipeline.from_pretrained")
                pipeline = DiffusionPipeline.from_pretrained(
                    self.model_path,
                    torch_dtype=self.dtype,
                    local_files_only=True,
                )
            else:
                # Try StableDiffusion as fallback
                logger.info("No model_index.json found, trying StableDiffusionPipeline")
                try:
                    pipeline = StableDiffusionXLPipeline.from_pretrained(
                        self.model_path,
                        torch_dtype=self.dtype,
                        local_files_only=True,
                    )
                except Exception:
                    pipeline = StableDiffusionPipeline.from_pretrained(
                        self.model_path,
                        torch_dtype=self.dtype,
                        local_files_only=True,
                    )
        except Exception as e:
            logger.error(f"Failed to load pipeline: {e}")
            raise RuntimeError(f"Failed to load diffusers model from {self.model_path}: {e}")

        # Move to device
        pipeline = pipeline.to(self.device)

        # Apply optimizations
        if self.enable_attention_slicing:
            logger.info("Enabling attention slicing")
            pipeline.enable_attention_slicing()

        # Enable memory efficient attention if available
        if self.device == "cuda":
            try:
                pipeline.enable_xformers_memory_efficient_attention()
                logger.info("Enabled xformers memory efficient attention")
            except Exception:
                logger.info("xformers not available, using default attention")

        return pipeline

    def generate(
        self,
        prompt: str,
        num_images: int = 1,
        width: int = 1024,
        height: int = 1024,
        num_inference_steps: int = 30,
        guidance_scale: float = 7.5,
        negative_prompt: Optional[str] = None,
        seed: Optional[int] = None,
    ) -> List[Image.Image]:
        """Generate images from a text prompt.

        Args:
            prompt: The text prompt to generate images from
            num_images: Number of images to generate
            width: Image width
            height: Image height
            num_inference_steps: Number of denoising steps
            guidance_scale: Guidance scale for classifier-free guidance
            negative_prompt: Negative prompt for guidance
            seed: Random seed for reproducibility

        Returns:
            List of PIL Images
        """
        generator = None
        if seed is not None:
            generator = torch.Generator(device=self.device).manual_seed(seed)

        logger.info(
            f"Generating {num_images} image(s): {width}x{height}, "
            f"steps={num_inference_steps}, guidance={guidance_scale}"
        )

        # Build kwargs based on pipeline capabilities
        kwargs = {
            "prompt": prompt,
            "num_images_per_prompt": num_images,
            "width": width,
            "height": height,
            "num_inference_steps": num_inference_steps,
            "guidance_scale": guidance_scale,
        }

        if generator is not None:
            kwargs["generator"] = generator

        if negative_prompt is not None:
            kwargs["negative_prompt"] = negative_prompt

        # Run inference
        with torch.inference_mode():
            result = self.pipeline(**kwargs)

        return result.images
