package agentpools

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"google.golang.org/protobuf/proto"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
	"github.com/Azure/aks-flex/flex-plugin/api"
	awsap "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/aws/ubuntu2404instance"
)

func newAWSAgentPool(ctx context.Context) proto.Message {
	return awsap.AgentPool_builder{
		Metadata: api.NewMetadata[*awsap.AgentPool]("aws-default"),
		Spec: awsap.AgentPoolSpec_builder{
			Region:        to.Ptr("us-east-1"),
			Subnet:        to.Ptr(configcmd.OrPlaceholder("")),
			SecurityGroup: to.Ptr(configcmd.OrPlaceholder("")),
			Kubeadm:       configcmd.DefaultKubeadmConfig(ctx),
		}.Build(),
	}.Build()
}
