package standalone

import (
	"context"
	"fmt"

	"github.com/moby/moby/client"
)

// modelStorageVolumeName is the name to use for the model storage volume.
const modelStorageVolumeName = "docker-model-runner-models"

// EnsureModelStorageVolume ensures that a model storage volume exists, creating
// it if necessary. It returns the name of the storage volume or any error that
// occurred.
func EnsureModelStorageVolume(ctx context.Context, dockerClient client.VolumeAPIClient, printer StatusPrinter) (string, error) {
	// Try to identify the storage volume.
	res, err := dockerClient.VolumeList(ctx, client.VolumeListOptions{
		Filters: make(client.Filters).Add("label", labelRole+"="+roleModelStorage),
	})
	if err != nil {
		return "", fmt.Errorf("unable to list volumes: %w", err)
	}

	// If any volumes with the correct role exist (ideally there should only be
	// one), then pick the first one.
	if len(res.Items) > 0 {
		return res.Items[0].Name, nil
	}

	// Create the volume.
	printer.Printf("Creating model storage volume %s...\n", modelStorageVolumeName)
	resp, err := dockerClient.VolumeCreate(ctx, client.VolumeCreateOptions{
		Name: modelStorageVolumeName,
		Labels: map[string]string{
			labelDesktopService: serviceModelRunner,
			labelRole:           roleModelStorage,
		},
	})
	if err != nil {
		return "", fmt.Errorf("unable to create volume: %w", err)
	}
	return resp.Volume.Name, nil
}

// PruneModelStorageVolumes removes any unused model storage volume(s).
func PruneModelStorageVolumes(ctx context.Context, dockerClient client.VolumeAPIClient, printer StatusPrinter) error {
	pruned, err := dockerClient.VolumePrune(ctx, client.VolumePruneOptions{
		Filters: make(client.Filters).Add("all", "true").Add("label", labelRole+"="+roleModelStorage),
	})
	if err != nil {
		return err
	}
	for _, volume := range pruned.Report.VolumesDeleted {
		printer.Println("Removed volume", volume)
	}
	if pruned.Report.SpaceReclaimed > 0 {
		printer.Printf("Reclaimed %d bytes\n", pruned.Report.SpaceReclaimed)
	}
	return nil
}
