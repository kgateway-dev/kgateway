package backendconfigpolicy

import (
	"context"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
	"github.com/kgateway-dev/kgateway/v2/pkg/xds/bootstrap"
)

// validateXDS performs xDS validation checks on the BCP IR definition. This acts as a
// safety net to catch bugs in the IR construction logic and prevents invalid configuration
// from being applied when STRICT mode is enabled.
func validateXDS(
	ctx context.Context,
	policyIR *BackendConfigPolicyIR,
	v validator.Validator,
	mode apisettings.ValidationMode,
) error {
	if mode != apisettings.ValidationStrict || v == nil {
		return nil
	}

	// default to STATIC cluster for validation, but switch to STRICT_DNS when
	// useHostnameForHashing is enabled to avoid false positives. The load balancer
	// logic requires STRICT_DNS clusters for hostname-based hashing to work properly.
	discoveryType := envoyclusterv3.Cluster_STATIC
	if policyIR.loadBalancerConfig != nil && policyIR.loadBalancerConfig.useHostnameForHashing {
		discoveryType = envoyclusterv3.Cluster_STRICT_DNS
	}

	testCluster := &envoyclusterv3.Cluster{
		Name: "test-cluster-for-validation",
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
			Type: discoveryType,
		},
		ConnectTimeout: durationpb.New(5 * time.Second),
	}
	dummyBackend := ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "core",
			Kind:      "Service",
			Name:      "test-backend",
			Namespace: "test",
		},
		Port: 80,
	}
	processBackend(ctx, policyIR, dummyBackend, testCluster)

	builder := bootstrap.New()
	builder.AddCluster(testCluster)
	bootstrap, err := builder.Build()
	if err != nil {
		return err
	}
	data, err := protojson.Marshal(bootstrap)
	if err != nil {
		return err
	}

	return v.Validate(ctx, string(data))
}
