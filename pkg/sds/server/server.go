package server

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"hash"
	"hash/fnv"
	"log"
	"log/slog"
	"math"
	"net"
	"os"
	"time"

	"github.com/avast/retry-go/v4"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	cache_types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/mitchellh/hashstructure"
	"google.golang.org/grpc"
)

var grpcOptions = []grpc.ServerOption{
	grpc.MaxConcurrentStreams(10000),
	grpc.MaxRecvMsgSize(math.MaxInt32),
}

const (
	// A debounced reload can spend a small bounded window re-reading files if the
	// key and cert were updated non-atomically during rotation.
	sdsKeyPairValidationAttempts = 15
	sdsKeyPairValidationDelay    = 100 * time.Millisecond
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
		secretCerts, secretItems, err := readAndValidateSecret(ctx, sec)
		if err != nil {
			return err
		}
		certs = append(certs, secretCerts...)
		items = append(items, secretItems...)
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

// readAndValidateSecret retries reading one secret until the certificate chain
// and private key form a valid pair, which avoids publishing mixed-generation
// material during a non-atomic rotation.
func readAndValidateSecret(ctx context.Context, sec Secret) ([][]byte, []cache_types.Resource, error) {
	var certs [][]byte
	var items []cache_types.Resource
	attempts := 0

	err := retry.Do(
		func() error {
			attempts++
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			key, err := readAndVerifyCert(ctx, sec.SslKeyFile)
			if err != nil {
				return fmt.Errorf("reading private key %q: %w", sec.SslKeyFile, err)
			}

			certChain, err := readAndVerifyCert(ctx, sec.SslCertFile)
			if err != nil {
				return fmt.Errorf("reading certificate chain %q: %w", sec.SslCertFile, err)
			}

			if _, err := tls.X509KeyPair(certChain, key); err != nil {
				return fmt.Errorf("validating certificate chain %q with private key %q: %w", sec.SslCertFile, sec.SslKeyFile, err)
			}

			ca, err := readAndVerifyCert(ctx, sec.SslCaFile)
			if err != nil {
				return fmt.Errorf("reading CA bundle %q: %w", sec.SslCaFile, err)
			}

			var ocspStaple []byte
			if sec.SslOcspFile != "" {
				ocspStaple, err = readFile(ctx, sec.SslOcspFile)
				if err != nil {
					return fmt.Errorf("reading OCSP staple %q: %w", sec.SslOcspFile, err)
				}
			}

			certs = [][]byte{key, certChain, ca}
			if sec.SslOcspFile != "" {
				certs = append(certs, ocspStaple)
			}

			items = []cache_types.Resource{
				serverCertSecret(key, certChain, ocspStaple, sec.ServerCert),
				validationContextSecret(ca, sec.ValidationContext),
			}

			return nil
		},
		retry.Attempts(sdsKeyPairValidationAttempts),
		retry.Context(ctx),
		retry.Delay(sdsKeyPairValidationDelay),
		retry.DelayType(retry.FixedDelay),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("building SDS secret %q after %d attempts: %w", sec.ServerCert, attempts, err)
	}

	if attempts > 1 {
		slog.Info(
			"recovered SDS secret after retrying torn cert rotation",
			"server_cert", sec.ServerCert,
			"attempts", attempts,
			"key_file", sec.SslKeyFile,
			"cert_file", sec.SslCertFile,
			"ca_file", sec.SslCaFile,
			"ocsp_file", sec.SslOcspFile,
		)
	}

	return certs, items, nil
}

// GetSnapshotVersion generates a version string by hashing the certs
func GetSnapshotVersion(certs ...any) (string, error) {
	hash, err := hashAllSafe(fnv.New64(), certs...)
	return fmt.Sprintf("%d", hash), err
}

// hashAllSafe replicates the behavior of hashutils.HashAllSafe from github.com/solo-io/go-utils
func hashAllSafe(hasher hash.Hash64, values ...any) (uint64, error) {
	if hasher == nil {
		hasher = fnv.New64()
	}
	for _, v := range values {
		if err := hashValueSafe(hasher, v); err != nil {
			return 0, err
		}
	}
	return hasher.Sum64(), nil
}

// hashValueSafe replicates the behavior of hashutils.hashValueSafe from github.com/solo-io/go-utils
func hashValueSafe(hasher hash.Hash64, val any) error {
	h, err := hashstructure.Hash(val, nil)
	if err != nil {
		return err
	}
	return binary.Write(hasher, binary.LittleEndian, h)
}

// readAndVerifyCert reads a PEM-encoded key/cert/CA file once and validates
// that it contains well-formed PEM blocks
func readAndVerifyCert(ctx context.Context, certFilePath string) ([]byte, error) {
	fileBytes, err := readFile(ctx, certFilePath)
	if err != nil {
		return nil, err
	}
	if !checkCert(fileBytes) {
		return nil, fmt.Errorf("failed to validate file %v", certFilePath)
	}

	return fileBytes, nil
}

func readFile(ctx context.Context, filePath string) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return os.ReadFile(filePath)
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
