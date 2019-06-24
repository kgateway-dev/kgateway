package main

import (
	"context"
	"fmt"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	aws2 "github.com/solo-io/gloo/projects/gloo/pkg/utils/aws"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// TEMP - TODO REMOVE
func main() {
	ctx := context.Background()
	err := run(core.ResourceRef{}, nil)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("failure while running", zap.Error(err))
	}
}

// TODO
func run(secretRef core.ResourceRef, secrets v1.SecretList) error {
	region := "us-east-1"
	sess, err := aws2.GetAwsSession(secretRef, secrets, &aws.Config{Region: &region})
	if err != nil {
		return err
	}
	svc := ec2.New(sess)
	tag := tagFilterName("Name")
	val := tagFilterValue("openshift-master")
	input := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   tag,
				Values: val,
			},
		},
	}
	result, err := svc.DescribeInstances(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
	}

	fmt.Println(result)
	return nil
}

func tagFilterName(tagName string) *string {
	str := fmt.Sprintf("tag:%v", tagName)
	return &str
}

func tagFilterValue(tagValue string) []*string {
	return []*string{&tagValue}
}

func getLocalAwsSession(region string) (*session.Session, error) {
	return session.NewSession(&aws.Config{
		Region: &region,
	})
}
