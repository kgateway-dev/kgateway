package awsutils

import "os"

const (
	AccessKeyEnv       = "AWS_ACCESS_KEY_ID"
	SecretAccessKeyEnv = "AWS_SECRET_ACCESS_KEY" //nolint:gosec // G101: Standard AWS SDK environment variable name, not a credential
	SessionTokenEnv    = "AWS_SESSION_TOKEN"     //nolint:gosec // G101: Standard AWS SDK environment variable name, not a credential
)

// SetAccessKeyEnv set the environment variable referred to by AccessKeyEnv to the passed string
func SetAccessKeyEnv(accessKey string) error {
	return os.Setenv(AccessKeyEnv, accessKey)
}

// SetSecretAccessKeyEnv set the environment variable referred to by SecretAccessKeyEnv to the passed string
func SetSecretAccessKeyEnv(secretAccessKey string) error {
	return os.Setenv(SecretAccessKeyEnv, secretAccessKey)
}

// SetSessionTokenEnv set the environment variable referred to by SessionTokenEnv to the passed string
func SetSessionTokenEnv(sessionToken string) error {
	return os.Setenv(SessionTokenEnv, sessionToken)
}
