package az

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"

	"github.com/Azure/aks-flex/flex-plugin/pkg/util/config"
)

func Deploy(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config, deploymentName string, templateb []byte, parameters map[string]*armresources.DeploymentParameter) error {
	var template any
	if err := json.Unmarshal(templateb, &template); err != nil {
		return err
	}

	resourceGroups, err := armresources.NewResourceGroupsClient(cfg.SubscriptionID, credentials, nil)
	if err != nil {
		return err
	}

	deployments, err := armresources.NewDeploymentsClient(cfg.SubscriptionID, credentials, nil)
	if err != nil {
		return err
	}

	_, err = resourceGroups.CreateOrUpdate(ctx, cfg.ResourceGroupName, armresources.ResourceGroup{
		Location: &cfg.Location,
	}, nil)
	if err != nil {
		return err
	}

	poller, err := deployments.BeginCreateOrUpdate(ctx, cfg.ResourceGroupName, deploymentName, armresources.Deployment{
		Properties: &armresources.DeploymentProperties{
			Template:   template,
			Parameters: parameters,
			Mode:       to.Ptr(armresources.DeploymentModeIncremental),
		},
	}, nil)
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
		Frequency: 5 * time.Second,
	})
	return err
}
