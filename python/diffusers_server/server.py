"""FastAPI server implementing OpenAI-compatible image generation API."""

import base64
import io
import logging
import os
import socket
import time
from typing import List, Optional

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
import uvicorn

from .pipeline import DiffusersPipeline

logger = logging.getLogger(__name__)

app = FastAPI(title="Diffusers Server", version="0.1.0")

# Global pipeline instance
_pipeline: Optional[DiffusersPipeline] = None
_served_model_names: List[str] = []


class ImageGenerationRequest(BaseModel):
    """OpenAI-compatible image generation request."""

    model: str
    prompt: str
    n: int = Field(default=1, ge=1, le=10)
    size: str = Field(default="1024x1024")
    quality: str = Field(default="standard")
    response_format: str = Field(default="b64_json")
    style: str = Field(default="vivid")


class ImageData(BaseModel):
    """Single generated image data."""

    url: Optional[str] = None
    b64_json: Optional[str] = None
    revised_prompt: Optional[str] = None


class ImageGenerationResponse(BaseModel):
    """OpenAI-compatible image generation response."""

    created: int
    data: List[ImageData]


class ErrorDetail(BaseModel):
    """Error detail for OpenAI-compatible error response."""

    message: str
    type: str
    param: Optional[str] = None
    code: Optional[str] = None


class ErrorResponse(BaseModel):
    """OpenAI-compatible error response."""

    error: ErrorDetail


def parse_size(size: str) -> tuple[int, int]:
    """Parse size string like '1024x1024' into (width, height)."""
    try:
        parts = size.lower().split("x")
        if len(parts) != 2:
            raise ValueError(f"Invalid size format: {size}")
        width, height = int(parts[0]), int(parts[1])
        return width, height
    except (ValueError, IndexError) as e:
        raise ValueError(f"Invalid size format: {size}") from e


@app.post("/v1/images/generations", response_model=ImageGenerationResponse)
async def generate_images(request: ImageGenerationRequest) -> ImageGenerationResponse:
    """Generate images from a text prompt."""
    global _pipeline, _served_model_names

    if _pipeline is None:
        raise HTTPException(status_code=503, detail="Model not loaded")

    # Validate model name if served_model_names is configured
    if _served_model_names and request.model not in _served_model_names:
        raise HTTPException(
            status_code=404,
            detail=f"Model '{request.model}' not found. Available: {_served_model_names}",
        )

    try:
        width, height = parse_size(request.size)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    # Map quality to inference steps
    num_inference_steps = 50 if request.quality == "hd" else 30

    try:
        images = _pipeline.generate(
            prompt=request.prompt,
            num_images=request.n,
            width=width,
            height=height,
            num_inference_steps=num_inference_steps,
        )
    except Exception as e:
        logger.exception("Error generating images")
        raise HTTPException(status_code=500, detail=str(e))

    # Convert images to response format
    data = []
    for img in images:
        if request.response_format == "b64_json":
            # Convert PIL Image to base64
            buffer = io.BytesIO()
            img.save(buffer, format="PNG")
            b64_data = base64.b64encode(buffer.getvalue()).decode("utf-8")
            data.append(ImageData(b64_json=b64_data))
        else:
            # URL format not supported without file storage
            raise HTTPException(
                status_code=400,
                detail="response_format 'url' not supported, use 'b64_json'",
            )

    return ImageGenerationResponse(
        created=int(time.time()),
        data=data,
    )


@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {"status": "ok", "model_loaded": _pipeline is not None}


@app.get("/v1/models")
async def list_models():
    """List available models (OpenAI-compatible)."""
    models = []
    for name in _served_model_names:
        models.append(
            {
                "id": name,
                "object": "model",
                "created": int(time.time()),
                "owned_by": "diffusers",
            }
        )
    return {"object": "list", "data": models}


def run_server(
    model_path: str,
    socket_path: str,
    device: str = "auto",
    precision: str = "auto",
    enable_attention_slicing: bool = False,
    served_model_names: Optional[List[str]] = None,
):
    """Run the diffusers server on a Unix domain socket."""
    global _pipeline, _served_model_names

    # Configure logging
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
    )

    logger.info(f"Loading model from {model_path}")
    logger.info(f"Device: {device}, Precision: {precision}")

    # Load the pipeline
    _pipeline = DiffusersPipeline(
        model_path=model_path,
        device=device,
        precision=precision,
        enable_attention_slicing=enable_attention_slicing,
    )

    _served_model_names = served_model_names or []
    logger.info(f"Serving model names: {_served_model_names}")

    # Remove existing socket if present
    if os.path.exists(socket_path):
        os.unlink(socket_path)

    logger.info(f"Starting server on unix://{socket_path}")

    # Create and bind the socket manually for Unix domain sockets
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.bind(socket_path)
    os.chmod(socket_path, 0o666)

    config = uvicorn.Config(
        app,
        log_level="info",
    )
    server = uvicorn.Server(config)

    # Override the socket
    server.config.fd = sock.fileno()

    try:
        server.run(sockets=[sock])
    finally:
        sock.close()
        if os.path.exists(socket_path):
            os.unlink(socket_path)
