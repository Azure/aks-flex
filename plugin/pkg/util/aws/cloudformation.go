package aws

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

func Deploy(ctx context.Context, awscfg aws.Config, stackName string, templateb []byte, parameters []types.Parameter) (map[string]string, error) {
	client := cloudformation.NewFromConfig(awscfg)

	_, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	switch {
	case isErrorCode(err, "ValidationError"):
		err = create(ctx, client, stackName, templateb, parameters)
	case err == nil:
		err = update(ctx, client, stackName, templateb, parameters)
	}
	if err != nil {
		return nil, err
	}

	return GetStack(ctx, awscfg, stackName)
}

func GetStack(ctx context.Context, awscfg aws.Config, stackName string) (map[string]string, error) {
	client := cloudformation.NewFromConfig(awscfg)

	stacks, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		return nil, err
	}

	outputs := map[string]string{}
	for _, o := range stacks.Stacks[0].Outputs {
		outputs[aws.ToString(o.OutputKey)] = aws.ToString(o.OutputValue)
	}

	return outputs, nil
}

func create(ctx context.Context, client *cloudformation.Client, stackName string, templateb []byte, parameters []types.Parameter) error {
	_, err := client.CreateStack(ctx, &cloudformation.CreateStackInput{
		StackName:    &stackName,
		TemplateBody: aws.String(string(templateb)),
		Parameters:   parameters,
	})
	if err != nil {
		return err
	}

	return cloudformation.NewStackCreateCompleteWaiter(client).Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	}, 30*time.Minute)
}

func update(ctx context.Context, client *cloudformation.Client, stackName string, templateb []byte, parameters []types.Parameter) error {
	_, err := client.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		StackName:    &stackName,
		TemplateBody: aws.String(string(templateb)),
		Parameters:   parameters,
	})
	if isErrorMessage(err, "No updates are to be performed.") {
		return nil
	}
	if err != nil {
		return err
	}

	return cloudformation.NewStackUpdateCompleteWaiter(client).Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	}, 30*time.Minute)
}

func Delete(ctx context.Context, awscfg aws.Config, stackName string) error {
	client := cloudformation.NewFromConfig(awscfg)

	if _, err := client.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: &stackName,
	}); err != nil {
		return err
	}

	return cloudformation.NewStackDeleteCompleteWaiter(client).Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	}, 30*time.Minute)
}
