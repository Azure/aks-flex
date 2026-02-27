package service

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/nebius/gosdk"
	nebiuscommon "github.com/nebius/gosdk/proto/nebius/common/v1"
	nebiuscompute "github.com/nebius/gosdk/proto/nebius/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NebiusInstance holds the combined state of a Nebius instance and its boot disk.
type NebiusInstance struct {
	Instance *nebiuscompute.Instance
	BootDisk *nebiuscompute.Disk
}

// NebiusInstanceService manages Nebius compute instances and their boot disks.
type NebiusInstanceService struct {
	sdk *gosdk.SDK
}

// NewNebiusInstanceService creates a new NebiusInstanceService.
func NewNebiusInstanceService(sdk *gosdk.SDK) *NebiusInstanceService {
	return &NebiusInstanceService{sdk: sdk}
}

// CreateDisk creates a boot disk for a Nebius instance.
// It returns the created disk, or an existing disk if one with the same name already exists.
func (s *NebiusInstanceService) CreateDisk(
	ctx context.Context,
	log logr.Logger,
	projectID string,
	diskName string,
	imageFamily string,
	sizeGibibytes int32,
) (*nebiuscompute.Disk, error) {
	diskSvc := s.sdk.Services().Compute().V1().Disk()

	// Check if disk already exists by name.
	existing, err := diskSvc.GetByName(ctx, &nebiuscommon.GetByNameRequest{
		ParentId: projectID,
		Name:     diskName,
	})
	if err == nil {
		log.Info("Boot disk already exists", "diskID", existing.GetMetadata().GetId(), "diskName", diskName)
		return existing, nil
	}
	if !isNotFound(err) {
		return nil, fmt.Errorf("checking for existing disk %q: %w", diskName, err)
	}

	log.Info("Creating boot disk", "diskName", diskName, "imageFamily", imageFamily, "sizeGiB", sizeGibibytes)

	op, err := diskSvc.Create(ctx, &nebiuscompute.CreateDiskRequest{
		Metadata: &nebiuscommon.ResourceMetadata{
			ParentId: projectID,
			Name:     diskName,
		},
		Spec: &nebiuscompute.DiskSpec{
			Size: &nebiuscompute.DiskSpec_SizeGibibytes{
				SizeGibibytes: int64(sizeGibibytes),
			},
			Type: nebiuscompute.DiskSpec_NETWORK_SSD,
			Source: &nebiuscompute.DiskSpec_SourceImageFamily{
				SourceImageFamily: &nebiuscompute.SourceImageFamily{
					ImageFamily: imageFamily,
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating disk %q: %w", diskName, err)
	}

	if _, err := op.Wait(ctx); err != nil {
		return nil, fmt.Errorf("waiting for disk %q creation: %w", diskName, err)
	}

	diskID := op.ResourceID()
	log.Info("Boot disk created", "diskID", diskID, "diskName", diskName)

	disk, err := diskSvc.Get(ctx, &nebiuscompute.GetDiskRequest{Id: diskID})
	if err != nil {
		return nil, fmt.Errorf("getting created disk %q: %w", diskID, err)
	}

	return disk, nil
}

// CreateInstance creates a Nebius compute instance with the given boot disk attached.
// It returns the created instance, or an existing instance if one with the same name already exists.
func (s *NebiusInstanceService) CreateInstance(
	ctx context.Context,
	log logr.Logger,
	projectID string,
	instanceName string,
	platform string,
	preset string,
	subnetID string,
	bootDiskID string,
	cloudInitUserData string,
) (*nebiuscompute.Instance, error) {
	instanceSvc := s.sdk.Services().Compute().V1().Instance()

	// Check if instance already exists by name.
	existing, err := instanceSvc.GetByName(ctx, &nebiuscommon.GetByNameRequest{
		ParentId: projectID,
		Name:     instanceName,
	})
	if err == nil {
		log.Info("Instance already exists", "instanceID", existing.GetMetadata().GetId(), "instanceName", instanceName)
		return existing, nil
	}
	if !isNotFound(err) {
		return nil, fmt.Errorf("checking for existing instance %q: %w", instanceName, err)
	}

	log.Info("Creating instance", "instanceName", instanceName, "platform", platform, "preset", preset)

	op, err := instanceSvc.Create(ctx, &nebiuscompute.CreateInstanceRequest{
		Metadata: &nebiuscommon.ResourceMetadata{
			ParentId: projectID,
			Name:     instanceName,
		},
		Spec: &nebiuscompute.InstanceSpec{
			Resources: &nebiuscompute.ResourcesSpec{
				Platform: platform,
				Size: &nebiuscompute.ResourcesSpec_Preset{
					Preset: preset,
				},
			},
			BootDisk: &nebiuscompute.AttachedDiskSpec{
				AttachMode: nebiuscompute.AttachedDiskSpec_READ_WRITE,
				Type: &nebiuscompute.AttachedDiskSpec_ExistingDisk{
					ExistingDisk: &nebiuscompute.ExistingDisk{
						Id: bootDiskID,
					},
				},
			},
			NetworkInterfaces: []*nebiuscompute.NetworkInterfaceSpec{
				{
					SubnetId:  subnetID,
					Name:      "eth0",
					IpAddress: &nebiuscompute.IPAddress{
						// Auto-allocate private IP.
					},
				},
			},
			CloudInitUserData: cloudInitUserData,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating instance %q: %w", instanceName, err)
	}

	if _, err := op.Wait(ctx); err != nil {
		return nil, fmt.Errorf("waiting for instance %q creation: %w", instanceName, err)
	}

	instanceID := op.ResourceID()
	log.Info("Instance created", "instanceID", instanceID, "instanceName", instanceName)

	instance, err := instanceSvc.Get(ctx, &nebiuscompute.GetInstanceRequest{Id: instanceID})
	if err != nil {
		return nil, fmt.Errorf("getting created instance %q: %w", instanceID, err)
	}

	return instance, nil
}

// GetInstance returns a Nebius instance by ID.
// It returns nil, nil if the instance is not found.
func (s *NebiusInstanceService) GetInstance(ctx context.Context, instanceID string) (*nebiuscompute.Instance, error) {
	instance, err := s.sdk.Services().Compute().V1().Instance().Get(ctx, &nebiuscompute.GetInstanceRequest{
		Id: instanceID,
	})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting instance %q: %w", instanceID, err)
	}
	return instance, nil
}

// GetInstanceByName returns a Nebius instance by name within a project.
// It returns nil, nil if the instance is not found.
func (s *NebiusInstanceService) GetInstanceByName(ctx context.Context, projectID, instanceName string) (*nebiuscompute.Instance, error) {
	instance, err := s.sdk.Services().Compute().V1().Instance().GetByName(ctx, &nebiuscommon.GetByNameRequest{
		ParentId: projectID,
		Name:     instanceName,
	})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting instance by name %q: %w", instanceName, err)
	}
	return instance, nil
}

// DeleteInstance deletes a Nebius instance by ID.
// It is idempotent: returns nil if the instance is already deleted.
func (s *NebiusInstanceService) DeleteInstance(ctx context.Context, log logr.Logger, instanceID string) error {
	if instanceID == "" {
		return nil
	}

	log.Info("Deleting instance", "instanceID", instanceID)

	op, err := s.sdk.Services().Compute().V1().Instance().Delete(ctx, &nebiuscompute.DeleteInstanceRequest{
		Id: instanceID,
	})
	if err != nil {
		if isNotFound(err) {
			log.Info("Instance already deleted", "instanceID", instanceID)
			return nil
		}
		return fmt.Errorf("deleting instance %q: %w", instanceID, err)
	}

	if _, err := op.Wait(ctx); err != nil {
		// If the instance was deleted between the Delete call and the Wait, treat it as success.
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("waiting for instance %q deletion: %w", instanceID, err)
	}

	log.Info("Instance deleted", "instanceID", instanceID)
	return nil
}

// DeleteDisk deletes a Nebius disk by ID.
// It is idempotent: returns nil if the disk is already deleted.
func (s *NebiusInstanceService) DeleteDisk(ctx context.Context, log logr.Logger, diskID string) error {
	if diskID == "" {
		return nil
	}

	log.Info("Deleting disk", "diskID", diskID)

	op, err := s.sdk.Services().Compute().V1().Disk().Delete(ctx, &nebiuscompute.DeleteDiskRequest{
		Id: diskID,
	})
	if err != nil {
		if isNotFound(err) {
			log.Info("Disk already deleted", "diskID", diskID)
			return nil
		}
		return fmt.Errorf("deleting disk %q: %w", diskID, err)
	}

	if _, err := op.Wait(ctx); err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("waiting for disk %q deletion: %w", diskID, err)
	}

	log.Info("Disk deleted", "diskID", diskID)
	return nil
}

// isNotFound returns true if the error is a gRPC NotFound error.
func isNotFound(err error) bool {
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.NotFound
	}
	return false
}
