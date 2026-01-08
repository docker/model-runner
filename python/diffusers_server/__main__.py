"""Entry point for running the diffusers server as a module.

Usage:
    python -m diffusers_server --model-path /path/to/model --socket /path/to/socket
"""

import argparse
import sys

from .server import run_server


def main():
    parser = argparse.ArgumentParser(
        description="Diffusers server for Docker Model Runner"
    )
    parser.add_argument(
        "--model-path",
        required=True,
        help="Path to the diffusers model directory",
    )
    parser.add_argument(
        "--socket",
        required=True,
        help="Unix socket path to listen on",
    )
    parser.add_argument(
        "--device",
        default="auto",
        choices=["auto", "cpu", "cuda", "mps"],
        help="Device to run inference on (default: auto)",
    )
    parser.add_argument(
        "--precision",
        default="auto",
        choices=["auto", "fp16", "bf16", "fp32"],
        help="Precision for inference (default: auto)",
    )
    parser.add_argument(
        "--enable-attention-slicing",
        action="store_true",
        help="Enable attention slicing for memory efficiency",
    )
    parser.add_argument(
        "--served-model-name",
        nargs="*",
        default=[],
        help="Model names to serve (for OpenAI API compatibility)",
    )

    args = parser.parse_args()

    try:
        run_server(
            model_path=args.model_path,
            socket_path=args.socket,
            device=args.device,
            precision=args.precision,
            enable_attention_slicing=args.enable_attention_slicing,
            served_model_names=args.served_model_name,
        )
    except KeyboardInterrupt:
        print("\nShutting down...")
        sys.exit(0)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
