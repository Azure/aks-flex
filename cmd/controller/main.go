package main

import (
	"context"
	"time"

	"github.com/Azure/karpenter-provider-azure/pkg/cloudprovider"
	"github.com/Azure/karpenter-provider-azure/pkg/controllers"
	"github.com/Azure/karpenter-provider-azure/pkg/operator"
	"github.com/Azure/karpenter-provider-azure/pkg/operator/options"
	"github.com/go-logr/zapr"
	"github.com/samber/lo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	"sigs.k8s.io/karpenter/pkg/cloudprovider/overlay"
	corecontrollers "sigs.k8s.io/karpenter/pkg/controllers"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"
	"sigs.k8s.io/karpenter/pkg/operator/injection"
	"sigs.k8s.io/karpenter/pkg/operator/logging"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"

	flexcontrollers "github.com/Azure/karpenter-provider-flex/pkg/controllers"
)

func main() {
	ctx := injection.WithOptionsOrDie(context.Background(), coreoptions.Injectables...)
	logger := zapr.NewLogger(logging.NewLogger(ctx, "controller"))
	lo.Must0(operator.WaitForCRDs(ctx, 2*time.Minute, ctrl.GetConfigOrDie(), logger), "failed waiting for CRDs")

	ctx, op := operator.NewOperator(coreoperator.NewOperator())

	// TODO: Consider also dumping at least some core options
	logger.V(0).Info("Initial options", "options", options.FromContext(ctx).String())

	aksCloudProvider := cloudprovider.New(
		op.InstanceTypesProvider,
		op.VMInstanceProvider,
		op.AKSMachineProvider,
		op.EventRecorder,
		op.GetClient(),
		op.ImageProvider,
		op.InstanceTypeStore,
	)

	lo.Must0(op.AddHealthzCheck("cloud-provider", aksCloudProvider.LivenessProbe))

	overlayUndecoratedCloudProvider := metrics.Decorate(aksCloudProvider)
	cloudProvider := overlay.Decorate(overlayUndecoratedCloudProvider, op.GetClient(), op.InstanceTypeStore)
	clusterState := state.NewCluster(op.Clock, op.GetClient(), cloudProvider)

	op.
		WithControllers(ctx, corecontrollers.NewControllers(
			ctx,
			op.Manager,
			op.Clock,
			op.GetClient(),
			op.EventRecorder,
			cloudProvider,
			overlayUndecoratedCloudProvider,
			clusterState,
			op.InstanceTypeStore,
		)...).
		WithControllers(ctx, controllers.NewControllers(
			ctx,
			op.Manager,
			op.GetClient(),
			op.EventRecorder,
			aksCloudProvider,
			op.VMInstanceProvider,
			op.AKSMachineProvider,
			// TODO: still need to refactor ImageProvider side of things.
			op.KubernetesVersionProvider,
			op.ImageProvider,
			op.InClusterKubernetesInterface,
			op.AZClient.SubnetsClient(),
		)...).
		WithControllers(ctx, flexcontrollers.NewControllers(
			ctx,
			op.GetClient(),
			op.EventRecorder,
		)...).
		Start(ctx)
}
