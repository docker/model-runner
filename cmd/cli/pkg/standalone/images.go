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
func EnsureControllerImage(ctx context.Context, dockerClient client.ImageAPIClient, gpu gpupkg.GPUSupport, vllm bool, printer StatusPrinter) error {
	imageName := controllerImageName(gpu, vllm)

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
