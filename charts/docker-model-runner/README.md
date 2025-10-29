# Docker Model Runner Kubernetes Support

Manifests for deploying Docker Model Runner on Kubernetes with ephemeral storage, GPU support, and model pre-pulling capabilities.

## Quickstart

### On Docker Desktop

```
kubectl apply -f static/docker-model-runner-desktop.yaml
kubectl wait --for=condition=Available deployment/docker-model-runner --timeout=5m
MODEL_RUNNER_HOST=http://localhost:31245 docker model run ai/smollm2:latest
```

### On any Kubernetes Cluster

```
kubectl apply -f static/docker-model-runner.yaml
kubectl wait --for=condition=Available deployment/docker-model-runner --timeout=5m
kubectl port-forward deployment/docker-model-runner 31245:12434
```

Then:

```
MODEL_RUNNER_HOST=http://localhost:31245 docker model run ai/smollm2:latest
```

## Helm Configuration

### Basic Configuration

Key configuration options in `values.yaml`:

```yaml
# Storage configuration
storage:
  size: 100Gi
  storageClass: ""  # Set this to the storage class of your cloud provider.

# Model pre-pull configuration
modelInit:
  enabled: false
  models:
    - "ai/smollm2:latest"

# Image configuration
image:
  repository: docker/model-runner
  tag: "latest"  # Use 'latest-cuda' for NVIDIA or 'latest-rocm' for AMD GPUs
  pullPolicy: IfNotPresent

# GPU configuration
gpu:
  enabled: false
  vendor: nvidia  # or amd
  count: 1
  # For AMD GPUs, use 'latest-rocm' image tag

# NodePort configuration
nodePort:
  enabled: false
  port: 31245
```

### GPU Scheduling

To enable GPU scheduling:

```yaml
gpu:
  enabled: true
  vendor: nvidia  # or amd
  count: 1
```

This will add the appropriate resource requests/limits:
- NVIDIA: `nvidia.com/gpu`
- AMD: `amd.com/gpu`

### Model Pre-pulling

Configure models to pre-pull during pod initialization:

```yaml
modelInit:
  enabled: true
  models:
    - "ai/smollm2:latest"
    - "ai/llama3.2:latest"
    - "ai/mistral:latest"
```

## Usage

### Testing the Installation

Once installed, set up a port-forward to access the service:

```bash
kubectl port-forward service/docker-model-runner-nodeport 31245:80
```

Then test the model runner:

```bash
MODEL_RUNNER_HOST=http://localhost:31245 docker model run ai/smollm2:latest
```

### Using with Open WebUI

To use Docker Model Runner with Open WebUI, install the Open WebUI Helm chart:

```bash
# Add the Open WebUI Helm repository
helm repo add open-webui https://helm.openwebui.com/
helm repo update

# Install Open WebUI with auth diabled
# See the open-webui Helm chart for
# connecting to your auth provider.
helm upgrade --install --wait open-webui open-webui/open-webui \
  --set ollama.enabled=false \
  --set pipelines.enabled=false \
  --set extraEnvVars[0].name="WEBUI_AUTH" \
  --set-string extraEnvVars[0].value=false \
  --set openaiBaseApiUrl="http://docker-model-runner/engines/v1"
```

Access Open WebUI:

```bash
kubectl port-forward service/open-webui 8080:80
```

Then visit http://localhost:8080 in your browser.

## Values Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Docker Model Runner image repository | `docker/model-runner` |
| `image.tag` | Docker Model Runner image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `storage.size` | Ephemeral volume size | `100Gi` |
| `storage.storageClass` | Storage class for ephemeral volume | `""` |
| `modelInit.enabled` | Enable model pre-pulling | `false` |
| `modelInit.models` | List of models to pre-pull | `["ai/smollm2:latest"]` |
| `gpu.enabled` | Enable GPU support | `false` |
| `gpu.vendor` | GPU vendor (nvidia or amd) | `nvidia` |
| `gpu.count` | Number of GPUs to request | `1` |
| `nodePort.enabled` | Enable NodePort service | `false` |
| `nodePort.port` | NodePort port number | `31245` |

## Troubleshooting

### Pod Fails to Start

Check the pod logs:

```bash
kubectl logs -f deployment/docker-model-runner
```

### Model Pre-pull Issues

Check the init container logs:

```bash
kubectl logs -f deployment/docker-model-runner -c model-init
```

### GPU Not Available

Your cluster must use [a GPU scheduling plugin](https://kubernetes.io/docs/tasks/manage-gpus/scheduling-gpus/).

Ensure your cluster has GPU support and the appropriate device plugin installed:

- For NVIDIA GPUs: Install the [NVIDIA device plugin](https://github.com/NVIDIA/k8s-device-plugin)
- For AMD GPUs: Install the [AMD device plugin](https://github.com/ROCm/k8s-device-plugin#deployment)

