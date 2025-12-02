package server_test

import (
	"context"
	"os"
	"strings"
	"time"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/spf13/afero"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/internal/sds/pkg/server"
	"github.com/kgateway-dev/kgateway/v2/internal/sds/pkg/testutils"
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
		fileString := `test`

		keyFile, err = afero.TempFile(fs, dir, "")
		Expect(err).NotTo(HaveOccurred())
		_, err = keyFile.WriteString(fileString)
		Expect(err).NotTo(HaveOccurred())

		certFile, err = afero.TempFile(fs, dir, "")
		Expect(err).NotTo(HaveOccurred())
		_, err = certFile.WriteString(fileString)
		Expect(err).NotTo(HaveOccurred())

		caFile, err = afero.TempFile(fs, dir, "")
		Expect(err).NotTo(HaveOccurred())
		_, err = caFile.WriteString(fileString)
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
				SslOcspFile:       ocspResponseFile.Name(),
				ValidationContext: "test-validation",
			},
		}
		srv = server.SetupEnvoySDS(secrets, sdsClient, serverAddr)
	})

	AfterEach(func() {
		_ = fs.RemoveAll(dir)
	})

	DescribeTable("correctly reads tls secrets from files to generate snapshot version", func(useOcsp bool, expectedHashes []string) {
		certs, err := testutils.FilesToBytes(keyFile.Name(), certFile.Name(), caFile.Name())
		Expect(err).NotTo(HaveOccurred())
		if useOcsp {
			ocspResponse, err := os.ReadFile(ocspResponseFile.Name())
			Expect(err).NotTo(HaveOccurred())
			certs = append(certs, ocspResponse)
		}

		snapshotVersion, err := server.GetSnapshotVersion(certs)
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshotVersion).To(Equal(expectedHashes[0]))

		// Test that the snapshot version changes if the contents of the file changes
		_, err = keyFile.WriteString(`newFileString`)
		Expect(err).NotTo(HaveOccurred())
		certs, err = testutils.FilesToBytes(keyFile.Name(), certFile.Name(), caFile.Name())
		Expect(err).NotTo(HaveOccurred())
		if useOcsp {
			ocspResponse, err := os.ReadFile(ocspResponseFile.Name())
			Expect(err).NotTo(HaveOccurred())
			certs = append(certs, ocspResponse)
		}

		snapshotVersion, err = server.GetSnapshotVersion(certs)
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshotVersion).To(Equal(expectedHashes[1]))
	},
		Entry("without ocsps", false, []string{"6730780456972595554", "4234248347190811569"}),
		Entry("with ocsps", true, []string{"969835737182439215", "6328977429293055969"}),
	)

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
						Expect(resource.String()).To(ContainSubstring("ocsp_staple"))
					} else if strings.Contains(resourceData, "test-validation") {
						Expect(resource.String()).To(ContainSubstring("trusted_ca"))
					}
				}
			}
		})
	})
})
