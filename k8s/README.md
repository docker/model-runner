# vLLM Kubernetes Deployment Resources

This directory contains deployment guides, manifests, and tools for deploying vLLM inference servers on Kubernetes.

## Structure

- `guides/` - Deployment guides and examples for different configurations
- `docker/` - Dockerfiles for building vLLM container images
- `scripts/` - Helper scripts for deployment and management

## Usage

Use the Docker Model CLI to deploy vLLM on Kubernetes:

```bash
# List available deployment configurations
docker model k8s list-configs

# View deployment guides
docker model k8s guide

# Deploy a configuration
docker model k8s deploy --config inference-scheduling --namespace vllm-inference
```

## Available Deployment Configurations

1. **inference-scheduling** - Intelligent inference scheduling with load balancing
2. **pd-disaggregation** - Prefill/Decode disaggregation for better performance  
3. **wide-ep** - Wide Expert-Parallelism for large MoE models
4. **simulated-accelerators** - Deploy with simulated accelerators for testing

## Prerequisites

- Kubernetes cluster (version 1.29 or later)
- kubectl configured to access your cluster
- helm (version 3.x or later)
- GPU support (NVIDIA, AMD, Google TPU, or Intel XPU)

## Documentation

For detailed deployment guides, see the `guides/` directory or run:

```bash
docker model k8s guide
```

## Hardware Support

vLLM on Kubernetes supports:
- NVIDIA GPUs (A100, H100, L4, etc.)
- AMD GPUs (MI250, MI300, etc.)
- Google TPUs (v5e and newer)
- Intel XPUs (Ponte Vecchio and newer)

## Contributing

This deployment configuration is based on production-tested patterns for running vLLM at scale.
