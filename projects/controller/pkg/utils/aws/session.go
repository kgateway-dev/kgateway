package aws

import (
	"unicode/utf8"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	// expected map identifiers for secrets
	awsAccessKey    = "access_key"
	awsSecretKey    = "secret_key"
	awsSessionToken = "session_token"
)

func GetAwsSession(secretRef *core.ResourceRef, secrets v1.SecretList, config *aws.Config) (*session.Session, error) {

	if config == nil {
		config = aws.NewConfig()
	}

	if secretRef == nil {
		// no secret ref, return the default session
		return session.NewSession(config)
	}
	awsSecrets, err := secrets.Find(secretRef.GetNamespace(), secretRef.GetName())
	if err != nil {
		return nil, errors.Wrapf(err, "secrets not found for secret ref %s.%s", secretRef.GetName(), secretRef.GetNamespace())
	}

	awsSecret, ok := awsSecrets.GetKind().(*v1.Secret_Aws)
	if !ok {
		return nil, errors.Errorf("provided secret is not an aws secret")
	}
	accessKey := awsSecret.Aws.GetAccessKey()
	if accessKey != "" && !utf8.Valid([]byte(accessKey)) {
		return nil, errors.Errorf("%s not a valid string", awsAccessKey)
	}
	secretKey := awsSecret.Aws.GetSecretKey()
	if secretKey != "" && !utf8.Valid([]byte(secretKey)) {
		return nil, errors.Errorf("%s not a valid string", awsSecretKey)
	}
	sessionKey := awsSecret.Aws.GetSessionToken()
	if secretKey != "" && !utf8.Valid([]byte(secretKey)) {
		return nil, errors.Errorf("%s not a valid string", awsSessionToken)
	}

	sess, err := session.NewSession(config.
		WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, sessionKey)))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create AWS session")
	}
	return sess, nil
}
