package lambda_test

import (
	"github.com/aws/aws-sdk-go/aws/credentials"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"fmt"

	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	. "github.com/solo-io/gloo/pkg/function-discovery/updater/lambda"
	awsplugin "github.com/solo-io/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("GetLambdaFuncs", func() {
	Context("happy path", func() {
		It("adds all available lambdas to the upstream", func() {
			accessKey, secretKey, err := idAndKey(os.Getenv("USE_ENV_AWS_CREDENTIALS") == "1", "", "", "")
			helpers.Must(err)
			region := "us-east-1"
			secrets := secretwatcher.SecretMap{
				"my-aws-creds": &dependencies.Secret{Ref: "ssl-secret-ref", Data: map[string]string{
					awsplugin.AwsAccessKey: accessKey,
					awsplugin.AwsSecretKey: secretKey,
				}},
			}
			us := &v1.Upstream{
				Name: "something",
				Type: awsplugin.UpstreamTypeAws,
				Spec: awsplugin.EncodeUpstreamSpec(awsplugin.UpstreamSpec{
					Region:    region,
					SecretRef: "my-aws-creds",
				}),
			}

			lambdas, err := getLambdas(accessKey, secretKey, region)
			Expect(err).NotTo(HaveOccurred())

			funcs, err := GetFuncs(us, secrets)
			Expect(err).NotTo(HaveOccurred())
			Expect(funcs).To(HaveLen(len(lambdas.Functions)))
			for i, f := range lambdas.Functions {
				version := aws.StringValue(f.Version)
				if version == "$LATEST" {
					version = ""
				}
				expectedFn := &v1.Function{
					Name: aws.StringValue(f.FunctionName) + ":" + aws.StringValue(f.Version),
					Spec: awsplugin.EncodeFunctionSpec(awsplugin.FunctionSpec{
						FunctionName: aws.StringValue(f.FunctionName),
						Qualifier:    version,
					}),
				}
				Expect(funcs[i]).To(Equal(expectedFn))
			}
		})
	})
})

func idAndKey(useEnv bool, keyId, secretKey, filename string) (string, string, error) {
	if keyId != "" || secretKey != "" {
		if keyId != "" && secretKey != "" {
			return keyId, secretKey, nil
		}
		return "", "", fmt.Errorf("both access-key-id and secret-access-key must be provided")
	}

	var creds *credentials.Credentials
	if useEnv {
		creds = credentials.NewEnvCredentials()
	} else {
		//TODO: add a flag for profile
		creds = credentials.NewSharedCredentials(filename, "")
	}
	vals, err := creds.Get()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to retrieve aws credentials")
	}
	return vals.AccessKeyID, vals.SecretAccessKey, nil
}

func getLambdas(accessKey, secretKey, region string) (*lambda.ListFunctionsOutput, error) {
	sess, err := session.NewSession(aws.NewConfig().
		WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, "")))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create AWS session")
	}
	svc := lambda.New(sess, &aws.Config{Region: aws.String(region)})
	options := &lambda.ListFunctionsInput{FunctionVersion: aws.String("ALL")}
	results, err := svc.ListFunctions(options)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get list of functions from AWS")
	}
	return results, nil
}
