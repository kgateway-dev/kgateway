package server_test

import (
	"context"
	"os"
	"strings"
	"time"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/spf13/afero"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/sds/server"
	"github.com/kgateway-dev/kgateway/v2/pkg/sds/testutils"
)

var _ = Describe("SDS Server", func() {
	var (
		fs                                          afero.Fs
		dir                                         string
		keyFile, certFile, caFile, ocspResponseFile afero.File
		err                                         error
		serverAddr                                  = "127.0.0.1:8888"
		sdsClient                                   = "test-client"
		srv                                         *server.Server
	)

	BeforeEach(func() {
		fs = afero.NewOsFs()
		dir, err = afero.TempDir(fs, "", "")
		Expect(err).NotTo(HaveOccurred())

		keyPEM, certPEM, caPEM := testutils.MustSelfSignedPEM()

		keyFile, err = afero.TempFile(fs, dir, "")
		Expect(err).NotTo(HaveOccurred())
		_, err = keyFile.Write(keyPEM)
		Expect(err).NotTo(HaveOccurred())

		certFile, err = afero.TempFile(fs, dir, "")
		Expect(err).NotTo(HaveOccurred())
		_, err = certFile.Write(certPEM)
		Expect(err).NotTo(HaveOccurred())

		caFile, err = afero.TempFile(fs, dir, "")
		Expect(err).NotTo(HaveOccurred())
		_, err = caFile.Write(caPEM)
		Expect(err).NotTo(HaveOccurred())

		ocspResponseFile, err = afero.TempFile(fs, dir, "")
		Expect(err).NotTo(HaveOccurred())
		ocspResp, err := os.ReadFile("certs/ocsp_response.der")
		Expect(err).NotTo(HaveOccurred())
		_, err = ocspResponseFile.Write(ocspResp)
		Expect(err).NotTo(HaveOccurred())
		secrets := []server.Secret{
			{
				ServerCert:        "test-server",
				SslCaFile:         caFile.Name(),
				SslCertFile:       certFile.Name(),
				SslKeyFile:        keyFile.Name(),
				ValidationContext: "test-validation",
			},
		}
		srv = server.SetupEnvoySDS(secrets, sdsClient, serverAddr)
	})

	AfterEach(func() {
		_ = fs.RemoveAll(dir)
	})

	It("correctly reads tls secrets from files to generate snapshot version", func() {
		certs, err := testutils.FilesToBytes(keyFile.Name(), certFile.Name(), caFile.Name())
		Expect(err).NotTo(HaveOccurred())

		snapshotVersionBefore, err := server.GetSnapshotVersion(certs)
		Expect(err).NotTo(HaveOccurred())

		// Test that the snapshot version changes if the contents of the file changes
		_, err = keyFile.WriteString(`newFileString`)
		Expect(err).NotTo(HaveOccurred())
		certs, err = testutils.FilesToBytes(keyFile.Name(), certFile.Name(), caFile.Name())
		Expect(err).NotTo(HaveOccurred())

		snapshotVersionAfter, err := server.GetSnapshotVersion(certs)
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshotVersionAfter).NotTo(Equal(snapshotVersionBefore))
	})

	It("includes configured ocsp staple in the served tls secret", func() {
		ocspServerAddr := "127.0.0.1:8889"
		ocspSrv := server.SetupEnvoySDS([]server.Secret{{
			ServerCert:        "test-server",
			SslCaFile:         caFile.Name(),
			SslCertFile:       certFile.Name(),
			SslKeyFile:        keyFile.Name(),
			SslOcspFile:       ocspResponseFile.Name(),
			ValidationContext: "test-validation",
		}}, sdsClient, ocspServerAddr)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err = ocspSrv.Run(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(ocspSrv.UpdateSDSConfig(ctx)).To(Succeed())

		conn, err := grpc.NewClient(ocspServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		Expect(err).NotTo(HaveOccurred())
		defer conn.Close()

		client := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(conn)
		var resp *envoy_service_discovery_v3.DiscoveryResponse
		Eventually(func() bool {
			resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			return err == nil
		}, "10s", "200ms").Should(BeTrue())

		ocspBytes, err := os.ReadFile(ocspResponseFile.Name())
		Expect(err).NotTo(HaveOccurred())

		var serverSecret *envoytlsv3.Secret
		for _, resource := range resp.GetResources() {
			parsed := new(envoytlsv3.Secret)
			Expect(resource.UnmarshalTo(parsed)).To(Succeed())
			if parsed.GetName() == "test-server" {
				serverSecret = parsed
				break
			}
		}
		Expect(serverSecret).NotTo(BeNil())
		Expect(serverSecret.GetTlsCertificate()).NotTo(BeNil())
		Expect(serverSecret.GetTlsCertificate().GetOcspStaple()).NotTo(BeNil())
		Expect(serverSecret.GetTlsCertificate().GetOcspStaple().GetInlineBytes()).To(Equal(ocspBytes))
	})

	Context("Test gRPC Server", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			_, err = srv.Run(ctx)
			// Give it a second to come up + read the certs
			time.Sleep(time.Second * 1)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cancel()
		})

		It("accepts client connections & updates secrets", func() {
			// Check that it's answering
			var conn *grpc.ClientConn

			// Initiate a connection with the server
			conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			client := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(conn)

			// Before any snapshot is set, expect an error when fetching secrets
			resp, err := client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			Expect(err).To(HaveOccurred())
			Expect(resp).To(BeNil())

			// After snapshot is set, expect to see the secrets
			srv.UpdateSDSConfig(ctx)
			resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetResources()).To(HaveLen(2))
			Expect(resp.Validate()).To(Succeed())

			// Check that the resources contain the expected data
			for _, resource := range resp.GetResources() {
				if resource.GetTypeUrl() == "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret" {
					resourceData := resource.String()
					if strings.Contains(resourceData, "test-server") {
						Expect(resource.String()).To(ContainSubstring("certificate_chain"))
						Expect(resource.String()).To(ContainSubstring("private_key"))
					} else if strings.Contains(resourceData, "test-validation") {
						Expect(resource.String()).To(ContainSubstring("trusted_ca"))
					}
				}
			}
		})
	})
})
