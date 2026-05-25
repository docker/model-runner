package standalone

import (
	"context"
	"fmt"

	gpupkg "github.com/docker/model-runner/cmd/cli/pkg/gpu"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/jsonmessage"
)

// EnsureControllerImage ensures that the controller container image is
// available. It first tries to pull from the registry; if that fails it
// falls back to a locally available image with the same name.
func EnsureControllerImage(ctx context.Context, dockerClient client.ImageAPIClient, gpu gpupkg.GPUSupport, backend string, printer StatusPrinter) error {
	imageName := controllerImageName(gpu, backend)

	var pullErr error
	out, pullErr := dockerClient.ImagePull(ctx, imageName, client.ImagePullOptions{})
	if pullErr == nil {
		defer out.Close()
		fd, isTerminal := printer.GetFdInfo()
		pullErr = jsonmessage.DisplayJSONMessagesStream(out, printer, fd, isTerminal, nil)
	}
	if pullErr == nil {
		printer.Println("Successfully pulled", imageName)
		return nil
	}

	// Pull failed — check if the image exists locally.
	_, inspectErr := dockerClient.ImageInspect(ctx, imageName)
	if inspectErr != nil {
		return fmt.Errorf("failed to pull image %s and no local image found: %w", imageName, pullErr)
	}
	printer.Println("Using local image", imageName)
	return nil
}

// PruneControllerImages removes any unused controller container images.
func PruneControllerImages(ctx context.Context, dockerClient client.ImageAPIClient, printer StatusPrinter) error {
	// Remove the standard image, if present.
	imageNameCPU := fmtControllerImageName(ControllerImage, controllerImageVersion(), "")
	if _, err := dockerClient.ImageRemove(ctx, imageNameCPU, client.ImageRemoveOptions{}); err == nil {
		printer.Println("Removed image", imageNameCPU)
	}

	// Remove the CUDA GPU image, if present.
	imageNameCUDA := fmtControllerImageName(ControllerImage, controllerImageVersion(), "cuda")
	if _, err := dockerClient.ImageRemove(ctx, imageNameCUDA, client.ImageRemoveOptions{}); err == nil {
		printer.Println("Removed image", imageNameCUDA)
	}
	return nil
}
