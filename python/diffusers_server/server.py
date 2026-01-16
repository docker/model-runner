"""
Diffusers Server for Docker Model Runner

A FastAPI-based server that provides OpenAI Images API compatible endpoints
for Stable Diffusion and other diffusion models using the Hugging Face diffusers library.
"""

import argparse
import base64
import io
import logging
import os
import time
from typing import Optional, List, Literal

import torch
from diffusers import DiffusionPipeline, StableDiffusionPipeline, AutoPipelineForText2Image
from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field
import uvicorn

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="Diffusers Server", description="OpenAI Images API compatible server for diffusion models")

# Global pipeline instance
pipeline: Optional[DiffusionPipeline] = None
current_model: Optional[str] = None
served_model_name: Optional[str] = None


class ImageGenerationRequest(BaseModel):
    """Request model for image generation (OpenAI Images API compatible)"""
    model: str = Field(..., description="The model to use for image generation")
    prompt: str = Field(..., description="A text description of the desired image(s)")
    n: int = Field(default=1, ge=1, le=10, description="The number of images to generate")
    size: str = Field(default="512x512", description="The size of the generated images")
    response_format: Literal["url", "b64_json"] = Field(default="b64_json", description="The format of the generated images")
    quality: Optional[str] = Field(default="standard", description="The quality of the image")
    style: Optional[str] = Field(default=None, description="The style of the generated images")
    negative_prompt: Optional[str] = Field(default=None, description="Text to avoid in generation")
    num_inference_steps: int = Field(default=50, ge=1, le=150, description="Number of denoising steps")
    guidance_scale: float = Field(default=7.5, ge=1.0, le=20.0, description="Guidance scale for generation")
    seed: Optional[int] = Field(default=None, description="Random seed for reproducibility")


class ImageData(BaseModel):
    """Single image in the response"""
    b64_json: Optional[str] = None
    url: Optional[str] = None
    revised_prompt: Optional[str] = None


class ImageGenerationResponse(BaseModel):
    """Response model for image generation (OpenAI Images API compatible)"""
    created: int
    data: List[ImageData]


def parse_size(size: str) -> tuple[int, int]:
    """Parse size string like '512x512' into (width, height) tuple"""
    try:
        parts = size.lower().split('x')
        if len(parts) != 2:
            raise ValueError(f"Invalid size format: {size}")
        width = int(parts[0])
        height = int(parts[1])
        return width, height
    except (ValueError, IndexError) as e:
        raise ValueError(f"Invalid size format '{size}'. Expected format like '512x512': {e}")


def is_dduf_file(path: str) -> bool:
    """Check if the given path is a DDUF file"""
    return path.lower().endswith('.dduf') and os.path.isfile(path)


def load_model_from_dduf(dduf_path: str, device: str, dtype: torch.dtype) -> DiffusionPipeline:
    """Load a diffusion model from a DDUF (Diffusers Unified Format) file"""
    logger.info(f"Loading model from DDUF file: {dduf_path}")
    
    try:
        # Get the directory and filename from the DDUF path
        # DiffusionPipeline.from_pretrained() expects:
        #   - First arg: directory containing the DDUF file (or repo ID for HF Hub)
        #   - dduf_file: the filename (string) of the DDUF file within that directory
        dduf_dir = os.path.dirname(dduf_path)
        dduf_filename = os.path.basename(dduf_path)
        
        logger.info(f"Using directory: {dduf_dir}")
        logger.info(f"Using DDUF filename: {dduf_filename}")
        
        # Load the pipeline from the DDUF file
        # The diffusers library will internally read the DDUF file and extract components
        pipe = DiffusionPipeline.from_pretrained(
            dduf_dir,
            dduf_file=dduf_filename,
            torch_dtype=dtype,
        )
        
        pipe = pipe.to(device)
        logger.info(f"Model loaded successfully from DDUF on {device}")
        return pipe
        
    except Exception as e:
        logger.exception("Error loading DDUF file")
        raise RuntimeError(f"Failed to load DDUF file: {e}")


def load_model(model_path: str) -> DiffusionPipeline:
    """Load a diffusion model from the given path, DDUF file, or HuggingFace model ID"""
    global pipeline, current_model
    
    if pipeline is not None and current_model == model_path:
        logger.info(f"Model {model_path} already loaded")
        return pipeline
    
    logger.info(f"Loading model: {model_path}")
    
    # Determine device
    if torch.cuda.is_available():
        device = "cuda"
        dtype = torch.float16
        logger.info("Using CUDA device with float16")
    elif torch.backends.mps.is_available():
        device = "mps"
        dtype = torch.float16
        logger.info("Using MPS device (Apple Silicon) with float16")
    else:
        device = "cpu"
        dtype = torch.float32
        logger.info("Using CPU device with float32")
    
    # Check if this is a DDUF file
    if is_dduf_file(model_path):
        pipeline = load_model_from_dduf(model_path, device, dtype)
        current_model = model_path
        return pipeline
    
    # Check if this is a directory containing a model
    if os.path.isdir(model_path):
        logger.info(f"Loading model from directory: {model_path}")
    
    try:
        # Try to load using AutoPipelineForText2Image which handles most model types
        pipeline = AutoPipelineForText2Image.from_pretrained(
            model_path,
            torch_dtype=dtype,
            safety_checker=None,  # Disable safety checker for performance
            requires_safety_checker=False,
        )
    except Exception as e:
        logger.warning(f"AutoPipelineForText2Image failed: {e}, trying StableDiffusionPipeline")
        try:
            pipeline = StableDiffusionPipeline.from_pretrained(
                model_path,
                torch_dtype=dtype,
                safety_checker=None,
                requires_safety_checker=False,
            )
        except Exception as e2:
            logger.warning(f"StableDiffusionPipeline failed: {e2}, trying generic DiffusionPipeline")
            pipeline = DiffusionPipeline.from_pretrained(
                model_path,
                torch_dtype=dtype,
            )
    
    pipeline = pipeline.to(device)
    
    # Enable memory efficient attention if available
    if hasattr(pipeline, 'enable_attention_slicing'):
        pipeline.enable_attention_slicing()
    
    current_model = model_path
    logger.info(f"Model loaded successfully on {device}")
    return pipeline


def generate_images(
    prompt: str,
    n: int = 1,
    width: int = 512,
    height: int = 512,
    negative_prompt: Optional[str] = None,
    num_inference_steps: int = 50,
    guidance_scale: float = 7.5,
    seed: Optional[int] = None,
) -> List[bytes]:
    """Generate images using the loaded pipeline"""
    global pipeline
    
    if pipeline is None:
        raise RuntimeError("No model loaded")
    
    # Set seed for reproducibility
    generator = None
    if seed is not None:
        if torch.cuda.is_available():
            generator = torch.Generator(device="cuda").manual_seed(seed)
        elif torch.backends.mps.is_available():
            generator = torch.Generator(device="mps").manual_seed(seed)
        else:
            generator = torch.Generator().manual_seed(seed)
    
    logger.info(f"Generating {n} image(s) with prompt: {prompt[:100]}...")
    
    # Generate images
    images = []
    for i in range(n):
        # If we have a seed, increment it for each image to get different but reproducible results
        current_generator = None
        if generator is not None and seed is not None:
            if torch.cuda.is_available():
                current_generator = torch.Generator(device="cuda").manual_seed(seed + i)
            elif torch.backends.mps.is_available():
                current_generator = torch.Generator(device="mps").manual_seed(seed + i)
            else:
                current_generator = torch.Generator().manual_seed(seed + i)
        
        result = pipeline(
            prompt=prompt,
            negative_prompt=negative_prompt,
            width=width,
            height=height,
            num_inference_steps=num_inference_steps,
            guidance_scale=guidance_scale,
            generator=current_generator,
        )
        
        image = result.images[0]
        
        # Convert to PNG bytes
        buffer = io.BytesIO()
        image.save(buffer, format="PNG")
        images.append(buffer.getvalue())
    
    logger.info(f"Generated {len(images)} image(s)")
    return images


@app.get("/health")
async def health():
    """Health check endpoint"""
    return {"status": "healthy", "model_loaded": current_model is not None}


@app.get("/v1/models")
async def list_models():
    """List available models (OpenAI API compatible)"""
    models = []
    if served_model_name:
        models.append({
            "id": served_model_name,
            "object": "model",
            "created": int(time.time()),
            "owned_by": "diffusers",
        })
    if current_model and current_model != served_model_name:
        models.append({
            "id": current_model,
            "object": "model",
            "created": int(time.time()),
            "owned_by": "diffusers",
        })
    return {"object": "list", "data": models}


@app.post("/v1/images/generations", response_model=ImageGenerationResponse)
async def create_image(request: ImageGenerationRequest):
    """Generate images from a prompt (OpenAI Images API compatible)"""
    global pipeline
    
    # Check if the requested model matches
    requested_model = request.model
    if served_model_name and requested_model != served_model_name and requested_model != current_model:
        raise HTTPException(
            status_code=421,
            detail=f"Model '{requested_model}' not loaded. Current model: {served_model_name or current_model}"
        )
    
    if pipeline is None:
        raise HTTPException(status_code=503, detail="No model loaded. Server is not ready.")
    
    try:
        # Parse size
        width, height = parse_size(request.size)
        
        # Generate images
        image_bytes_list = generate_images(
            prompt=request.prompt,
            n=request.n,
            width=width,
            height=height,
            negative_prompt=request.negative_prompt,
            num_inference_steps=request.num_inference_steps,
            guidance_scale=request.guidance_scale,
            seed=request.seed,
        )
        
        # Format response
        data = []
        for img_bytes in image_bytes_list:
            if request.response_format == "b64_json":
                b64_str = base64.b64encode(img_bytes).decode("utf-8")
                data.append(ImageData(b64_json=b64_str))
            else:
                # URL format not supported in this implementation
                raise HTTPException(
                    status_code=400,
                    detail="URL response format is not supported. Use 'b64_json' instead."
                )
        
        return ImageGenerationResponse(
            created=int(time.time()),
            data=data
        )
    
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        logger.exception("Error generating image")
        raise HTTPException(status_code=500, detail=f"Image generation failed: {str(e)}")


@app.on_event("startup")
async def startup_event():
    """Startup event handler"""
    logger.info("Diffusers server starting up...")
    if current_model:
        logger.info(f"Model path: {current_model}")


def main():
    """Main entry point for the diffusers server"""
    parser = argparse.ArgumentParser(description="Diffusers Server - OpenAI Images API compatible server")
    parser.add_argument("--model-path", type=str, required=True, help="Path to the diffusion model, DDUF file, or HuggingFace model ID")
    parser.add_argument("--host", type=str, default="0.0.0.0", help="Host to bind to")
    parser.add_argument("--port", type=int, default=8000, help="Port to bind to")
    parser.add_argument("--served-model-name", type=str, default=None, help="Name to serve the model as")
    
    args = parser.parse_args()
    
    global served_model_name
    served_model_name = args.served_model_name or args.model_path
    
    try:
        # Load the model at startup
        load_model(args.model_path)
        
        # Start the server
        logger.info(f"Starting server on {args.host}:{args.port}")
        uvicorn.run(app, host=args.host, port=args.port, log_level="info")
    except Exception as e:
        # Extract the root cause error message for cleaner output
        error_msg = str(e)
        # If this is a chained exception, try to get the original cause
        root_cause = e
        while root_cause.__cause__ is not None:
            root_cause = root_cause.__cause__
        if root_cause is not e:
            error_msg = str(root_cause)
        
        # Print a clean, single-line error message that can be easily parsed
        # This format is recognized by the Go backend for better error reporting
        import sys
        print(f"DIFFUSERS_ERROR: {error_msg}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
