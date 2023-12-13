package awsutils

import "os"

const (
	AccessKeyEnv       = "AWS_ACCESS_KEY_ID"
	SecretAccessKeyEnv = "AWS_SECRET_ACCESS_KEY"
	SessionTokenEnv    = "AWS_SESSION_TOKEN"
)

func SetAccessKeyEnv(accessKey string) error {
	return os.Setenv(AccessKeyEnv, accessKey)
}

func SetSecretAccessKeyEnv(secretAccessKey string) error {
	return os.Setenv(SecretAccessKeyEnv, secretAccessKey)
}

func SetSessionTokenEnv(sessionToken string) error {
	return os.Setenv(SessionTokenEnv, sessionToken)
}
