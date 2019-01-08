package options

type Secret struct {
	TlsSecret TlsSecret
	AwsSecret AwsSecret
}

type AwsSecret struct {
	AccessKey string
	SecretKey string
}

type TlsSecret struct {
	RootCaFilename     string
	PrivateKeyFilename string
	CertChainFilename  string
}

/*
// SOLO
type TlsSecret struct {
	CertChain            string
	PrivateKey           string
	RootCa               string
// ISTIO
RootCert  string
CertChain string
CaCert    string
CaKey     string
// Metadata contains the object metadata for this resource
Metadata             core.Metadata `protobuf:"bytes,7,opt,name=metadata" json:"metadata"`
*/
