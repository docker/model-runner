package standalone

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// modelStorageVolumeName is the name to use for the model storage volume.
const modelStorageVolumeName = "docker-model-runner-models"

// EnsureModelStorageVolume ensures that a model storage volume exists, creating
// it if necessary. It returns the name of the storage volume or any error that
// occurred.
func EnsureModelStorageVolume(ctx context.Context, dockerClient client.VolumeAPIClient, printer StatusPrinter) (string, error) {
	// Try to identify the storage volume.
	volumes, err := dockerClient.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", labelRole+"="+roleModelStorage),
		),
	})
	if err != nil {
		return "", fmt.Errorf("unable to list volumes: %w", err)
	}

	// If any volumes with the correct role exist (ideally there should only be
	// one), then pick the first one.
	if len(volumes.Volumes) > 0 {
		return volumes.Volumes[0].Name, nil
	}

	// Create the volume.
	printer.Printf("Creating model storage volume %s...\n", modelStorageVolumeName)
	volume, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{
		Name: modelStorageVolumeName,
		Labels: map[string]string{
			labelDesktopService: serviceModelRunner,
			labelRole:           roleModelStorage,
		},
	})
	if err != nil {
		return "", fmt.Errorf("unable to create volume: %w", err)
	}
	return volume.Name, nil
}

// PruneModelStorageVolumes removes any unused model storage volume(s).
func PruneModelStorageVolumes(ctx context.Context, dockerClient client.VolumeAPIClient, printer StatusPrinter) error {
	pruned, err := dockerClient.VolumesPrune(ctx, filters.NewArgs(
		filters.Arg("all", "true"),
		filters.Arg("label", labelRole+"="+roleModelStorage),
	))
	if err != nil {
		return err
	}
	for _, volume := range pruned.VolumesDeleted {
		printer.Println("Removed volume", volume)
	}
	if pruned.SpaceReclaimed > 0 {
		printer.Printf("Reclaimed %d bytes\n", pruned.SpaceReclaimed)
	}
	return nil
}
