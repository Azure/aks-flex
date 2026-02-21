package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func GetVpnConnection(ctx context.Context, awscfg aws.Config, id string) (*types.VpnConnection, error) {
	client := ec2.NewFromConfig(awscfg)

	output, err := client.DescribeVpnConnections(ctx, &ec2.DescribeVpnConnectionsInput{
		VpnConnectionIds: []string{id},
	})
	if err != nil {
		return nil, err
	}

	return &output.VpnConnections[0], nil
}
