package options

type Secret struct {
	TlsSecret   TlsSecret
	AwsSecret   AwsSecret
	AzureSecret AzureSecret
}

type AwsSecret struct {
	AccessKey string
	SecretKey string
}

type AzureSecret struct {
	ApiKeys InputMapStringString
}

type TlsSecret struct {
	RootCaFilename     string
	PrivateKeyFilename string
	CertChainFilename  string
}
