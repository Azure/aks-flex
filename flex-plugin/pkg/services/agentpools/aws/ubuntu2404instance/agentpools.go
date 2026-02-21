package ubuntu2404instance

import (
	"context"
	_ "embed"
	"encoding/base64"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/helper"
	agentpools "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/userdata/ubuntu"
	"github.com/Azure/aks-flex/flex-plugin/pkg/topology"
	utilaws "github.com/Azure/aks-flex/flex-plugin/pkg/util/aws"
	"github.com/Azure/aks-flex/flex-plugin/pkg/util/ssh"
)

var _ api.Object = (*AgentPool)(nil)

//go:embed assets/aws.json
var awsJSON []byte

type agentpoolsServer struct {
	agentpools.UnimplementedAgentPoolsServer
	storage db.RODB
}

func NewAgentPoolsServer(storage db.RODB) (agentpools.AgentPoolsServer, error) {
	return &agentpoolsServer{
		storage: storage,
	}, nil
}

func (srv *agentpoolsServer) CreateOrUpdate(ctx context.Context, req *api.CreateOrUpdateRequest) (*api.CreateOrUpdateResponse, error) {
	const instanceType = "m8i.large" // FIXME: FIXME: make it configurable and align with the value in aws.json

	ap, err := helper.AnyTo[*AgentPool](req.GetItem())
	if err != nil {
		return nil, err
	}

	awscfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(ap.GetSpec().GetRegion()))
	if err != nil {
		return nil, err
	}

	sshKey, err := ssh.PublicKey()
	if err != nil {
		return nil, err
	}

	kubeadmConfig := ap.GetSpec().GetKubeadm()
	kubeadmConfig.AddNodeLabels(map[string]string{
		topology.NodeLabelKeyCloud:  "aws",
		topology.NodeLabelKeyRegion: strings.ToLower(ap.GetSpec().GetRegion()),
		// TODO: zone
		topology.NodeLabelKeyInstanceType: strings.ToLower(instanceType),
	})

	userData, err := ubuntu.UserData(kubeadmConfig)
	if err != nil {
		return nil, err
	}

	userDataContent, err := userData.Gzip()
	if err != nil {
		return nil, err
	}

	outputs, err := utilaws.Deploy(ctx, awscfg, "agentpool-"+ap.GetMetadata().GetId(), awsJSON, []types.Parameter{
		{
			ParameterKey:   aws.String("PublicKeyMaterial"),
			ParameterValue: aws.String(string(sshKey)),
		},
		{
			ParameterKey:   aws.String("SecurityGroup"),
			ParameterValue: aws.String(ap.GetSpec().GetSecurityGroup()),
		},
		{
			ParameterKey:   aws.String("Subnet"),
			ParameterValue: aws.String(ap.GetSpec().GetSubnet()),
		},
		{
			ParameterKey:   aws.String("UserData"),
			ParameterValue: aws.String(base64.StdEncoding.EncodeToString(userDataContent)),
		},
	})
	if err != nil {
		return nil, err
	}

	ap.SetStatus(AgentPoolStatus_builder{
		Instance: to.Ptr(outputs["Instance"]),
	}.Build())

	item, err := anypb.New(ap)
	if err != nil {
		return nil, err
	}

	return api.CreateOrUpdateResponse_builder{
		Item: item,
	}.Build(), nil
}

func (srv *agentpoolsServer) Delete(ctx context.Context, req *api.DeleteRequest) (*api.DeleteResponse, error) {
	obj, ok := srv.storage.Get(req.GetId())
	if !ok {
		return api.DeleteResponse_builder{}.Build(), nil
	}

	ap, err := helper.To[*AgentPool](obj)
	if err != nil {
		return nil, err
	}

	awscfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(ap.GetSpec().GetRegion()))
	if err != nil {
		return nil, err
	}

	if err := utilaws.Delete(ctx, awscfg, "agentpool-"+ap.GetMetadata().GetId()); err != nil {
		return nil, err
	}

	return api.DeleteResponse_builder{}.Build(), nil
}
