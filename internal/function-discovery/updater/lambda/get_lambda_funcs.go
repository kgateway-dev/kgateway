package lambda

import (
	"unicode/utf8"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	lambdaplugin "github.com/solo-io/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

const (
	// expected map identifiers for secrets
	awsAccessKey = "access_key"
	awsSecretKey = "secret_key"
)

func GetFuncs(us *v1.Upstream, secrets secretwatcher.SecretMap) ([]*v1.Function, error) {
	lambdaSpec, err := lambdaplugin.DecodeUpstreamSpec(us.Spec)
	if err != nil {
		return nil, errors.Wrap(err, "decoding lambda upstream spec")
	}
	awsSecrets, ok := secrets[lambdaSpec.SecretRef]
	if !ok {
		return nil, errors.Wrapf(err, "secrets not found for secret ref %v", lambdaSpec.SecretRef)
	}

	accessKey, ok := awsSecrets.Data[awsAccessKey]
	if !ok {
		return nil, errors.Errorf("key %v missing from provided secret", awsAccessKey)
	}
	if accessKey != "" && !utf8.Valid([]byte(accessKey)) {
		return nil, errors.Errorf("%s not a valid string", awsAccessKey)
	}
	secretKey, ok := awsSecrets.Data[awsSecretKey]
	if !ok {
		return nil, errors.Errorf("key %v missing from provided secret", awsSecretKey)
	}
	if secretKey != "" && !utf8.Valid([]byte(secretKey)) {
		return nil, errors.Errorf("%s not a valid string", awsSecretKey)
	}

	sess, err := session.NewSession(aws.NewConfig().
		WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, "")))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create AWS session")
	}
	svc := lambda.New(sess, &aws.Config{Region: aws.String(lambdaSpec.Region)})
	options := &lambda.ListFunctionsInput{FunctionVersion: aws.String("ALL")}
	results, err := svc.ListFunctions(options)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get list of functions from AWS")
	}
	return convertResultToFunctionSpec(results), nil
}

func GetSecretRef(us *v1.Upstream) (string, error) {
	lambdaSpec, err := lambdaplugin.DecodeUpstreamSpec(us.Spec)
	if err != nil {
		return "", errors.Wrap(err, "decoding lambda upstream spec")
	}
	return lambdaSpec.SecretRef, nil
}

func convertResultToFunctionSpec(results *lambda.ListFunctionsOutput) []*v1.Function {
	var funcs []*v1.Function
	for _, f := range results.Functions {
		version := aws.StringValue(f.Version)
		if version == "$LATEST" {
			version = ""
		}
		fn := &v1.Function{
			Name: aws.StringValue(f.FunctionName) + ":" + aws.StringValue(f.Version),
			Spec: lambdaplugin.EncodeFunctionSpec(lambdaplugin.FunctionSpec{
				FunctionName: aws.StringValue(f.FunctionName),
				Qualifier:    version,
			}),
		}
		funcs = append(funcs, fn)
	}
	return funcs
}
