package grpcweb_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpcweb"

	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc_web"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Grpcweb", func() {

	It("should not add filter if disabled", func() {
		hl := &v1.HttpListener{
			ListenerPlugins: &v1.ListenerPlugins{
				GrpcWeb: &grpc_web.GrpcWeb{
					Disable: true,
				},
			},
		}

		p := NewPlugin()
		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).To(BeNil())
	})

	It("should add filter if disabled", func() {
		hl := &v1.HttpListener{
			ListenerPlugins: &v1.ListenerPlugins{},
		}

		p := NewPlugin()
		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).NotTo(HaveOccurred())

		exptected := []plugins.StagedHttpFilter{
			{
				HttpFilter: &envoyhttp.HttpFilter{Name: envoyutil.GRPCWeb},
				Stage:      plugins.PostInAuth,
			},
		}
		Expect(f).To(BeEquivalentTo(exptected))

	})

})
