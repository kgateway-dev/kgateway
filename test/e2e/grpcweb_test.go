package e2e_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/solo-io/gloo/test/gomega/matchers"

	proto_matchers "github.com/solo-io/solo-kit/test/matchers"

	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_web"

	envoy_data_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	envoyals "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Grpc Web", func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
		testContext.BeforeEach()
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("Disable", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				GrpcWeb: &grpc_web.GrpcWeb{
					Disable: true,
				},
			}

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gw,
			}
		})

		It("can disable grpc web filter", func() {
			Eventually(func(g Gomega) {
				proxy, err := testContext.ReadDefaultProxy()
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(proxy.GetListeners()).To(HaveLen(1))
				g.Expect(proxy.GetListeners()[0].GetHttpListener().GetOptions().GetGrpcWeb()).To(proto_matchers.MatchProto(&grpc_web.GrpcWeb{
					Disable: true,
				}))
			}, "5s", ".5s").Should(Succeed())
		})
	})

	Context("Grpc", func() {

		var (
			msgChan <-chan *envoy_data_accesslog_v3.HTTPAccessLogEntry
		)

		BeforeEach(func() {
			accessLogPort := services.NextBindPort()
			grpcUpstream := &gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "grpc-service",
					Namespace: writeNamespace,
				},
				UseHttp2: &wrappers.BoolValue{Value: true},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &static_plugin_gloo.UpstreamSpec{
						Hosts: []*static_plugin_gloo.Host{
							{
								Addr: testContext.EnvoyInstance().LocalAddr(),
								Port: accessLogPort,
							},
						},
					},
				},
			}
			vsToGrpcUpstream := helpers.NewVirtualServiceBuilder().
				WithName("vs-grpc").
				WithNamespace(writeNamespace).
				WithDomain("grpc.com").
				WithRoutePrefixMatcher("grpc", "/").
				WithRouteActionToUpstream("grpc", grpcUpstream).
				Build()

			// we want to test grpc web, so lets reuse the access log service
			// we could use any other service, but we already have the ALS setup for tests
			msgChan = runAccessLog(testContext.Ctx(), accessLogPort)

			gw := gatewaydefaults.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				GrpcWeb: &grpc_web.GrpcWeb{
					Disable: false,
				},
			}

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gw,
			}
			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vsToGrpcUpstream,
			}
			testContext.ResourcesToCreate().Upstreams = gloov1.UpstreamList{
				grpcUpstream,
			}
		})

		It("works with grpc web", func() {
			// make a grpc web request
			toSend := &envoyals.StreamAccessLogsMessage{
				LogEntries: &envoyals.StreamAccessLogsMessage_HttpLogs{
					HttpLogs: &envoyals.StreamAccessLogsMessage_HTTPAccessLogEntries{
						LogEntry: []*envoy_data_accesslog_v3.HTTPAccessLogEntry{{
							CommonProperties: &envoy_data_accesslog_v3.AccessLogCommon{
								UpstreamCluster: "foo",
							},
						}},
					},
				},
			}

			// send toSend using grpc web
			body, err := proto.Marshal(toSend)
			Expect(err).NotTo(HaveOccurred())

			var buffer bytes.Buffer
			// write the length in the buffer
			// compressed flag
			buffer.Write([]byte{0})
			// length
			Expect(len(body)).To(BeNumerically("<=", 0xff))
			buffer.Write([]byte{0, 0, 0, byte(len(body))})

			// write the body to the buffer
			buffer.Write(body)

			dest := make([]byte, base64.StdEncoding.EncodedLen(len(buffer.Bytes())))
			base64.StdEncoding.Encode(dest, buffer.Bytes())
			var bufferbase64 bytes.Buffer
			bufferbase64.Write(dest)

			req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/envoy.service.accesslog.v3.AccessLogService/StreamAccessLogs", defaults.HttpPort), &bufferbase64)
			Expect(err).NotTo(HaveOccurred())

			req.Host = "grpc.com"
			req.Header.Set("content-type", "application/grpc-web-text")

			Eventually(func() (*http.Response, error) {
				return http.DefaultClient.Do(req)
			}, "10s", "0.5s").Should(matchers.HaveOkResponse())

			var entry *envoy_data_accesslog_v3.HTTPAccessLogEntry
			Eventually(msgChan, time.Second).Should(Receive(&entry))
			Expect(entry.CommonProperties.UpstreamCluster).To(Equal("foo"))
		})
	})

})
