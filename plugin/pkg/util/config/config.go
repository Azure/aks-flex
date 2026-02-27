package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/ini.v1"
)

var (
	rxSubscriptionID    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$`)
	rxResourceGroupName = regexp.MustCompile(`(?i)^[-0-9a-z()_.]{0,89}[-0-9a-z()_]$`)
	rxLocation          = regexp.MustCompile(`(?i)^[0-9a-z]{1,32}$`)
)

type Config struct {
	// Azure fields
	SubscriptionID    string
	ResourceGroupName string
	Location          string
	ClusterName       string
	AKSNodeVMSize     string
	StretchNodeVMSize string
	StretchNodeZones  []string

	// GCP fields
	GCPProjectID string
	GCPZone      string
	GCPRegion    string

	// OCI fields
	OCITenancyID          string
	OCIParentCompartment  string
	OCICompartmentName    string
	OCIRegion             string
	OCIAvailabilityDomain string

	// Nebius fields
	NebiusProjectID       string
	NebiusRegion          string
	NebiusCredentialsFile string
}

func New() (*Config, error) {
	c := &Config{
		SubscriptionID:    defaultSubscriptionID(),
		ResourceGroupName: defaultResourceGroupName(),
		Location:          defaultLocation(),
		ClusterName:       defaultClusterName(),
		AKSNodeVMSize:     defaultAKSNodeVMSize(),
		StretchNodeVMSize: defaultStretchNodeVMSize(),
		StretchNodeZones:  defaultStretchNodeZones(),

		GCPProjectID: defaultGCPProjectID(),
		GCPZone:      defaultGCPZone(),
		GCPRegion:    defaultGCPRegion(),

		OCITenancyID:          defaultOCITenancyID(),
		OCIParentCompartment:  defaultOCIParentCompartment(),
		OCICompartmentName:    defaultOCICompartmentName(),
		OCIRegion:             defaultOCIRegion(),
		OCIAvailabilityDomain: defaultOCIAvailabilityDomain(),
		NebiusProjectID:       defaultNebiusProjectID(),
		NebiusRegion:          defaultNebiusRegion(),
		NebiusCredentialsFile: defaultNebiusCredentialsFile(),
	}

	if err := c.validate(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Config) validate() error {
	if !rxSubscriptionID.MatchString(c.SubscriptionID) {
		return fmt.Errorf("AZURE_SUBSCRIPTION_ID is missing or invalid (expected a UUID)")
	}

	if !rxResourceGroupName.MatchString(c.ResourceGroupName) {
		return fmt.Errorf("RESOURCE_GROUP_NAME is missing or invalid")
	}

	if !rxLocation.MatchString(c.Location) {
		return fmt.Errorf("LOCATION is missing or invalid (e.g. eastus, westeurope)")
	}

	if c.ClusterName == "" {
		return fmt.Errorf("CLUSTER_NAME is empty")
	}

	return nil
}

func defaultSubscriptionID() string {
	if subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID"); subscriptionID != "" {
		return subscriptionID
	}

	b, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".azure/clouds.config"))
	if err != nil {
		return ""
	}

	f, err := ini.Load(b)
	if err != nil {
		return ""
	}

	return f.Section("AzureCloud").Key("subscription").String()
}

func defaultResourceGroupName() string {
	if resourceGroupName := os.Getenv("RESOURCE_GROUP_NAME"); resourceGroupName != "" {
		return resourceGroupName
	}

	return os.Getenv("USER")
}

func defaultLocation() string {
	return os.Getenv("LOCATION")
}

func defaultClusterName() string {
	if clusterName := os.Getenv("CLUSTER_NAME"); clusterName != "" {
		return clusterName
	}
	return "aks"
}

func defaultAKSNodeVMSize() string {
	if vmSize := os.Getenv("AKS_NODE_VM_SIZE"); vmSize != "" {
		return vmSize
	}
	return "Standard_D2s_v3"
}

func defaultStretchNodeVMSize() string {
	if vmSize := os.Getenv("STRETCH_NODE_VM_SIZE"); vmSize != "" {
		return vmSize
	}
	return "Standard_D2ds_v5"
}

func defaultStretchNodeZones() []string {
	zonesStr := os.Getenv("STRETCH_NODE_ZONES")
	if zonesStr == "" {
		// Default: zones 1,2,3 for HA (backwards compatible)
		return []string{"1", "2", "3"}
	}
	if zonesStr == "none" {
		return nil // Explicitly disable zones
	}
	// Split comma-separated zones: "1,2,3" -> ["1", "2", "3"]
	zones := make([]string, 0)
	for _, z := range strings.Split(zonesStr, ",") {
		zone := strings.TrimSpace(z)
		if zone != "" {
			zones = append(zones, zone)
		}
	}
	return zones
}

func defaultGCPProjectID() string {
	return os.Getenv("GOOGLE_CLOUD_PROJECT")
}

func defaultGCPZone() string {
	return os.Getenv("GOOGLE_CLOUD_ZONE")
}

func defaultGCPRegion() string {
	return os.Getenv("GOOGLE_CLOUD_REGION")
}

func defaultOCITenancyID() string {
	return os.Getenv("OCI_TENANCY_OCID")
}

func defaultOCIParentCompartment() string {
	return os.Getenv("OCI_PARENT_COMPARTMENT")
}

func defaultOCICompartmentName() string {
	if name := os.Getenv("OCI_COMPARTMENT_NAME"); name != "" {
		return name
	}
	return "stretch"
}

func defaultOCIRegion() string {
	return os.Getenv("OCI_REGION")
}

func defaultOCIAvailabilityDomain() string {
	return os.Getenv("OCI_AVAILABILITY_DOMAIN")
}

func defaultNebiusProjectID() string {
	return os.Getenv("NEBIUS_PROJECT_ID")
}

func defaultNebiusRegion() string {
	return os.Getenv("NEBIUS_REGION")
}

func defaultNebiusCredentialsFile() string {
	return os.Getenv("NEBIUS_CREDENTIALS_FILE")
}
