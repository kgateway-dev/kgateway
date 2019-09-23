package aws

import (
	"unicode/utf8"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	// expected map identifiers for secrets
	awsAccessKey = "access_key"
	awsSecretKey = "secret_key"
)

func GetAwsSession(secretRef *core.ResourceRef, secrets v1.SecretList, config *aws.Config) (*session.Session, error) {

	if config == nil {
		config = aws.NewConfig()
	}

	if secretRef == nil {
		// no secret ref, return the default session
		return session.NewSession(config)
	}
	awsSecrets, err := secrets.Find(secretRef.Namespace, secretRef.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "secrets not found for secret ref %v", secretRef)
	}

	awsSecret, ok := awsSecrets.Kind.(*v1.Secret_Aws)
	if !ok {
		return nil, errors.Errorf("provided secret is not an aws secret")
	}
	accessKey := awsSecret.Aws.AccessKey
	if accessKey != "" && !utf8.Valid([]byte(accessKey)) {
		return nil, errors.Errorf("%s not a valid string", awsAccessKey)
	}
	secretKey := awsSecret.Aws.SecretKey
	if secretKey != "" && !utf8.Valid([]byte(secretKey)) {
		return nil, errors.Errorf("%s not a valid string", awsSecretKey)
	}

	sess, err := session.NewSession(config.
		WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, "")))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create AWS session")
	}
	return sess, nil
}
