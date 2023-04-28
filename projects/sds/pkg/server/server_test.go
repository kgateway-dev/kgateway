package server_test

import (
	"context"
	"os"
	"strings"
	"time"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/sds/pkg/server"
	"github.com/solo-io/gloo/projects/sds/pkg/testutils"
	"github.com/spf13/afero"
	"google.golang.org/grpc"
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
		Expect(err).To(BeNil())
		fileString := `test`

		keyFile, err = afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = keyFile.WriteString(fileString)
		Expect(err).To(BeNil())

		certFile, err = afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = certFile.WriteString(fileString)
		Expect(err).To(BeNil())

		caFile, err = afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = caFile.WriteString(fileString)
		Expect(err).To(BeNil())

		ocspResponseFile, err = afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		ocspResp, err := os.ReadFile("certs/ocsp_response.der")
		Expect(err).To(BeNil())
		_, err = ocspResponseFile.Write(ocspResp)
		Expect(err).To(BeNil())
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
			Expect(err).To(BeNil())
			certs = append(certs, ocspResponse)
		}

		snapshotVersion, err := server.GetSnapshotVersion(certs)
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal(expectedHashes[0]))

		// Test that the snapshot version changes if the contents of the file changes
		_, err = keyFile.WriteString(`newFileString`)
		Expect(err).To(BeNil())
		certs, err = testutils.FilesToBytes(keyFile.Name(), certFile.Name(), caFile.Name())
		Expect(err).NotTo(HaveOccurred())
		if useOcsp {
			ocspResponse, err := os.ReadFile(ocspResponseFile.Name())
			Expect(err).To(BeNil())
			certs = append(certs, ocspResponse)
		}

		snapshotVersion, err = server.GetSnapshotVersion(certs)
		Expect(err).To(BeNil())
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
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			cancel()
		})

		It("accepts client connections & updates secrets", func() {
			// Check that it's answering
			var conn *grpc.ClientConn

			// Initiate a connection with the server
			conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
			Expect(err).To(BeNil())
			defer conn.Close()

			client := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(conn)

			// Before any snapshot is set, expect an error when fetching secrets
			resp, err := client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			Expect(err).NotTo(BeNil())

			// After snapshot is set, expect to see the secrets
			srv.UpdateSDSConfig(ctx)
			resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			Expect(err).To(BeNil())
			Expect(len(resp.GetResources())).To(Equal(2))
			Expect(resp.Validate()).To(BeNil())

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
