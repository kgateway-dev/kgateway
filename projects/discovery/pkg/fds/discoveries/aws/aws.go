package aws

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/service/lambda"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	glooaws "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	awsutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/aws"
)

const (
	AWS_WEB_IDENTITY_TOKEN_FILE = "AWS_WEB_IDENTITY_TOKEN_FILE"
	AWS_ROLE_ARN                = "AWS_ROLE_ARN"
	AWS_REGION                  = "AWS_REGION"
)

func NewFunctionDiscoveryFactory() fds.FunctionDiscoveryFactory {
	return &AWSLambdaFunctionDiscoveryFactory{
		PollingTime: time.Second,
	}
}

// AWSLambdaFunctionDiscoveryFactory represents a factory for AWS Lambda function discovery.
type AWSLambdaFunctionDiscoveryFactory struct {
	PollingTime time.Duration
}

func (f *AWSLambdaFunctionDiscoveryFactory) NewFunctionDiscovery(u *v1.Upstream, _ fds.AdditionalClients) fds.UpstreamFunctionDiscovery {
	return &AWSLambdaFunctionDiscovery{
		timeToWait: f.PollingTime,
		upstream:   u,
	}
}

// AWSLambdaFunctionDiscovery is a discovery that polls AWS Lambda for function discovery.
type AWSLambdaFunctionDiscovery struct {
	timeToWait time.Duration
	upstream   *v1.Upstream
}

func (f *AWSLambdaFunctionDiscovery) IsFunctional() bool {
	_, ok := f.upstream.GetUpstreamType().(*v1.Upstream_Aws)
	return ok
}

func (f *AWSLambdaFunctionDiscovery) DetectType(ctx context.Context, url *url.URL) (*plugins.ServiceSpec, error) {
	return nil, nil
}

// DetectFunctions perhaps the in param for the upstream should be a function? in func() *v1.Upstream
// TODO: how to handle changes in secret or upstream (like the upstream ref)?
func (f *AWSLambdaFunctionDiscovery) DetectFunctions(ctx context.Context, url *url.URL, dependencies func() fds.Dependencies, updatecb func(fds.UpstreamMutator) error) error {
	// TODO: get backoff values from config?
	err := contextutils.NewExponentialBackoff(contextutils.ExponentialBackoff{}).Backoff(ctx, func(ctx context.Context) error {
		newFunctions, err := f.DetectFunctionsOnce(ctx, dependencies().Secrets)

		if err != nil {
			return err
		}

		// sort for idempotency
		sort.Slice(newFunctions, func(i, j int) bool {
			return newFunctions[i].GetLogicalName() < newFunctions[j].GetLogicalName()
		})

		// TODO(yuval-k): only update functions if newFunctions != oldFunctions
		// no need to constantly write to storage

		err = updatecb(func(out *v1.Upstream) error {
			// TODO(yuval-k): this should never happen. but it did. add logs?
			if out == nil {
				return errors.New("nil upstream")
			}

			if out.GetUpstreamType() == nil {
				return errors.New("nil upstream type")
			}

			awsSpec, ok := out.GetUpstreamType().(*v1.Upstream_Aws)
			if !ok {
				return errors.New("not aws upstream")
			}
			awsSpec.Aws.LambdaFunctions = newFunctions
			return nil
		})

		if err != nil {
			return errors.Wrap(err, "unable to update upstream")
		}
		return nil

	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// only log other errors as we would like to continue forever.
		contextutils.LoggerFrom(ctx).Warnf("Unable to perform aws function discovery for upstream %s in namespace %s, error: ",
			f.upstream.GetMetadata().GetName(),
			f.upstream.GetMetadata().GetNamespace(),
			err.Error(),
		)
	}

	// sleep so we are not hogging
	if err := contextutils.Sleep(ctx, f.timeToWait); err != nil {
		return err
	}
	return nil
}

func (f *AWSLambdaFunctionDiscovery) DetectFunctionsOnce(ctx context.Context, secrets v1.SecretList) ([]*glooaws.LambdaFunctionSpec, error) {
	in := f.upstream
	awsSpec, ok := in.GetUpstreamType().(*v1.Upstream_Aws)

	if !ok {
		return nil, errors.New("not a lambda upstream spec")
	}
	lambdaSpec := awsSpec.Aws
	awsRegion := lambdaSpec.GetRegion()
	if awsRegion == "" {
		awsRegion = os.Getenv(AWS_REGION)
	}
	sess, err := awsutils.GetAwsSession(lambdaSpec.GetSecretRef(), secrets, &aws.Config{Region: aws.String(lambdaSpec.GetRegion())})
	if err != nil {
		return nil, errors.Wrap(err, "unable to create AWS session")
	}

	var svc *lambda.Lambda

	tokenPath := os.Getenv(AWS_WEB_IDENTITY_TOKEN_FILE)
	roleArn := os.Getenv(AWS_ROLE_ARN)
	// If aws web token, and role arn are available, authenticate lambda service using mounted credentials.
	// See: https://aws.amazon.com/blogs/opensource/introducing-fine-grained-iam-roles-service-accounts/
	if tokenPath != "" && roleArn != "" {
		if awsSpec.Aws.GetRoleArn() != "" {
			roleArn = awsSpec.Aws.GetRoleArn()
		}
		contextutils.LoggerFrom(ctx).Debugf("Discovering lambda functions using assumed role [%s]", roleArn)
		webProvider := stscreds.NewWebIdentityCredentials(sess, roleArn, "", tokenPath)
		svc = lambda.New(sess, aws.NewConfig().WithCredentials(webProvider))
	} else {
		svc = lambda.New(sess)
	}

	var newfunctions []*glooaws.LambdaFunctionSpec

	options := &lambda.ListFunctionsInput{FunctionVersion: aws.String("ALL")}
	err = svc.ListFunctionsPagesWithContext(ctx, options, func(results *lambda.ListFunctionsOutput, _ bool) bool {

		for _, f := range results.Functions {
			version := aws.StringValue(f.Version)
			name := aws.StringValue(f.FunctionName)

			logicalName := fmt.Sprintf("%s:%s", name, version)
			if version == "$LATEST" {
				logicalName = name
			}

			newfunctions = append(newfunctions, &glooaws.LambdaFunctionSpec{
				LambdaFunctionName: name,
				Qualifier:          version,
				LogicalName:        logicalName,
			})
		}

		return true
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get list of functions from AWS")
	}

	return newfunctions, nil
}
