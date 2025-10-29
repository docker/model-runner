.PHONY: render clean install upgrade uninstall template lint package help

CHART_NAME := docker-model-runner
RELEASE_NAME := docker-model-runner
NAMESPACE := default

render:
	@echo "Rendering Helm chart to plain Kubernetes YAML..."
	mkdir -p static
	helm template $(RELEASE_NAME) . --namespace $(NAMESPACE) > static/docker-model-runner.yaml
	helm template $(RELEASE_NAME) . --namespace $(NAMESPACE) --set "nodePort.enabled=true" > static/docker-model-runner-desktop.yaml
	helm template $(RELEASE_NAME) . --namespace $(NAMESPACE) --set "modelInit.enabled=true" --set "modelInit.models[0]=ai/smollm2:latest" > static/docker-model-runner-smollm2.yaml
	helm template $(RELEASE_NAME) . --namespace $(NAMESPACE) --set "storage.storageClass=gp2" > static/docker-model-runner-eks.yaml
	@echo "Rendered YAML saved to static"

clean:
	@echo "Cleaning up rendered files..."
	rm -fR static

install:
	@echo "Installing Helm chart..."
	helm install $(RELEASE_NAME) . --namespace $(NAMESPACE) --create-namespace

upgrade:
	@echo "Upgrading Helm chart..."
	helm upgrade $(RELEASE_NAME) . --namespace $(NAMESPACE)

uninstall:
	@echo "Uninstalling Helm chart..."
	helm uninstall $(RELEASE_NAME) --namespace $(NAMESPACE)

template:
	@echo "Templating Helm chart..."
	helm template $(RELEASE_NAME) . --namespace $(NAMESPACE)

lint:
	@echo "Linting Helm chart..."
	helm lint .

package:
	@echo "Packaging Helm chart..."
	helm package .

help:
	@echo "Available targets:"
	@echo "  render    - Render Helm chart to plain Kubernetes YAML (saves to rendered.yaml)"
	@echo "  template  - Template Helm chart (output to stdout)"
	@echo "  lint      - Lint Helm chart"
	@echo "  package   - Package Helm chart"
	@echo "  install   - Install Helm chart"
	@echo "  upgrade   - Upgrade Helm chart"
	@echo "  uninstall - Uninstall Helm chart"
	@echo "  clean     - Clean up rendered files"
	@echo "  help      - Show this help message"
