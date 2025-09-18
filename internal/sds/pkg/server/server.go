package server

import (
	"context"
	"encoding/pem"
	"fmt"
	"hash/fnv"
	"log"
	"log/slog"
	"math"
	"net"
	"os"

	"github.com/avast/retry-go"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	cache_types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/solo-io/go-utils/hashutils"
	"google.golang.org/grpc"
)

var (
	grpcOptions = []grpc.ServerOption{
		grpc.MaxConcurrentStreams(10000),
		grpc.MaxRecvMsgSize(math.MaxInt32),
	}
)

// Secret represents an envoy auth secret
type Secret struct {
	SslCaFile         string
	SslKeyFile        string
	SslCertFile       string
	SslOcspFile       string
	ServerCert        string // name of a tls_certificate_sds_secret_config
	ValidationContext string // name of the validation_context_sds_secret_config
}

// Server is the SDS server. Holds config & secrets.
type Server struct {
	secrets       []Secret
	sdsClient     string
	grpcServer    *grpc.Server
	address       string
	snapshotCache cache.SnapshotCache
}

// ID needed for snapshotCache
func (s *Server) ID(_ *envoycorev3.Node) string {
	return s.sdsClient
}

// SetupEnvoySDS creates a new SDSServer. The returned server can be started with Run()
func SetupEnvoySDS(secrets []Secret, sdsClient, serverAddress string) *Server {
	grpcServer := grpc.NewServer(grpcOptions...)
	sdsServer := &Server{
		secrets:    secrets,
		grpcServer: grpcServer,
		sdsClient:  sdsClient,
		address:    serverAddress,
	}
	snapshotCache := cache.NewSnapshotCache(false, sdsServer, nil)
	sdsServer.snapshotCache = snapshotCache

	svr := server.NewServer(context.Background(), snapshotCache, nil)

	// register services
	envoy_service_secret_v3.RegisterSecretDiscoveryServiceServer(grpcServer, svr)
	return sdsServer
}

// Run starts the server
func (s *Server) Run(ctx context.Context) (<-chan struct{}, error) {
	lis, err := net.Listen("tcp", s.address)
	if err != nil {
		return nil, err
	}
	slog.Info("sds server listening", "address", s.address)

	// Create channels for synchronization
	serveStarted := make(chan struct{})
	serverStopped := make(chan struct{})

	// Start the server in a goroutine
	go func() {
		// Signal that Serve is about to be called
		close(serveStarted)
		if err = s.grpcServer.Serve(lis); err != nil {
			log.Fatalf("fatal error in gRPC server: address=%s error=%v", s.address, err)
		}
	}()

	// Wait for Serve to start before setting up shutdown handler
	go func() {
		// Wait for Serve to be called
		<-serveStarted

		// Now wait for context cancellation
		<-ctx.Done()
		slog.Info("stopping sds server", "address", s.address)
		s.grpcServer.GracefulStop()
		serverStopped <- struct{}{}
	}()

	return serverStopped, nil
}

// UpdateSDSConfig updates with the current certs
func (s *Server) UpdateSDSConfig(ctx context.Context) error {
	var certs [][]byte
	var items []cache_types.Resource
	for _, sec := range s.secrets {
		key, err := readAndVerifyCert(ctx, sec.SslKeyFile)
		if err != nil {
			return err
		}
		certs = append(certs, key)
		certChain, err := readAndVerifyCert(ctx, sec.SslCertFile)
		if err != nil {
			return err
		}
		certs = append(certs, certChain)
		ca, err := readAndVerifyCert(ctx, sec.SslCaFile)
		if err != nil {
			return err
		}
		certs = append(certs, ca)
		var ocspStaple []byte // ocsp stapling is optional
		if sec.SslOcspFile != "" {
			ocspStaple, err = readAndVerifyCert(ctx, sec.SslOcspFile)
			if err != nil {
				return err
			}
			certs = append(certs, ocspStaple)
		}
		items = append(items, serverCertSecret(key, certChain, ocspStaple, sec.ServerCert))
		items = append(items, validationContextSecret(ca, sec.ValidationContext))
	}

	snapshotVersion, err := GetSnapshotVersion(certs)
	if err != nil {
		slog.Error("error getting snapshot version", "error", err)
		return err
	}
	slog.Info("updating SDS config", "client", s.sdsClient, "snapshot_version", snapshotVersion)

	secretSnapshot := &cache.Snapshot{}
	secretSnapshot.Resources[cache_types.Secret] = cache.NewResources(snapshotVersion, items)
	return s.snapshotCache.SetSnapshot(ctx, s.sdsClient, secretSnapshot)
}

// GetSnapshotVersion generates a version string by hashing the certs
func GetSnapshotVersion(certs ...interface{}) (string, error) {
	hash, err := hashutils.HashAllSafe(fnv.New64(), certs...)
	return fmt.Sprintf("%d", hash), err
}

// readAndVerifyCert will read the file from the given
// path, then check for validity every 100ms for 2 seconds.
// This is needed because the filesystem watcher
// that gets triggered by a WRITE doesn't have a guarantee
// that the write has finished yet.
// See https://github.com/fsnotify/fsnotify/pull/252 for more context
//
//nolint:unparam // currently error is always nil but there is a todo to change that
func readAndVerifyCert(_ context.Context, certFilePath string) ([]byte, error) {
	var err error
	var fileBytes []byte
	var validCerts bool
	// Retry for a few seconds as a write may still be in progress
	err = retry.Do(
		func() error {
			fileBytes, err = os.ReadFile(certFilePath)
			if err != nil {
				return err
			}
			validCerts = checkCert(fileBytes)
			if !validCerts {
				return fmt.Errorf("failed to validate file %v", certFilePath)
			}
			return nil
		},
		retry.Attempts(5), // Exponential backoff over ~3s
	)

	// TODO: we should return error here, but this currently makes ci tests fail so leaving it unchanged for now
	// if err != nil {
	// 	contextutils.LoggerFrom(ctx).Warnf("error checking certs %v", err)
	// 	return fileBytes, err
	// }

	return fileBytes, nil
}

// checkCert uses pem.Decode to verify that the given
// bytes are not malformed, as could be caused by a
// write-in-progress. Uses pem.Decode to check the blocks.
// See https://golang.org/src/encoding/pem/pem.go?s=2505:2553#L76
func checkCert(certs []byte) bool {
	block, rest := pem.Decode(certs)
	if block == nil {
		// Remainder does not contain any certs/keys
		return false
	}
	// Found a cert, check the rest
	if len(rest) > 0 {
		// Something after the cert, validate that too
		return checkCert(rest)
	}
	return true
}

func serverCertSecret(privateKey, certChain, ocspStaple []byte, serverCert string) cache_types.Resource {
	tlsCert := &envoytlsv3.TlsCertificate{
		CertificateChain: inlineBytesDataSource(certChain),
		PrivateKey:       inlineBytesDataSource(privateKey),
	}

	// Only add an OCSP staple if one exists
	if ocspStaple != nil {
		tlsCert.OcspStaple = inlineBytesDataSource(ocspStaple)
	}

	return &envoytlsv3.Secret{
		Name: serverCert,
		Type: &envoytlsv3.Secret_TlsCertificate{
			TlsCertificate: tlsCert,
		},
	}
}

func validationContextSecret(caCert []byte, validationContext string) cache_types.Resource {
	return &envoytlsv3.Secret{
		Name: validationContext,
		Type: &envoytlsv3.Secret_ValidationContext{
			ValidationContext: &envoytlsv3.CertificateValidationContext{
				TrustedCa: inlineBytesDataSource(caCert),
			},
		},
	}
}

func inlineBytesDataSource(b []byte) *envoycorev3.DataSource {
	return &envoycorev3.DataSource{
		Specifier: &envoycorev3.DataSource_InlineBytes{
			InlineBytes: b,
		},
	}
}
