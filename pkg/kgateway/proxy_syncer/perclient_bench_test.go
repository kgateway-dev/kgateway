package proxy_syncer

import (
	"context"
	"fmt"
	"testing"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

const (
	benchBackends          = 200
	benchClients           = 8
	benchValidationLatency = 2 * time.Millisecond
)

type benchLatencyValidator struct{ latency time.Duration }

func (f *benchLatencyValidator) Validate(_ context.Context, _ *envoybootstrapv3.Bootstrap) error {
	time.Sleep(f.latency)
	return nil
}

func benchTranslator(v validator.Validator) *irtranslator.BackendTranslator {
	return &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "", Kind: "Service"}: {
				InitEnvoyBackend: func(_ context.Context, _ ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
		Validator: v,
		Mode:      apisettings.ValidationStrict,
	}
}

func benchBackend(name string) *ir.BackendObjectIR {
	b := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "",
		Kind:      "Service",
		Namespace: "default",
		Name:      name,
	}, 443, "", "")
	return &b
}

func benchClient(role, pod string) ir.UniquelyConnectedClient {
	return ir.NewUniquelyConnectedClient(role, "ns",
		map[string]string{"app": "gw", "pod": pod}, ir.PodLocality{})
}

func benchDrainScenario(b *testing.B, v validator.Validator) {
	b.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	clients := make([]ir.UniquelyConnectedClient, 0, benchClients)
	for i := range benchClients {
		clients = append(clients, benchClient(fmt.Sprintf("role-%d", i), fmt.Sprintf("p%d", i)))
	}
	uccs := krt.NewStaticCollection(nil, clients, krtopts.ToOptions("UniqueClients")...)
	backends := make([]*ir.BackendObjectIR, 0, benchBackends)
	for i := range benchBackends {
		backends = append(backends, benchBackend(fmt.Sprintf("b%d", i)))
	}
	finalBackends := krt.NewStaticCollection(nil, backends, krtopts.ToOptions("FinalBackends")...)
	clusters := NewPerClientEnvoyClusters(ctx, krtopts, benchTranslator(v), finalBackends, uccs)

	waitDrained := func(ucc ir.UniquelyConnectedClient) {
		deadline := time.Now().Add(10 * time.Minute)
		for time.Now().Before(deadline) {
			if len(clusters.FetchClustersForClient(krt.TestingDummyContext{}, ucc)) == benchBackends {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		b.Fatalf("client %s never drained", ucc.ResourceName())
	}
	waitDrained(clients[len(clients)-1])

	probe := benchClient("role-probe", "probe")
	b.ResetTimer()
	for b.Loop() {
		uccs.UpdateObject(probe)
		waitDrained(probe)
		b.StopTimer()
		uccs.DeleteObject(probe.ResourceName())
		deadline := time.Now().Add(10 * time.Minute)
		for time.Now().Before(deadline) {
			if len(clusters.FetchClustersForClient(krt.TestingDummyContext{}, probe)) == 0 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		b.StartTimer()
	}
}

func BenchmarkPerClientConnectDrain_BinaryValidator(b *testing.B) {
	benchDrainScenario(b, &benchLatencyValidator{latency: benchValidationLatency})
}

func BenchmarkPerClientConnectDrain_CachedValidator(b *testing.B) {
	benchDrainScenario(b, validator.NewCaching(&benchLatencyValidator{latency: benchValidationLatency}, 0))
}
