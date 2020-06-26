package grpcweb_test

import (
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpcweb"

	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	types "github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_web"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Grpcweb", func() {
	var (
		initParams     plugins.InitParams
		expectedFilter = []plugins.StagedHttpFilter{
			{
				HttpFilter: &envoyhttp.HttpFilter{Name: wellknown.GRPCWeb},
				Stage:      plugins.AfterStage(plugins.AuthZStage),
			},
		}
	)
	BeforeEach(func() {
		settings := &v1.Settings{
			Gloo: &v1.GlooOptions{
				DisableGrpcWeb: &types.BoolValue{
					Value: false,
				},
			},
		}
		initParams = plugins.InitParams{
			Settings: settings,
		}
	})
	Describe("enabled  in settings", func() {

		It("should not add filter if disabled", func() {
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					GrpcWeb: &grpc_web.GrpcWeb{
						Disable: true,
					},
				},
			}

			p := NewPlugin()
			p.Init(initParams)
			f, err := p.HttpFilters(plugins.Params{}, hl)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).To(BeNil())
		})

		It("should add filter if not disabled", func() {
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{},
			}

			p := NewPlugin()
			p.Init(initParams)
			f, err := p.HttpFilters(plugins.Params{}, hl)
			Expect(err).NotTo(HaveOccurred())

			Expect(f).To(BeEquivalentTo(expectedFilter))
		})
	})

	Describe("disabled in settings", func() {
		BeforeEach(func() {
			initParams.Settings.Gloo.DisableGrpcWeb.Value = true
		})
		It("should not filter if disabled by settings", func() {
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{},
			}

			p := NewPlugin()
			p.Init(initParams)
			f, err := p.HttpFilters(plugins.Params{}, hl)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).To(BeNil())
		})
		It("should filter if default by settings", func() {
			initParams.Settings.Gloo.DisableGrpcWeb = nil

			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{},
			}

			p := NewPlugin()
			p.Init(initParams)
			f, err := p.HttpFilters(plugins.Params{}, hl)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).To(BeEquivalentTo(expectedFilter))
		})
		It("should filter when enabled in listener", func() {
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					GrpcWeb: &grpc_web.GrpcWeb{
						Disable: false,
					},
				},
			}

			p := NewPlugin()
			p.Init(initParams)
			f, err := p.HttpFilters(plugins.Params{}, hl)
			Expect(err).NotTo(HaveOccurred())

			Expect(f).To(BeEquivalentTo(expectedFilter))
		})
	})

})
