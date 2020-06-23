package server_test

import (
	"context"

	"github.com/solo-io/gloo/projects/sds/pkg/server"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_service_discovery_v2 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/spf13/afero"
	"google.golang.org/grpc"

	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SDS Server", func() {

	var (
		fs                        afero.Fs
		dir                       string
		keyFile, certFile, caFile afero.File
		err                       error
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
	})

	AfterEach(func() {
		_ = fs.RemoveAll(dir)
	})

	It("correctly reads tls secrets from files to generate snapshot version", func() {
		snapshotVersion, err := server.GetSnapshotVersion(keyFile.Name(), certFile.Name(), caFile.Name())
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("11240719828806193304"))

		// Test that the snapshot version changes if the contents of the file changes
		_, err = keyFile.WriteString(`newFileString`)
		Expect(err).To(BeNil())
		snapshotVersion, err = server.GetSnapshotVersion(keyFile.Name(), certFile.Name(), caFile.Name())
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("15327026688369869607"))
	})

	It("correctly updates SDSConfig", func() {
		ctx, _ := context.WithCancel(context.Background())
		hasher := &server.EnvoyKey{}
		snapshotCache := cache.NewSnapshotCache(false, hasher, nil)
		server.UpdateSDSConfig(ctx, keyFile.Name(), certFile.Name(), caFile.Name(), snapshotCache)
		_, err := snapshotCache.GetSnapshot(hasher.ID(nil))
		Expect(err).To(BeNil())
	})

	Context("Test gRPC Server", func() {
		var (
			ctx               context.Context
			cancel            context.CancelFunc
			grpcServer        *grpc.Server
			snapshotCache     cache.SnapshotCache
			testServerAddress = "127.0.0.1:8236"
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			grpcServer, snapshotCache = server.SetupEnvoySDS()
			_, err = server.RunSDSServer(ctx, grpcServer, testServerAddress)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			cancel()
		})

		It("accepts client connections", func() {
			// Check that it's answering
			var conn *grpc.ClientConn

			// Initiate a connection with the server
			conn, err := grpc.Dial(testServerAddress, grpc.WithInsecure())
			Expect(err).To(BeNil())
			defer conn.Close()

			client := envoy_service_discovery_v2.NewSecretDiscoveryServiceClient(conn)

			// Before any snapshot is set, expect an error when fetching secrets
			resp, err := client.FetchSecrets(ctx, &envoy_api_v2.DiscoveryRequest{})
			Expect(err).NotTo(BeNil())

			// After snapshot is set, expect to see the secrets
			server.UpdateSDSConfig(ctx, keyFile.Name(), certFile.Name(), caFile.Name(), snapshotCache)
			resp, err = client.FetchSecrets(ctx, &envoy_api_v2.DiscoveryRequest{})
			Expect(err).To(BeNil())
			Expect(len(resp.GetResources())).To(Equal(2))
			Expect(resp.Validate()).To(BeNil())
		})
	})
})
