package backendconfigpolicy

import (
	"context"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
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

	testCluster := &envoyclusterv3.Cluster{
		Name:                 "test-cluster-for-validation",
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC},
		ConnectTimeout:       durationpb.New(5 * time.Second),
	}
	if policyIR.loadBalancerConfig != nil && policyIR.loadBalancerConfig.useHostnameForHashing {
		dnsClusterConfig, err := utils.MessageToAny(&envoydnsv3.DnsCluster{})
		if err != nil {
			return err
		}
		testCluster.ClusterDiscoveryType = &envoyclusterv3.Cluster_ClusterType{
			ClusterType: &envoyclusterv3.Cluster_CustomClusterType{
				Name:        dnsClusterExtensionName,
				TypedConfig: dnsClusterConfig,
			},
		}
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

	return v.Validate(ctx, bootstrap)
}
