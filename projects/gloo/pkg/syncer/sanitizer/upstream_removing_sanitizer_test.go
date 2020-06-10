package sanitizer_test

import (
	"context"

	envoycluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
)

var _ = Describe("UpstreamRemovingSanitizer", func() {
	var (
		us = &v1.Upstream{
			Metadata: core.Metadata{
				Name:      "my",
				Namespace: "upstream",
			},
		}
		goodClusterName = translator.UpstreamToClusterName(us.Metadata.Ref())
		goodCluster     = &envoycluster.Cluster{
			Name: goodClusterName,
		}

		badUs = &v1.Upstream{
			Metadata: core.Metadata{
				Name:      "bad",
				Namespace: "upstream",
			},
		}
		badClusterName = translator.UpstreamToClusterName(badUs.Metadata.Ref())
		badCluster     = &envoycluster.Cluster{
			Name: badClusterName,
		}
	)
	It("removes upstreams whose reports have an error, and changes the error to a warning", func() {

		xdsSnapshot := xds.NewSnapshotFromResources(
			envoycache.NewResources("", nil),
			envoycache.NewResources("clusters", []envoycache.Resource{
				xds.NewEnvoyResource(goodCluster),
				xds.NewEnvoyResource(badCluster),
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

		glooSnapshot := &v1.ApiSnapshot{
			Upstreams: v1.UpstreamList{us, badUs},
		}

		snap, err := sanitizer.SanitizeSnapshot(context.TODO(), glooSnapshot, xdsSnapshot, reports)
		Expect(err).NotTo(HaveOccurred())

		clusters := snap.GetResources(xds.ClusterType)

		Expect(clusters.Items).To(HaveLen(1))
		Expect(clusters.Items[goodClusterName].ResourceProto()).To(Equal(goodCluster))

		Expect(reports[badUs]).To(Equal(reporter.Report{
			Warnings: []string{"don't get me started"},
		}))
	})
})
