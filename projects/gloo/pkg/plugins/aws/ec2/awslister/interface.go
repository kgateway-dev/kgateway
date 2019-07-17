package awslister

import (
	"context"

	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// Ec2InstanceLister is a simple interface for calling the AWS API.
// This allows us to easily mock the API in our tests.
type Ec2InstanceLister interface {
	ListForCredentials(ctx context.Context, awsRegion string, secretRef core.ResourceRef, secrets v1.SecretList) ([]*ec2.Instance, error)
}
