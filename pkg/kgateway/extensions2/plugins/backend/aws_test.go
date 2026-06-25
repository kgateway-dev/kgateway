package backend

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestProcessAwsUsesDnsClusterWithSingleEndpointAggregation(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{Name: "test-cluster"}

	err := processAws(&AwsIr{
		lambdaIr: &LambdaIr{
			lambdaFilters:  &lambdaFilters{},
			lambdaEndpoint: &lambdaEndpointConfig{hostname: "lambda.us-east-1.amazonaws.com", port: 443},
		},
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
		Type:       kgateway.AwsAuthTypeAssumeRole,
		AssumeRole: &kgateway.AwsAssumeRole{RoleArn: "arn:aws:iam::311275790335:role/project-invoke-role"},
	}

	signing, err := configureAWSAuth(auth, nil, "us-east-1")
	require.NoError(t, err)

	assumeRole := signing.GetCredentialProvider().GetAssumeRoleCredentialProvider()
	require.NotNil(t, assumeRole, "assume role auth should set the assume role credential provider")
	assert.Equal(t, "arn:aws:iam::311275790335:role/project-invoke-role", assumeRole.GetRoleArn())
	// base credentials must be left unset so Envoy falls back to the default provider chain (IRSA).
	assert.Nil(t, assumeRole.GetCredentialProvider(), "base credential provider should be unset to use the gateway's ambient credentials")
}

func TestBuildLambdaARNUsesPreferredNestedAccountID(t *testing.T) {
	backend := &kgateway.AwsBackend{
		AccountId: "111111111111",
		Lambda: &kgateway.AwsLambda{
			AccountId:    "222222222222",
			FunctionName: "hello-function",
			Qualifier:    "live",
		},
	}

	arn, err := buildLambdaARN(backend, "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:lambda:us-east-1:222222222222:function:hello-function:live", arn)
}

func TestBuildLambdaARNFallsBackToDeprecatedBackendAccountID(t *testing.T) {
	backend := &kgateway.AwsBackend{
		AccountId: "111111111111",
		Lambda: &kgateway.AwsLambda{
			FunctionName: "hello-function",
			Qualifier:    "live",
		},
	}

	arn, err := buildLambdaARN(backend, "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:lambda:us-east-1:111111111111:function:hello-function:live", arn)
}

func TestBuildTranslateFuncFailsClosedForLambdaEndpointWithoutPort(t *testing.T) {
	translate := buildTranslateFunc(nil, true)

	backendIR := translate(krt.TestingDummyContext{}, newLambdaBackend("lambda-backend", "https://lambda.us-east-1.amazonaws.com"))

	require.NotEmpty(t, backendIR.errors)
	assert.ErrorContains(t, backendIR.errors[0], "failed to parse port")
	assert.Nil(t, backendIR.awsIr, "translate() should not build AWS IR for an invalid lambda endpoint")
}

func TestBackendIrEqualsDetectsLambdaErrorOnlyChanges(t *testing.T) {
	backend := newLambdaBackend("example-aws-backend", "https://lambda.us-east-1.amazonaws.com:443")
	backend.ObjectMeta = metav1.ObjectMeta{
		Name:      "example-aws-backend",
		Namespace: "kgateway-base",
	}
	backend.Spec.Aws.Auth = &kgateway.AwsAuth{
		Type: kgateway.AwsAuthTypeSecret,
		SecretRef: &corev1.LocalObjectReference{
			Name: "lambda-secret",
		},
	}

	missingSecretIR := buildTranslateFunc(newSecretIndexForTest(t), true)(krt.TestingDummyContext{}, backend)
	invalidSecretIR := buildTranslateFunc(newSecretIndexForTest(t, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "lambda-secret",
			Namespace:       "kgateway-base",
			ResourceVersion: "1",
		},
		Data: map[string][]byte{
			"token": []byte("sk-test-secret"),
		},
	}), true)(krt.TestingDummyContext{}, backend)

	require.NotEmpty(t, missingSecretIR.errors)
	require.NotEmpty(t, invalidSecretIR.errors)
	assert.Nil(t, missingSecretIR.awsIr)
	assert.Nil(t, invalidSecretIR.awsIr)
	assert.False(t, missingSecretIR.Equals(invalidSecretIR), "different translation errors must invalidate KRT equality")
	assert.False(t, invalidSecretIR.Equals(missingSecretIR), "backend IR equality should remain symmetric")
}

func newLambdaBackend(name, endpointURL string) *kgateway.Backend {
	return &kgateway.Backend{
		Spec: kgateway.BackendSpec{
			Aws: &kgateway.AwsBackend{
				Region:    "us-east-1",
				AccountId: "111111111111",
				Lambda: &kgateway.AwsLambda{
					FunctionName: "hello-function",
					Qualifier:    "live",
					EndpointURL:  &endpointURL,
				},
			},
		},
	}
}
