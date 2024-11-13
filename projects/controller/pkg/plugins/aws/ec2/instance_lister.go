package ec2

import (
	"context"

	"github.com/rotisserie/eris"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
)

// Ec2InstanceLister is a simple interface for calling the AWS API.
// This allows us to easily mock the API in our tests.
type Ec2InstanceLister interface {
	ListForCredentials(ctx context.Context, cred *CredentialSpec, secrets v1.SecretList) ([]*ec2.Instance, error)
}

type ec2InstanceLister struct {
}

func NewEc2InstanceLister() *ec2InstanceLister {
	return &ec2InstanceLister{}
}

var _ Ec2InstanceLister = &ec2InstanceLister{}

func (c *ec2InstanceLister) ListForCredentials(ctx context.Context, cred *CredentialSpec, secrets v1.SecretList) ([]*ec2.Instance, error) {
	svc, err := GetEc2Client(cred, secrets)
	if err != nil {
		return nil, GetClientError(err)
	}
	return c.ListWithClient(ctx, svc)
}

func (c *ec2InstanceLister) ListWithClient(ctx context.Context, svc *ec2.EC2) ([]*ec2.Instance, error) {

	var results []*ec2.DescribeInstancesOutput
	// pass a filter to only get running instances.
	input := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{Name: aws.String("instance-state-name"),
				Values: []*string{aws.String("running")}},
		},
	}
	err := svc.DescribeInstancesPagesWithContext(ctx, input, func(r *ec2.DescribeInstancesOutput, more bool) bool {
		results = append(results, r)
		return true
	})
	if err != nil {
		return nil, DescribeInstancesError(err)
	}

	var result []*ec2.Instance
	for _, dio := range results {
		result = append(result, GetInstancesFromDescription(dio)...)
	}

	contextutils.LoggerFrom(ctx).Debugw("ec2Upstream result", zap.Any("value", result))
	return result, nil
}

var (
	GetClientError = func(err error) error {
		return eris.Wrapf(err, "unable to get aws client")
	}

	DescribeInstancesError = func(err error) error {
		return eris.Wrapf(err, "unable to describe instances")
	}
)
