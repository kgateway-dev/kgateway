package e2e_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	envoy_data_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	envoyals "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Grpc Web", func() {

	var (
		tc                TestContext
		baseAccessLogPort = uint32(37000)
		accessLogPort     uint32
	)

	Describe("in memory", func() {

		BeforeEach(func() {
			tc.What = services.What{
				DisableGateway: false,
				DisableFds:     true,
				DisableUds:     true,
			}
			tc.Before()
			tc.EnsureDefaultGateways()
		})
		AfterEach(tc.After)

		Context("Grpc Web", func() {

			var (
				envoyInstance *services.EnvoyInstance
			)

			BeforeEach(func() {
				envoyInstance = envoyFactory.MustEnvoyInstance()
			})

			Context("Grpc", func() {

				var (
					msgChan      <-chan *envoy_data_accesslog_v3.HTTPAccessLogEntry
					grpcUpstream *gloov1.Upstream
				)

				BeforeEach(func() {
					accessLogPort = services.AdvanceBindPort(&baseAccessLogPort)
					grpcUpstream = &gloov1.Upstream{
						Metadata: &core.Metadata{
							Name:      "grpc-service",
							Namespace: "default",
						},
						UseHttp2: &wrappers.BoolValue{Value: true},
						UpstreamType: &gloov1.Upstream_Static{
							Static: &static_plugin_gloo.UpstreamSpec{
								Hosts: []*static_plugin_gloo.Host{
									{
										Addr: envoyInstance.LocalAddr(),
										Port: accessLogPort,
									},
								},
							},
						},
					}
					_, err := tc.TestClients.UpstreamClient.Write(grpcUpstream, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					err = envoyInstance.RunWith(tc)
					Expect(err).NotTo(HaveOccurred())

					// we want to test grpc web, so lets reuse the access log service
					// we could use any other service, but we already have the ALS setup for tests
					msgChan = runAccessLog(tc.Ctx, accessLogPort)

					// make sure the vs is set and the upstream is ready
					vs := getTrivialVirtualServiceForUpstream("gloo-system", grpcUpstream.Metadata.Ref())
					_, err = tc.TestClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())
					v1helpers.ExpectGrpcHealthOK(nil, defaults.HttpPort, "AccessLog")
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

					req.Header.Set("content-type", "application/grpc-web-text")

					Eventually(func() error {
						resp, err := http.DefaultClient.Do(req)
						if err != nil {
							return err
						}
						if resp.StatusCode != http.StatusOK {
							return fmt.Errorf("not ok")
						}
						return nil
					}, 10*time.Second, time.Second/10).Should(Not(HaveOccurred()))

					var entry *envoy_data_accesslog_v3.HTTPAccessLogEntry
					Eventually(msgChan, time.Second).Should(Receive(&entry))
					Expect(entry.CommonProperties.UpstreamCluster).To(Equal("foo"))
				})
			})
		})
	})
})
