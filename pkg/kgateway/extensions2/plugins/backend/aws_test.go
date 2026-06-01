package backend

import (
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func ptrTo[T any](v T) *T { return &v }

func TestProcessAwsUsesDnsClusterWithSingleEndpointAggregation(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{Name: "test-cluster"}

	err := processAws(&AwsIr{
		lambdaFilters:  &lambdaFilters{},
		lambdaEndpoint: &lambdaEndpointConfig{hostname: "lambda.us-east-1.amazonaws.com", port: 443},
	}, cluster)
	require.NoError(t, err)

	clusterType := cluster.GetClusterType()
	require.NotNil(t, clusterType, "expected custom dns cluster type")
	require.Equal(t, dnsClusterExtensionName, clusterType.GetName())

	var dnsCluster envoydnsv3.DnsCluster
	err = anypb.UnmarshalTo(clusterType.GetTypedConfig(), &dnsCluster, proto.UnmarshalOptions{})
	require.NoError(t, err)
	assert.True(t, dnsCluster.GetAllAddressesInSingleEndpoint(), "aws backends should aggregate resolved addresses into a single endpoint")
}

func TestConfigureAWSAuthDefaultProviderChain(t *testing.T) {
	signing, err := configureAWSAuth(nil, nil, "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, lambdaServiceName, signing.GetServiceName())
	assert.Equal(t, "us-east-1", signing.GetRegion())
	assert.Nil(t, signing.GetCredentialProvider(), "default provider chain should not set an explicit credential provider")
}

func TestConfigureAWSAuthSecret(t *testing.T) {
	secret := &ir.Secret{Data: map[string][]byte{
		wellknown.AccessKey:    []byte("access"),
		wellknown.SecretKey:    []byte("secret"),
		wellknown.SessionToken: []byte("session"),
	}}
	auth := &kgateway.AwsAuth{
		Type:      kgateway.AwsAuthTypeSecret,
		SecretRef: &corev1.LocalObjectReference{Name: "aws-creds"},
	}

	signing, err := configureAWSAuth(auth, secret, "us-east-1")
	require.NoError(t, err)
	inline := signing.GetCredentialProvider().GetInlineCredential()
	require.NotNil(t, inline)
	assert.Equal(t, "access", inline.GetAccessKeyId())
	assert.Equal(t, "secret", inline.GetSecretAccessKey())
	assert.Equal(t, "session", inline.GetSessionToken())
}

func TestConfigureAWSAuthSecretMissing(t *testing.T) {
	auth := &kgateway.AwsAuth{Type: kgateway.AwsAuthTypeSecret, SecretRef: &corev1.LocalObjectReference{Name: "aws-creds"}}
	_, err := configureAWSAuth(auth, nil, "us-east-1")
	require.Error(t, err)
}

func TestConfigureAWSAuthAssumeRole(t *testing.T) {
	auth := &kgateway.AwsAuth{
		Type: kgateway.AwsAuthTypeAssumeRole,
		AssumeRole: &kgateway.AwsAssumeRole{
			RoleArn:         "arn:aws:iam::311275790335:role/project-invoke-role",
			SessionName:     ptrTo("kgateway-session"),
			ExternalId:      ptrTo("ext-123"),
			SessionDuration: &metav1.Duration{Duration: 30 * time.Minute},
		},
	}

	signing, err := configureAWSAuth(auth, nil, "us-east-1")
	require.NoError(t, err)

	assumeRole := signing.GetCredentialProvider().GetAssumeRoleCredentialProvider()
	require.NotNil(t, assumeRole, "assume role auth should set the assume role credential provider")
	assert.Equal(t, "arn:aws:iam::311275790335:role/project-invoke-role", assumeRole.GetRoleArn())
	assert.Equal(t, "kgateway-session", assumeRole.GetRoleSessionName())
	assert.Equal(t, "ext-123", assumeRole.GetExternalId())
	assert.Equal(t, 30*time.Minute, assumeRole.GetSessionDuration().AsDuration())
	// base credentials must be left unset so Envoy falls back to the default provider chain (IRSA).
	assert.Nil(t, assumeRole.GetCredentialProvider(), "base credential provider should be unset to use the gateway's ambient credentials")
}

func TestConfigureAWSAuthAssumeRoleMinimal(t *testing.T) {
	auth := &kgateway.AwsAuth{
		Type:       kgateway.AwsAuthTypeAssumeRole,
		AssumeRole: &kgateway.AwsAssumeRole{RoleArn: "arn:aws:iam::311275790335:role/project-invoke-role"},
	}

	signing, err := configureAWSAuth(auth, nil, "us-east-1")
	require.NoError(t, err)
	assumeRole := signing.GetCredentialProvider().GetAssumeRoleCredentialProvider()
	require.NotNil(t, assumeRole)
	assert.Equal(t, "arn:aws:iam::311275790335:role/project-invoke-role", assumeRole.GetRoleArn())
	assert.Empty(t, assumeRole.GetRoleSessionName())
	assert.Empty(t, assumeRole.GetExternalId())
	assert.Nil(t, assumeRole.GetSessionDuration())
}
