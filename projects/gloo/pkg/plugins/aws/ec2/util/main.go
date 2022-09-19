package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awsec2 "github.com/aws/aws-sdk-go/service/ec2"
)

var accessKey = flag.String("accesskey", "", "access key id")
var secretKey = flag.String("secretkey", "", "secret key")
var region = flag.String("region", "", "secret key")
var debug = flag.Bool("debug", false, "debug")

func main() {
	flag.Parse()
	config := &aws.Config{Region: aws.String(*region)}
	if *debug {
		config = config.WithLogLevel(aws.LogDebugWithHTTPBody)
	}
	sess, err := session.NewSession(config.
		WithCredentials(credentials.NewStaticCredentials(*accessKey, *secretKey, "")))

	if err != nil {
		contextutils.LoggerFrom(nil).DPanic(err)
	}
	ec2api := awsec2.New(sess)

	e := ec2.NewEc2InstanceLister()

	instances, err := e.ListWithClient(context.Background(), ec2api)

	if err != nil {
		contextutils.LoggerFrom(nil).DPanic(err)
	}

	fmt.Println("found ", len(instances))

	for _, inst := range instances {
		for _, tag := range inst.Tags {
			if *tag.Key == "Name" {
				fmt.Printf("%v\n", *tag.Value)
			}
		}
	}
}
