package az

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"

	"github.com/Azure/aks-flex/plugin/pkg/util/config"
)

func PublicIPAddress(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config) (*armnetwork.PublicIPAddress, error) {
	publicIPAddresses, err := armnetwork.NewPublicIPAddressesClient(cfg.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, err
	}

	pip, err := publicIPAddresses.Get(ctx, cfg.ResourceGroupName, "gw", nil)
	if err != nil {
		return nil, err
	}

	return &pip.PublicIPAddress, nil
}
