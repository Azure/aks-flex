package controllers

import (
	"context"

	"github.com/Azure/karpenter-provider-flex/pkg/controllers/nodes"
	"github.com/awslabs/operatorpkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/karpenter/pkg/events"
)

func NewControllers(
	ctx context.Context,
	kubeClient client.Client,
	recorder events.Recorder,
) []controller.Controller {
	return []controller.Controller{
		nodes.NewSetProviderIDController(kubeClient),
	}
}
