package sanitizer_test

import (
	"context"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyclusterapi "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
)

var _ = Describe("UpstreamRemovingSanitizer", func() {
	var (
		us = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "my",
				Namespace: "upstream",
			},
		}
		clusterType = &envoy_config_cluster_v3.Cluster_Type{
			Type: envoy_config_cluster_v3.Cluster_EDS,
		}
		goodClusterName = translator.UpstreamToClusterName(us.Metadata.Ref())
		goodCluster     = &envoyclusterapi.Cluster{
			Name:                 goodClusterName,
			ClusterDiscoveryType: clusterType,
			EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
				ServiceName: goodClusterName + "-123",
			},
		}
		goodEndpoint = &envoyclusterapi.Cluster{
			Name: goodClusterName + "-123",
		}

		badUs = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "bad",
				Namespace: "upstream",
			},
		}
		badClusterName = translator.UpstreamToClusterName(badUs.Metadata.Ref())
		badCluster     = &envoyclusterapi.Cluster{
			Name:                 badClusterName,
			ClusterDiscoveryType: clusterType,
			EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
				ServiceName: badClusterName + "-123",
			},
		}
		badEndpoint = &envoyclusterapi.Cluster{
			Name: badClusterName + "-123",
		}
	)
	It("removes upstreams whose reports have an error, and changes the error to a warning", func() {

		xdsSnapshot := xds.NewSnapshotFromResources(
			envoycache.NewResources("unit_test", []envoycache.Resource{
				resource.NewEnvoyResource(goodEndpoint),
				resource.NewEnvoyResource(badEndpoint),
			}),
			envoycache.NewResources("unit_test", []envoycache.Resource{
				resource.NewEnvoyResource(goodCluster),
				resource.NewEnvoyResource(badCluster),
			}),
			envoycache.NewResources("", nil),
			envoycache.NewResources("", nil),
		)
		sanitizer := NewUpstreamRemovingSanitizer()

		reports := reporter.ResourceReports{
			&v1.Proxy{}: {
				Warnings: []string{"route with missing upstream"},
			},
			us: {},
			badUs: {
				Errors: eris.Errorf("don't get me started"),
			},
		}

		glooSnapshot := &v1snap.ApiSnapshot{
			Upstreams: v1.UpstreamList{us, badUs},
		}

		snap := sanitizer.SanitizeSnapshot(context.TODO(), glooSnapshot, xdsSnapshot, reports)

		clusters := snap.GetResources(types.ClusterTypeV3)

		Expect(clusters.Items).To(HaveLen(1))
		Expect(clusters.Items[goodClusterName].ResourceProto()).To(Equal(goodCluster))

		Expect(reports[badUs]).To(Equal(reporter.Report{
			Warnings: []string{"don't get me started"},
		}))
	})
})
