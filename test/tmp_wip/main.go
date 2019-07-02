// TEMP - TODO REMOVE
// THIS WILL BE TURNED INTO A TEST
package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"
)

/*
WIP Questions:
What about uniqueness?
- can multiple upstreams point to the same ec2 targets?

Reporting activity?
- can gloo write to the upstream to say which ec2 instances it points to?
  - if so, how should those be identified? Would it be considered a leak to show the id of an instance that is not visible to some of the people who can view the upstream?
*/

func main() {
	ctx := context.Background()
	err := run(ctx)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("failure while running", zap.Error(err))
	}
}
func run(ctx context.Context) error {
	tmpUpstream := &glooec2.UpstreamSpec{
		Region:    "us-east-1",
		SecretRef: core.ResourceRef{},
		Filters: []*glooec2.Filter{{
			Spec: &glooec2.Filter_KvPair_{
				KvPair: &glooec2.Filter_KvPair{
					Key:   "Name",
					Value: "openshift-master",
				},
			},
		}},
	}
	sess, err := getLocalAwsSession(tmpUpstream.Region)
	result, err := ec2.ListEc2InstancesForCredentials(sess, tmpUpstream)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("failure while running", zap.Error(err))
	}
	fmt.Println(result)
	return nil
}

func getLocalAwsSession(region string) (*session.Session, error) {
	return session.NewSession(&aws.Config{
		Region: &region,
	})
}
