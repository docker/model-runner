package standalone

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	gpupkg "github.com/docker/model-runner/cmd/cli/pkg/gpu"
)

// EnsureControllerImage ensures that the controller container image is pulled.
func EnsureControllerImage(ctx context.Context, dockerClient client.ImageAPIClient, gpu gpupkg.GPUSupport, printer StatusPrinter) error {
	imageName := controllerImageName(gpu)
	return ensureImage(ctx, dockerClient, imageName, printer)
}

// EnsureOllamaImage ensures that the ollama container image is pulled.
func EnsureOllamaImage(ctx context.Context, dockerClient client.ImageAPIClient, gpu gpupkg.GPUSupport, gpuVariant string, printer StatusPrinter) error {
	imageName := ollamaImageName(gpu, gpuVariant)
	return ensureImage(ctx, dockerClient, imageName, printer)
}

// ensureImage pulls a container image if needed.
func ensureImage(ctx context.Context, dockerClient client.ImageAPIClient, imageName string, printer StatusPrinter) error {
	// Perform the pull.
	out, err := dockerClient.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer out.Close()

	// Decode and print status updates.
	decoder := json.NewDecoder(out)
	for {
		var response jsonmessage.JSONMessage
		if err := decoder.Decode(&response); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to decode pull response: %w", err)
		}

		if response.ID != "" {
			printer.Printf("\r%s: %s %s", response.ID, response.Status, response.ProgressMessage)
		} else {
			printer.Println(response.Status)
		}
	}
	printer.Println("\nSuccessfully pulled", imageName)
	return nil
}

// PruneControllerImages removes any unused controller container images.
func PruneControllerImages(ctx context.Context, dockerClient client.ImageAPIClient, printer StatusPrinter) error {
	// Remove the standard image, if present.
	imageNameCPU := fmtControllerImageName(ControllerImage, controllerImageVersion(), "")
	if _, err := dockerClient.ImageRemove(ctx, imageNameCPU, image.RemoveOptions{}); err == nil {
		printer.Println("Removed image", imageNameCPU)
	}

	// Remove the CUDA GPU image, if present.
	imageNameCUDA := fmtControllerImageName(ControllerImage, controllerImageVersion(), "cuda")
	if _, err := dockerClient.ImageRemove(ctx, imageNameCUDA, image.RemoveOptions{}); err == nil {
		printer.Println("Removed image", imageNameCUDA)
	}
	return nil
}

// PruneOllamaImages removes any unused ollama container images.
func PruneOllamaImages(ctx context.Context, dockerClient client.ImageAPIClient, printer StatusPrinter) error {
	// Remove the standard ollama image, if present.
	imageNameBase := fmtControllerImageName(OllamaImage, ollamaImageVersion(), "")
	if _, err := dockerClient.ImageRemove(ctx, imageNameBase, image.RemoveOptions{}); err == nil {
		printer.Println("Removed image", imageNameBase)
	}

	// Remove the ROCm ollama image, if present.
	imageNameROCm := fmtControllerImageName(OllamaImage, ollamaImageVersion(), "rocm")
	if _, err := dockerClient.ImageRemove(ctx, imageNameROCm, image.RemoveOptions{}); err == nil {
		printer.Println("Removed image", imageNameROCm)
	}
	return nil
}
