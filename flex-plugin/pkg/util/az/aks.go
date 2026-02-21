package az

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v8"

	"github.com/Azure/aks-flex/flex-plugin/pkg/util/config"
)

func ManagedCluster(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config) (*armcontainerservice.ManagedCluster, error) {
	managedClusters, err := armcontainerservice.NewManagedClustersClient(cfg.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, err
	}

	mc, err := managedClusters.Get(ctx, cfg.ResourceGroupName, cfg.ClusterName, nil)
	if err != nil {
		return nil, err
	}

	return &mc.ManagedCluster, nil
}
