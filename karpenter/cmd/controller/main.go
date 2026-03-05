package main

import (
	"context"
	"time"

	"github.com/Azure/karpenter-provider-azure/pkg/apis"
	"github.com/Azure/karpenter-provider-azure/pkg/cloudprovider"
	"github.com/Azure/karpenter-provider-azure/pkg/controllers"
	"github.com/Azure/karpenter-provider-azure/pkg/operator"
	"github.com/Azure/karpenter-provider-azure/pkg/operator/options"
	"github.com/go-logr/zapr"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	"sigs.k8s.io/karpenter/pkg/cloudprovider/overlay"
	corecontrollers "sigs.k8s.io/karpenter/pkg/controllers"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"
	"sigs.k8s.io/karpenter/pkg/operator/injection"
	"sigs.k8s.io/karpenter/pkg/operator/logging"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"

	kaitov1alpha1 "github.com/Azure/aks-flex/karpenter/pkg/apis/kaito/v1alpha1"
	"github.com/Azure/aks-flex/karpenter/pkg/apis/v1alpha1"
	flexcloudproviders "github.com/Azure/aks-flex/karpenter/pkg/cloudproviders"
	"github.com/Azure/aks-flex/karpenter/pkg/cloudproviders/kaito"
	"github.com/Azure/aks-flex/karpenter/pkg/cloudproviders/nebius"
	flexcontrollers "github.com/Azure/aks-flex/karpenter/pkg/controllers"
	flexoptions "github.com/Azure/aks-flex/karpenter/pkg/options"
	utilsk8s "github.com/Azure/aks-flex/karpenter/pkg/utils/k8s"
)

func init() {
	// FIXME: review this logic... are we sure this is the right way?
	v1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme)
	kaitov1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme)
}

func main() {
	ctx := injection.WithOptionsOrDie(context.Background(), coreoptions.Injectables...)
	logger := zapr.NewLogger(logging.NewLogger(ctx, "controller"))
	lo.Must0(
		operator.WaitForCRDs(
			ctx, 2*time.Minute, ctrl.GetConfigOrDie(), logger,
			&v1alpha1.NebiusNodeClass{},
			&kaitov1alpha1.KaitoNodeClass{},
		),
		"failed waiting for CRDs",
	)

	ctx, op := operator.NewOperator(coreoperator.NewOperator())

	// TODO: Consider also dumping at least some core options
	logger.V(0).Info("Initial options", "options", options.FromContext(ctx).String())

	flexoptions.MustInitalizeStretchPlugin(ctx, op.GetConfig())

	hubCloudProvider := flexcloudproviders.NewCloudProvidersHub()
	defer func() {
		if err := hubCloudProvider.Close(ctx); err != nil {
			logger.Error(err, "closing cloud providers")
		}
	}()

	// AKS cloud provider...
	var aksCloudProvider *cloudprovider.CloudProvider
	{
		aksCloudProvider = cloudprovider.New(
			op.InstanceTypesProvider,
			op.VMInstanceProvider,
			op.AKSMachineProvider,
			op.EventRecorder,
			op.GetClient(),
			op.ImageProvider,
			op.InstanceTypeStore,
		)
		lo.Must0(op.AddHealthzCheck("cloud-provider", aksCloudProvider.LivenessProbe))
		hubCloudProvider.Register(aksCloudProvider, schema.GroupKind{
			Group: apis.Group,
			Kind:  "AKSNodeClass",
		}, "azure")
	}

	clusterCA := lo.Must(utilsk8s.RetrieveClusterCA(op.GetConfig()))

	// nebius cloud provider...
	{
		err := nebius.Register(
			ctx,
			hubCloudProvider,
			flexoptions.MustNewNebiusSDK(ctx),
			op.GetClient(),
			clusterCA,
		)
		lo.Must0(err, "registering nebius cloud provider")
	}

	// kaito
	{
		err := kaito.Register(
			ctx,
			hubCloudProvider,
			clusterCA,
		)
		lo.Must0(err, "registering kaito cloud provider")
	}

	overlayUndecoratedCloudProvider := metrics.Decorate(hubCloudProvider)
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
