package translator_test

import (
	"context"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoycluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	types "github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	staticplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Translator", func() {
	var (
		settings   *v1.Settings
		translator Translator
		upstream   *v1.Upstream
		proxy      *v1.Proxy
		params     plugins.Params
		cluster    *envoyapi.Cluster
	)

	BeforeEach(func() {
		cluster = nil
		settings = &v1.Settings{}
		tplugins := []plugins.Plugin{
			staticplugin.NewPlugin(),
		}
		translator = NewTranslator(tplugins, settings)

		upname := core.Metadata{
			Name:      "test",
			Namespace: "gloo-system",
		}
		upstream = &v1.Upstream{
			Metadata: upname,
			UpstreamSpec: &v1.UpstreamSpec{
				UpstreamType: &v1.UpstreamSpec_Static{
					Static: &v1static.UpstreamSpec{
						Hosts: []*v1static.Host{
							{
								Addr: "Test",
								Port: 124,
							},
						},
					},
				},
			},
		}
		params = plugins.Params{
			Ctx: context.Background(),
			Snapshot: &v1.ApiSnapshot{
				Upstreams: v1.UpstreamsByNamespace{
					"gloo-system": v1.UpstreamList{
						upstream,
					},
				},
			},
		}

		proxy = &v1.Proxy{}
	})

	translate := func() {

		snap, errs, err := translator.Translate(params, proxy)
		Expect(err).NotTo(HaveOccurred())
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())

		clusters := snap.GetResources(xds.ClusterType)
		clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
		cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
		Expect(cluster).NotTo(BeNil())
	}

	It("should NOT translate circuit breakers on upstream", func() {
		translate()
		Expect(cluster.CircuitBreakers).To(BeNil())
	})

	It("should translate circuit breakers on upstream", func() {

		upstream.UpstreamSpec.CircuitBreakers = &v1.CircuitBreakerConfig{
			MaxConnections:     &types.UInt32Value{Value: 1},
			MaxPendingRequests: &types.UInt32Value{Value: 2},
			MaxRequests:        &types.UInt32Value{Value: 3},
			MaxRetries:         &types.UInt32Value{Value: 4},
		}

		expectedCircuitBreakers := &envoycluster.CircuitBreakers{
			Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
				{
					MaxConnections:     &types.UInt32Value{Value: 1},
					MaxPendingRequests: &types.UInt32Value{Value: 2},
					MaxRequests:        &types.UInt32Value{Value: 3},
					MaxRetries:         &types.UInt32Value{Value: 4},
				},
			},
		}
		translate()

		Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
	})

	It("should translate circuit breakers on settings", func() {

		settings.CircuitBreakers = &v1.CircuitBreakerConfig{
			MaxConnections:     &types.UInt32Value{Value: 1},
			MaxPendingRequests: &types.UInt32Value{Value: 2},
			MaxRequests:        &types.UInt32Value{Value: 3},
			MaxRetries:         &types.UInt32Value{Value: 4},
		}

		expectedCircuitBreakers := &envoycluster.CircuitBreakers{
			Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
				{
					MaxConnections:     &types.UInt32Value{Value: 1},
					MaxPendingRequests: &types.UInt32Value{Value: 2},
					MaxRequests:        &types.UInt32Value{Value: 3},
					MaxRetries:         &types.UInt32Value{Value: 4},
				},
			},
		}
		translate()

		Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
	})

	It("should override circuit breakers on upstream", func() {

		settings.CircuitBreakers = &v1.CircuitBreakerConfig{
			MaxConnections:     &types.UInt32Value{Value: 11},
			MaxPendingRequests: &types.UInt32Value{Value: 12},
			MaxRequests:        &types.UInt32Value{Value: 13},
			MaxRetries:         &types.UInt32Value{Value: 14},
		}

		upstream.UpstreamSpec.CircuitBreakers = &v1.CircuitBreakerConfig{
			MaxConnections:     &types.UInt32Value{Value: 1},
			MaxPendingRequests: &types.UInt32Value{Value: 2},
			MaxRequests:        &types.UInt32Value{Value: 3},
			MaxRetries:         &types.UInt32Value{Value: 4},
		}

		expectedCircuitBreakers := &envoycluster.CircuitBreakers{
			Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
				{
					MaxConnections:     &types.UInt32Value{Value: 1},
					MaxPendingRequests: &types.UInt32Value{Value: 2},
					MaxRequests:        &types.UInt32Value{Value: 3},
					MaxRetries:         &types.UInt32Value{Value: 4},
				},
			},
		}
		translate()

		Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
	})

})
