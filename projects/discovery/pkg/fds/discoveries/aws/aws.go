package aws

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins"
	glooaws "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws"
	awsutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/aws"
	"github.com/solo-io/go-utils/contextutils"
)

type AWSLambdaFunctionDiscoveryFactory struct {
	PollingTime time.Duration
}

func (f *AWSLambdaFunctionDiscoveryFactory) NewFunctionDiscovery(u *v1.Upstream) fds.UpstreamFunctionDiscovery {
	return &AWSLambdaFunctionDiscovery{
		timetowait: f.PollingTime,
		upstream:   u,
	}
}

type AWSLambdaFunctionDiscovery struct {
	timetowait time.Duration
	upstream   *v1.Upstream
}

func (f *AWSLambdaFunctionDiscovery) IsFunctional() bool {
	_, ok := f.upstream.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Aws)
	return ok
}

func (f *AWSLambdaFunctionDiscovery) DetectType(ctx context.Context, url *url.URL) (*plugins.ServiceSpec, error) {
	return nil, nil
}

// TODO: how to handle changes in secret or upstream (like the upstream ref)?
// perhaps the in param for the upstream should be a function? in func() *v1.Upstream
func (f *AWSLambdaFunctionDiscovery) DetectFunctions(ctx context.Context, url *url.URL, dependencies func() fds.Dependencies, updatecb func(fds.UpstreamMutator) error) error {
	for {
		// TODO: get backoff values from config?
		err := contextutils.NewExponentioalBackoff(contextutils.ExponentioalBackoff{}).Backoff(ctx, func(ctx context.Context) error {
			newfunctions, err := f.DetectFunctionsOnce(ctx, dependencies().Secrets)

			if err != nil {
				return err
			}

			// sort for idempotency
			sort.Slice(newfunctions, func(i, j int) bool {
				return newfunctions[i].LogicalName < newfunctions[j].LogicalName
			})

			// TODO(yuval-k): only update functions if newfunctions != oldfunctions
			// no need to constantly write to storage

			err = updatecb(func(out *v1.Upstream) error {
				// TODO(yuval-k): this should never happen. but it did. add logs?
				if out == nil {
					return errors.New("nil upstream")
				}
				if out.UpstreamSpec == nil {
					return errors.New("nil upstream spec")
				}

				if out.UpstreamSpec.UpstreamType == nil {
					return errors.New("nil upstream type")
				}

				awsspec, ok := out.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Aws)
				if !ok {
					return errors.New("not aws upstream")
				}
				awsspec.Aws.LambdaFunctions = newfunctions
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
			// ignore other erros as we would like to continue forever.
		}

		// sleep so we are not hogging
		if err := contextutils.Sleep(ctx, f.timetowait); err != nil {
			return err
		}
	}
}

func (f *AWSLambdaFunctionDiscovery) DetectFunctionsOnce(ctx context.Context, secrets v1.SecretList) ([]*glooaws.LambdaFunctionSpec, error) {
	in := f.upstream
	awsspec, ok := in.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Aws)

	if !ok {
		return nil, errors.New("not a lambda upstream spec")
	}
	lambdaSpec := awsspec.Aws
	sess, err := awsutils.GetAwsSession(lambdaSpec.SecretRef, secrets, &aws.Config{Region: aws.String(lambdaSpec.Region)})
	if err != nil {
		return nil, errors.Wrap(err, "unable to create AWS session")
	}
	svc := lambda.New(sess)

	var newfunctions []*glooaws.LambdaFunctionSpec

	options := &lambda.ListFunctionsInput{FunctionVersion: aws.String("ALL")}
	err = svc.ListFunctionsPagesWithContext(ctx, options, func(results *lambda.ListFunctionsOutput, _ bool) bool {

		for _, f := range results.Functions {
			version := aws.StringValue(f.Version)
			name := aws.StringValue(f.FunctionName)

			logicalname := fmt.Sprintf("%s:%s", name, version)
			if version == "$LATEST" {
				logicalname = name
			}

			newfunctions = append(newfunctions, &glooaws.LambdaFunctionSpec{
				LambdaFunctionName: name,
				Qualifier:          version,
				LogicalName:        logicalname,
			})
		}

		return true
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get list of functions from AWS")
	}

	return newfunctions, nil
}
