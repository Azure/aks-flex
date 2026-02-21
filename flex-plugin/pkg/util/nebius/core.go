package nebius

import (
	"context"
	"fmt"
	"log"

	"github.com/nebius/gosdk"
	common "github.com/nebius/gosdk/proto/nebius/common/v1"
	compute "github.com/nebius/gosdk/proto/nebius/compute/v1"
	vpc "github.com/nebius/gosdk/proto/nebius/vpc/v1"
)

// =============================================================================
// Network Entity
// =============================================================================

// NetworkConfig holds the configuration for creating a Nebius network.
type NetworkConfig struct {
	ProjectID string
	Name      string
	CIDR      string // CIDR for the network pool and subnet (e.g., "172.20.0.0/16")
}

// Network represents a Nebius network (pool + VPC + subnet) with lifecycle methods.
type Network struct {
	cfg NetworkConfig
	sdk *gosdk.SDK

	poolID    string
	networkID string
	subnetID  string
}

// NewNetwork creates a new Network entity from config.
func NewNetwork(sdk *gosdk.SDK, cfg NetworkConfig) *Network {
	return &Network{sdk: sdk, cfg: cfg}
}

// NetworkID returns the network ID.
func (n *Network) NetworkID() string {
	return n.networkID
}

// SubnetID returns the subnet ID.
func (n *Network) SubnetID() string {
	return n.subnetID
}

// Refresh looks up the network and subnet by name and populates the IDs.
func (n *Network) Refresh(ctx context.Context) error {
	poolService := n.sdk.Services().VPC().V1().Pool()
	networkService := n.sdk.Services().VPC().V1().Network()
	subnetService := n.sdk.Services().VPC().V1().Subnet()

	// Look up pool by name
	poolName := fmt.Sprintf("%s-pool", n.cfg.Name)
	pool, err := poolService.GetByName(ctx, &vpc.GetPoolByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     poolName,
	})
	if err != nil {
		return fmt.Errorf("pool %q not found: %w", poolName, err)
	}
	n.poolID = pool.GetMetadata().GetId()

	// Look up network by name
	network, err := networkService.GetByName(ctx, &vpc.GetNetworkByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     n.cfg.Name,
	})
	if err != nil {
		return fmt.Errorf("network %q not found: %w", n.cfg.Name, err)
	}
	n.networkID = network.GetMetadata().GetId()

	// Look up subnet by name
	subnetName := fmt.Sprintf("%s-subnet", n.cfg.Name)
	subnet, err := subnetService.GetByName(ctx, &vpc.GetSubnetByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     subnetName,
	})
	if err != nil {
		return fmt.Errorf("subnet %q not found: %w", subnetName, err)
	}
	n.subnetID = subnet.GetMetadata().GetId()

	return nil
}

// Provision creates the pool, network and subnet if they don't exist.
func (n *Network) Provision(ctx context.Context) error {
	// Create pool with custom CIDR
	if err := n.createPool(ctx); err != nil {
		return err
	}

	// Create network using the pool
	if err := n.createNetwork(ctx); err != nil {
		return err
	}

	// Create subnet
	if err := n.createSubnet(ctx); err != nil {
		return err
	}

	return nil
}

func (n *Network) createPool(ctx context.Context) error {
	poolService := n.sdk.Services().VPC().V1().Pool()
	poolName := fmt.Sprintf("%s-pool", n.cfg.Name)

	// Check if pool already exists
	existing, err := poolService.GetByName(ctx, &vpc.GetPoolByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     poolName,
	})
	if err == nil {
		n.poolID = existing.GetMetadata().GetId()
		log.Printf("Pool %s already exists: %s", poolName, n.poolID)
		return nil
	}
	if !isNotFound(err) {
		return fmt.Errorf("failed to check for existing pool: %w", err)
	}

	log.Printf("Creating pool %s with CIDR %s...", poolName, n.cfg.CIDR)

	op, err := poolService.Create(ctx, &vpc.CreatePoolRequest{
		Metadata: &common.ResourceMetadata{
			ParentId: n.cfg.ProjectID,
			Name:     poolName,
		},
		Spec: &vpc.PoolSpec{
			Version:    vpc.IpVersion_IPV4,
			Visibility: vpc.IpVisibility_PRIVATE,
			Cidrs: []*vpc.PoolCidr{
				{
					Cidr: n.cfg.CIDR,
				},
			},
		},
	})
	if err != nil {
		if isAlreadyExists(err) {
			log.Printf("Pool %s already exists, looking it up...", poolName)
			return n.refreshPool(ctx, poolName)
		}
		return fmt.Errorf("failed to create pool: %w", err)
	}

	// Wait for completion
	op, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for pool creation: %w", err)
	}

	n.poolID = op.ResourceID()
	log.Printf("Pool %s created: %s", poolName, n.poolID)
	return nil
}

func (n *Network) refreshPool(ctx context.Context, poolName string) error {
	poolService := n.sdk.Services().VPC().V1().Pool()
	pool, err := poolService.GetByName(ctx, &vpc.GetPoolByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     poolName,
	})
	if err != nil {
		return fmt.Errorf("failed to get pool: %w", err)
	}
	n.poolID = pool.GetMetadata().GetId()
	return nil
}

func (n *Network) createNetwork(ctx context.Context) error {
	networkService := n.sdk.Services().VPC().V1().Network()

	// Check if network already exists
	existing, err := networkService.GetByName(ctx, &vpc.GetNetworkByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     n.cfg.Name,
	})
	if err == nil {
		n.networkID = existing.GetMetadata().GetId()
		log.Printf("Network %s already exists: %s", n.cfg.Name, n.networkID)
		return nil
	}
	if !isNotFound(err) {
		return fmt.Errorf("failed to check for existing network: %w", err)
	}

	log.Printf("Creating network %s with pool %s...", n.cfg.Name, n.poolID)

	op, err := networkService.Create(ctx, &vpc.CreateNetworkRequest{
		Metadata: &common.ResourceMetadata{
			ParentId: n.cfg.ProjectID,
			Name:     n.cfg.Name,
		},
		Spec: &vpc.NetworkSpec{
			Ipv4PrivatePools: &vpc.IPv4PrivateNetworkPools{
				Pools: []*vpc.NetworkPool{
					{
						Id: n.poolID,
					},
				},
			},
		},
	})
	if err != nil {
		if isAlreadyExists(err) {
			log.Printf("Network %s already exists, looking it up...", n.cfg.Name)
			return n.refreshNetwork(ctx)
		}
		return fmt.Errorf("failed to create network: %w", err)
	}

	// Wait for completion
	op, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for network creation: %w", err)
	}

	n.networkID = op.ResourceID()
	log.Printf("Network %s created: %s", n.cfg.Name, n.networkID)
	return nil
}

func (n *Network) refreshNetwork(ctx context.Context) error {
	networkService := n.sdk.Services().VPC().V1().Network()
	network, err := networkService.GetByName(ctx, &vpc.GetNetworkByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     n.cfg.Name,
	})
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	n.networkID = network.GetMetadata().GetId()
	return nil
}

func (n *Network) createSubnet(ctx context.Context) error {
	subnetService := n.sdk.Services().VPC().V1().Subnet()
	subnetName := fmt.Sprintf("%s-subnet", n.cfg.Name)

	// Check if subnet already exists
	existing, err := subnetService.GetByName(ctx, &vpc.GetSubnetByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     subnetName,
	})
	if err == nil {
		n.subnetID = existing.GetMetadata().GetId()
		log.Printf("Subnet %s already exists: %s", subnetName, n.subnetID)
		return nil
	}
	if !isNotFound(err) {
		return fmt.Errorf("failed to check for existing subnet: %w", err)
	}

	log.Printf("Creating subnet %s with CIDR %s...", subnetName, n.cfg.CIDR)

	op, err := subnetService.Create(ctx, &vpc.CreateSubnetRequest{
		Metadata: &common.ResourceMetadata{
			ParentId: n.cfg.ProjectID,
			Name:     subnetName,
		},
		Spec: &vpc.SubnetSpec{
			NetworkId: n.networkID,
			Ipv4PrivatePools: &vpc.IPv4PrivateSubnetPools{
				// Inherit from network pools (which uses our custom pool)
				// When UseNetworkPools is true, Pools must be empty
				UseNetworkPools: true,
			},
		},
	})
	if err != nil {
		if isAlreadyExists(err) {
			log.Printf("Subnet %s already exists, looking it up...", subnetName)
			return n.refreshSubnet(ctx, subnetName)
		}
		return fmt.Errorf("failed to create subnet: %w", err)
	}

	// Wait for completion
	op, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for subnet creation: %w", err)
	}

	n.subnetID = op.ResourceID()
	log.Printf("Subnet %s created: %s", subnetName, n.subnetID)
	return nil
}

func (n *Network) refreshSubnet(ctx context.Context, subnetName string) error {
	subnetService := n.sdk.Services().VPC().V1().Subnet()
	subnet, err := subnetService.GetByName(ctx, &vpc.GetSubnetByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     subnetName,
	})
	if err != nil {
		return fmt.Errorf("failed to get subnet: %w", err)
	}
	n.subnetID = subnet.GetMetadata().GetId()
	return nil
}

// Delete deletes the network, subnet, and pool.
func (n *Network) Delete(ctx context.Context) error {
	// Refresh to get IDs (partial refresh is OK)
	n.refreshAll(ctx)

	// Delete subnet first (depends on network)
	if err := n.deleteSubnet(ctx); err != nil {
		return err
	}

	// Delete network (depends on pool)
	if err := n.deleteNetwork(ctx); err != nil {
		return err
	}

	// Delete pool
	if err := n.deletePool(ctx); err != nil {
		return err
	}

	return nil
}

func (n *Network) refreshAll(ctx context.Context) {
	poolService := n.sdk.Services().VPC().V1().Pool()
	networkService := n.sdk.Services().VPC().V1().Network()
	subnetService := n.sdk.Services().VPC().V1().Subnet()

	poolName := fmt.Sprintf("%s-pool", n.cfg.Name)
	if pool, err := poolService.GetByName(ctx, &vpc.GetPoolByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     poolName,
	}); err == nil {
		n.poolID = pool.GetMetadata().GetId()
	}

	if network, err := networkService.GetByName(ctx, &vpc.GetNetworkByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     n.cfg.Name,
	}); err == nil {
		n.networkID = network.GetMetadata().GetId()
	}

	subnetName := fmt.Sprintf("%s-subnet", n.cfg.Name)
	if subnet, err := subnetService.GetByName(ctx, &vpc.GetSubnetByNameRequest{
		ParentId: n.cfg.ProjectID,
		Name:     subnetName,
	}); err == nil {
		n.subnetID = subnet.GetMetadata().GetId()
	}
}

func (n *Network) deleteSubnet(ctx context.Context) error {
	if n.subnetID == "" {
		return nil
	}

	subnetService := n.sdk.Services().VPC().V1().Subnet()
	subnetName := fmt.Sprintf("%s-subnet", n.cfg.Name)

	log.Printf("Deleting subnet %s...", subnetName)

	op, err := subnetService.Delete(ctx, &vpc.DeleteSubnetRequest{
		Id: n.subnetID,
	})
	if err != nil {
		if isNotFound(err) {
			log.Printf("Subnet %s not found, skipping", subnetName)
			return nil
		}
		return fmt.Errorf("failed to delete subnet: %w", err)
	}

	// Wait for completion
	_, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for subnet deletion: %w", err)
	}

	log.Printf("Subnet %s deleted", subnetName)
	return nil
}

func (n *Network) deleteNetwork(ctx context.Context) error {
	if n.networkID == "" {
		return nil
	}

	networkService := n.sdk.Services().VPC().V1().Network()

	log.Printf("Deleting network %s...", n.cfg.Name)

	op, err := networkService.Delete(ctx, &vpc.DeleteNetworkRequest{
		Id: n.networkID,
	})
	if err != nil {
		if isNotFound(err) {
			log.Printf("Network %s not found, skipping", n.cfg.Name)
			return nil
		}
		return fmt.Errorf("failed to delete network: %w", err)
	}

	// Wait for completion
	_, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for network deletion: %w", err)
	}

	log.Printf("Network %s deleted", n.cfg.Name)
	return nil
}

func (n *Network) deletePool(ctx context.Context) error {
	if n.poolID == "" {
		return nil
	}

	poolService := n.sdk.Services().VPC().V1().Pool()
	poolName := fmt.Sprintf("%s-pool", n.cfg.Name)

	log.Printf("Deleting pool %s...", poolName)

	op, err := poolService.Delete(ctx, &vpc.DeletePoolRequest{
		Id: n.poolID,
	})
	if err != nil {
		if isNotFound(err) {
			log.Printf("Pool %s not found, skipping", poolName)
			return nil
		}
		return fmt.Errorf("failed to delete pool: %w", err)
	}

	// Wait for completion
	_, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for pool deletion: %w", err)
	}

	log.Printf("Pool %s deleted", poolName)
	return nil
}

// =============================================================================
// Instance Entity
// =============================================================================

// InstanceConfig holds the configuration for creating a Nebius compute instance.
type InstanceConfig struct {
	ProjectID      string
	Name           string
	Platform       string // e.g., "cpu-d3", "cpu-e2"
	Preset         string // e.g., "4vcpu-16gb"
	SubnetID       string
	ImageFamily    string // e.g., "ubuntu24.04-driverless"
	DiskSizeGB     int64
	CloudInitData  string // cloud-init user data
	CreatePublicIP bool   // whether to assign a public IP
}

// Instance represents a Nebius compute instance with lifecycle methods.
type Instance struct {
	cfg InstanceConfig
	sdk *gosdk.SDK

	diskID     string
	instanceID string
}

// NewInstance creates a new Instance entity from config.
func NewInstance(sdk *gosdk.SDK, cfg InstanceConfig) *Instance {
	return &Instance{sdk: sdk, cfg: cfg}
}

// InstanceID returns the instance ID.
func (i *Instance) InstanceID() string {
	return i.instanceID
}

// Provision creates the boot disk and instance if they don't exist.
func (i *Instance) Provision(ctx context.Context) error {
	// Create boot disk
	if err := i.createDisk(ctx); err != nil {
		return err
	}

	// Create instance
	if err := i.createInstance(ctx); err != nil {
		return err
	}

	return nil
}

func (i *Instance) createDisk(ctx context.Context) error {
	diskService := i.sdk.Services().Compute().V1().Disk()
	diskName := fmt.Sprintf("%s-boot", i.cfg.Name)

	// Check if disk already exists
	existing, err := diskService.GetByName(ctx, &common.GetByNameRequest{
		ParentId: i.cfg.ProjectID,
		Name:     diskName,
	})
	if err == nil {
		i.diskID = existing.GetMetadata().GetId()
		log.Printf("Disk %s already exists: %s", diskName, i.diskID)
		return nil
	}
	if !isNotFound(err) {
		return fmt.Errorf("failed to check for existing disk: %w", err)
	}

	log.Printf("Creating boot disk %s with image family %s...", diskName, i.cfg.ImageFamily)

	op, err := diskService.Create(ctx, &compute.CreateDiskRequest{
		Metadata: &common.ResourceMetadata{
			ParentId: i.cfg.ProjectID,
			Name:     diskName,
		},
		Spec: &compute.DiskSpec{
			Size: &compute.DiskSpec_SizeGibibytes{
				SizeGibibytes: i.cfg.DiskSizeGB,
			},
			Type: compute.DiskSpec_NETWORK_SSD,
			Source: &compute.DiskSpec_SourceImageFamily{
				SourceImageFamily: &compute.SourceImageFamily{
					ImageFamily: i.cfg.ImageFamily,
				},
			},
		},
	})
	if err != nil {
		if isAlreadyExists(err) {
			log.Printf("Disk %s already exists, looking it up...", diskName)
			return i.refreshDisk(ctx, diskName)
		}
		return fmt.Errorf("failed to create disk: %w", err)
	}

	op, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for disk creation: %w", err)
	}

	i.diskID = op.ResourceID()
	log.Printf("Disk %s created: %s", diskName, i.diskID)
	return nil
}

func (i *Instance) refreshDisk(ctx context.Context, diskName string) error {
	diskService := i.sdk.Services().Compute().V1().Disk()
	disk, err := diskService.GetByName(ctx, &common.GetByNameRequest{
		ParentId: i.cfg.ProjectID,
		Name:     diskName,
	})
	if err != nil {
		return fmt.Errorf("failed to get disk: %w", err)
	}
	i.diskID = disk.GetMetadata().GetId()
	return nil
}

func (i *Instance) createInstance(ctx context.Context) error {
	instanceService := i.sdk.Services().Compute().V1().Instance()

	// Check if instance already exists
	existing, err := instanceService.GetByName(ctx, &common.GetByNameRequest{
		ParentId: i.cfg.ProjectID,
		Name:     i.cfg.Name,
	})
	if err == nil {
		i.instanceID = existing.GetMetadata().GetId()
		log.Printf("Instance %s already exists: %s", i.cfg.Name, i.instanceID)
		return nil
	}
	if !isNotFound(err) {
		return fmt.Errorf("failed to check for existing instance: %w", err)
	}

	log.Printf("Creating instance %s (platform=%s, preset=%s)...", i.cfg.Name, i.cfg.Platform, i.cfg.Preset)

	nic := &compute.NetworkInterfaceSpec{
		SubnetId:  i.cfg.SubnetID,
		Name:      "eth0",
		IpAddress: &compute.IPAddress{
			// Auto-allocate private IP
		},
	}
	if i.cfg.CreatePublicIP {
		nic.PublicIpAddress = &compute.PublicIPAddress{}
	}

	spec := &compute.InstanceSpec{
		Resources: &compute.ResourcesSpec{
			Platform: i.cfg.Platform,
			Size: &compute.ResourcesSpec_Preset{
				Preset: i.cfg.Preset,
			},
		},
		BootDisk: &compute.AttachedDiskSpec{
			AttachMode: compute.AttachedDiskSpec_READ_WRITE,
			Type: &compute.AttachedDiskSpec_ExistingDisk{
				ExistingDisk: &compute.ExistingDisk{
					Id: i.diskID,
				},
			},
		},
		NetworkInterfaces: []*compute.NetworkInterfaceSpec{
			nic,
		},
		CloudInitUserData: i.cfg.CloudInitData,
	}

	op, err := instanceService.Create(ctx, &compute.CreateInstanceRequest{
		Metadata: &common.ResourceMetadata{
			ParentId: i.cfg.ProjectID,
			Name:     i.cfg.Name,
		},
		Spec: spec,
	})
	if err != nil {
		if isAlreadyExists(err) {
			log.Printf("Instance %s already exists, looking it up...", i.cfg.Name)
			return i.refreshInstance(ctx)
		}
		return fmt.Errorf("failed to create instance: %w", err)
	}

	op, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for instance creation: %w", err)
	}

	i.instanceID = op.ResourceID()
	log.Printf("Instance %s created: %s", i.cfg.Name, i.instanceID)
	return nil
}

func (i *Instance) refreshInstance(ctx context.Context) error {
	instanceService := i.sdk.Services().Compute().V1().Instance()
	instance, err := instanceService.GetByName(ctx, &common.GetByNameRequest{
		ParentId: i.cfg.ProjectID,
		Name:     i.cfg.Name,
	})
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}
	i.instanceID = instance.GetMetadata().GetId()
	return nil
}

// Delete deletes the instance and boot disk.
func (i *Instance) Delete(ctx context.Context) error {
	// Refresh to get IDs
	i.refreshAll(ctx)

	// Delete instance first
	if err := i.deleteInstance(ctx); err != nil {
		return err
	}

	// Delete boot disk
	if err := i.deleteDisk(ctx); err != nil {
		return err
	}

	return nil
}

func (i *Instance) refreshAll(ctx context.Context) {
	diskService := i.sdk.Services().Compute().V1().Disk()
	instanceService := i.sdk.Services().Compute().V1().Instance()

	diskName := fmt.Sprintf("%s-boot", i.cfg.Name)
	if disk, err := diskService.GetByName(ctx, &common.GetByNameRequest{
		ParentId: i.cfg.ProjectID,
		Name:     diskName,
	}); err == nil {
		i.diskID = disk.GetMetadata().GetId()
	}

	if instance, err := instanceService.GetByName(ctx, &common.GetByNameRequest{
		ParentId: i.cfg.ProjectID,
		Name:     i.cfg.Name,
	}); err == nil {
		i.instanceID = instance.GetMetadata().GetId()
	}
}

func (i *Instance) deleteInstance(ctx context.Context) error {
	if i.instanceID == "" {
		return nil
	}

	instanceService := i.sdk.Services().Compute().V1().Instance()

	log.Printf("Deleting instance %s...", i.cfg.Name)

	op, err := instanceService.Delete(ctx, &compute.DeleteInstanceRequest{
		Id: i.instanceID,
	})
	if err != nil {
		if isNotFound(err) {
			log.Printf("Instance %s not found, skipping", i.cfg.Name)
			return nil
		}
		return fmt.Errorf("failed to delete instance: %w", err)
	}

	_, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for instance deletion: %w", err)
	}

	log.Printf("Instance %s deleted", i.cfg.Name)
	return nil
}

func (i *Instance) deleteDisk(ctx context.Context) error {
	if i.diskID == "" {
		return nil
	}

	diskService := i.sdk.Services().Compute().V1().Disk()
	diskName := fmt.Sprintf("%s-boot", i.cfg.Name)

	log.Printf("Deleting disk %s...", diskName)

	op, err := diskService.Delete(ctx, &compute.DeleteDiskRequest{
		Id: i.diskID,
	})
	if err != nil {
		if isNotFound(err) {
			log.Printf("Disk %s not found, skipping", diskName)
			return nil
		}
		return fmt.Errorf("failed to delete disk: %w", err)
	}

	_, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for disk deletion: %w", err)
	}

	log.Printf("Disk %s deleted", diskName)
	return nil
}
