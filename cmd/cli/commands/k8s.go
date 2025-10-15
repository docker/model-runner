package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newK8sCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "k8s",
		Short: "Deploy vLLM on Kubernetes",
		Long: `Deploy vLLM inference servers on Kubernetes with intelligent load balancing.

This command provides deployment guides and tools for running vLLM on Kubernetes
with production-ready configurations including:
- Intelligent inference scheduling with load balancing
- Prefill/Decode disaggregation for better performance
- Wide Expert-Parallelism for large MoE models
- Support for NVIDIA GPUs, AMD GPUs, Google TPUs, and Intel XPUs`,
	}

	c.AddCommand(
		newK8sDeployCmd(),
		newK8sListConfigsCmd(),
		newK8sGuideCmd(),
	)

	return c
}

func newK8sDeployCmd() *cobra.Command {
	var namespace string
	var config string
	var model string
	var replicas int

	c := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy vLLM on Kubernetes",
		Long:  "Deploy vLLM inference server on Kubernetes with the specified configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if config == "" {
				return fmt.Errorf("--config is required. Use 'docker model k8s list-configs' to see available configurations")
			}

			// Get the path to the k8s resources
			resourcesPath, err := getK8sResourcesPath()
			if err != nil {
				return err
			}

			configPath := filepath.Join(resourcesPath, "configs", config)
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				return fmt.Errorf("configuration '%s' not found. Use 'docker model k8s list-configs' to see available configurations", config)
			}

			cmd.Printf("Deploying vLLM with configuration: %s\n", config)
			cmd.Printf("Namespace: %s\n", namespace)
			if model != "" {
				cmd.Printf("Model: %s\n", model)
			}
			cmd.Printf("Replicas: %d\n", replicas)

			// Check if kubectl is available
			if _, err := exec.LookPath("kubectl"); err != nil {
				return fmt.Errorf("kubectl not found in PATH. Please install kubectl to deploy to Kubernetes")
			}

			// Check if helm is available for more complex deployments
			if _, err := exec.LookPath("helm"); err != nil {
				cmd.PrintErrln("Warning: helm not found in PATH. Some deployment options may not be available.")
			}

			cmd.Println("\nDeployment instructions:")
			cmd.Printf("1. Ensure your kubectl context is set to the correct cluster\n")
			cmd.Printf("2. Create namespace if it doesn't exist: kubectl create namespace %s\n", namespace)
			cmd.Printf("3. Apply the configuration: kubectl apply -f %s -n %s\n", configPath, namespace)
			cmd.Printf("\nFor detailed deployment guides, run: docker model k8s guide\n")

			return nil
		},
	}

	c.Flags().StringVarP(&namespace, "namespace", "n", "vllm-inference", "Kubernetes namespace")
	c.Flags().StringVarP(&config, "config", "c", "", "Configuration to deploy (required)")
	c.Flags().StringVarP(&model, "model", "m", "", "Model to deploy")
	c.Flags().IntVarP(&replicas, "replicas", "r", 1, "Number of replicas")
	_ = c.MarkFlagRequired("config")

	return c
}

func newK8sListConfigsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "list-configs",
		Short: "List available Kubernetes deployment configurations",
		Long:  "List all available pre-configured deployment options for vLLM on Kubernetes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Available Kubernetes deployment configurations:")
			cmd.Println()
			cmd.Println("1. inference-scheduling")
			cmd.Println("   Deploy vLLM with intelligent inference scheduling for optimal load balancing")
			cmd.Println()
			cmd.Println("2. pd-disaggregation")
			cmd.Println("   Prefill/Decode disaggregation for improved latency and throughput")
			cmd.Println()
			cmd.Println("3. wide-ep")
			cmd.Println("   Wide Expert-Parallelism for large Mixture-of-Experts models")
			cmd.Println()
			cmd.Println("4. simulated-accelerators")
			cmd.Println("   Deploy with simulated accelerators for testing")
			cmd.Println()
			cmd.Println("Use 'docker model k8s deploy --config <name>' to deploy a configuration")
			cmd.Println("Use 'docker model k8s guide' to view detailed deployment guides")
			return nil
		},
	}
	return c
}

func newK8sGuideCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "guide",
		Short: "Display deployment guides",
		Long:  "Display detailed guides for deploying vLLM on Kubernetes",
		RunE: func(cmd *cobra.Command, args []string) error {
			resourcesPath, err := getK8sResourcesPath()
			if err != nil {
				return err
			}

			guidePath := filepath.Join(resourcesPath, "guides", "README.md")
			content, err := os.ReadFile(guidePath)
			if err != nil {
				// Fallback to inline guide if file doesn't exist
				cmd.Println(getInlineGuide())
				return nil
			}

			cmd.Println(string(content))
			return nil
		},
	}
	return c
}

func getK8sResourcesPath() (string, error) {
	// Try to find the k8s resources directory
	// First check if it's in the current directory structure
	candidates := []string{
		"./k8s",
		"./deploy/k8s",
		filepath.Join(os.Getenv("HOME"), ".docker", "model", "k8s"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// If not found, we'll create a minimal structure
	homePath := filepath.Join(os.Getenv("HOME"), ".docker", "model", "k8s")
	if err := os.MkdirAll(homePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create k8s resources directory: %w", err)
	}

	return homePath, nil
}

func getInlineGuide() string {
	return `# vLLM Kubernetes Deployment Guide

## Overview

This guide helps you deploy vLLM inference servers on Kubernetes with production-ready configurations.

## Prerequisites

1. **Kubernetes Cluster**: A running Kubernetes cluster (version 1.29 or later)
2. **kubectl**: Kubernetes command-line tool configured to access your cluster
3. **helm**: Helm package manager (version 3.x or later)
4. **GPU Support**: Nodes with supported GPUs (NVIDIA, AMD, Google TPU, or Intel XPU)

## Quick Start

### 1. Choose a Deployment Configuration

Available configurations:
- **inference-scheduling**: Intelligent load balancing for optimal throughput
- **pd-disaggregation**: Separate prefill and decode phases for better latency
- **wide-ep**: Expert parallelism for large MoE models

### 2. Deploy

` + "```bash" + `
# Set your namespace
export NAMESPACE=vllm-inference

# Create namespace
kubectl create namespace $NAMESPACE

# Deploy with a configuration
docker model k8s deploy --config inference-scheduling --namespace $NAMESPACE
` + "```" + `

### 3. Verify Deployment

` + "```bash" + `
# Check pods
kubectl get pods -n $NAMESPACE

# Check services
kubectl get services -n $NAMESPACE
` + "```" + `

## Hardware Support

vLLM on Kubernetes supports:
- NVIDIA GPUs (A100, H100, L4, etc.)
- AMD GPUs (MI250, MI300, etc.)
- Google TPUs (v5e and newer)
- Intel XPUs (Ponte Vecchio and newer)

## Next Steps

1. Configure your model serving parameters
2. Set up monitoring and observability
3. Configure autoscaling based on your workload
4. Implement request routing and load balancing

For more detailed information, visit the vLLM documentation at https://docs.vllm.ai
`
}
