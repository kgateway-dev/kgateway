package backend

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
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
