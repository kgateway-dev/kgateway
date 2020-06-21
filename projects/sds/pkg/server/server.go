package server

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"net"

	"github.com/solo-io/go-utils/hashutils"
	"go.uber.org/zap"

	"github.com/solo-io/go-utils/contextutils"

	"google.golang.org/grpc"

	auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	sds "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	cache_types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v2"
)

// These values must match the values in the envoy sidecar's common_tls_context
const (
	sdsClient         = "sds_client"         // node ID
	serverCert        = "server_cert"        // name of a tls_certificate_sds_secret_config
	validationContext = "validation_context" // name of the validation_context_sds_secret_config
)

var (
	grpcOptions = []grpc.ServerOption{grpc.MaxConcurrentStreams(10000)}
)

type EnvoyKey struct{}

func (h *EnvoyKey) ID(_ *core.Node) string {
	return sdsClient
}

func SetupEnvoySDS() (*grpc.Server, cache.SnapshotCache) {
	grpcServer := grpc.NewServer(grpcOptions...)
	hasher := &EnvoyKey{}
	snapshotCache := cache.NewSnapshotCache(false, hasher, nil)
	svr := server.NewServer(context.Background(), snapshotCache, nil)

	// register services
	sds.RegisterSecretDiscoveryServiceServer(grpcServer, svr)
	return grpcServer, snapshotCache
}

func RunSDSServer(ctx context.Context, grpcServer *grpc.Server, serverAddress string) (<-chan struct{}, error) {
	lis, err := net.Listen("tcp", serverAddress)
	if err != nil {
		return nil, err
	}
	contextutils.LoggerFrom(ctx).Infof("sds server listening on %s", serverAddress)
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			contextutils.LoggerFrom(ctx).Fatalw("fatal error in gRPC server", zap.String("address", serverAddress), zap.Error(err))
		}
	}()
	serverStopped := make(chan struct{})
	go func() {
		<-ctx.Done()
		contextutils.LoggerFrom(ctx).Infof("stopping sds server on %s\n", serverAddress)
		grpcServer.GracefulStop()
		serverStopped <- struct{}{}
	}()
	return serverStopped, nil
}

func GetSnapshotVersion(sslKeyFile, sslCertFile, sslCaFile string) (string, error) {
	var err error
	key, err := ioutil.ReadFile(sslKeyFile)
	if err != nil {
		return "", err
	}
	cert, err := ioutil.ReadFile(sslCertFile)
	if err != nil {
		return "", err
	}
	ca, err := ioutil.ReadFile(sslCaFile)
	if err != nil {
		return "", err
	}
	hash, err := hashutils.HashAllSafe(fnv.New64(), key, cert, ca)
	return fmt.Sprintf("%d", hash), err
}

func UpdateSDSConfig(ctx context.Context, sslKeyFile, sslCertFile, sslCaFile string, snapshotCache cache.SnapshotCache) error {
	snapshotVersion, err := GetSnapshotVersion(sslKeyFile, sslCertFile, sslCaFile)
	if err != nil {
		contextutils.LoggerFrom(ctx).Info("Error getting snapshot version", zap.Error(err))
		return err
	}
	contextutils.LoggerFrom(ctx).Infof("Updating SDS config. Snapshot version is %s", snapshotVersion)

	items := []cache_types.Resource{
		serverCertSecret(sslCertFile, sslKeyFile),
		validationContextSecret(sslCaFile),
	}
	secretSnapshot := cache.Snapshot{}
	secretSnapshot.Resources[cache_types.Secret] = cache.NewResources(snapshotVersion, items)
	return snapshotCache.SetSnapshot(sdsClient, secretSnapshot)
}

func serverCertSecret(certFile, keyFile string) cache_types.Resource {
	return &auth.Secret{
		Name: serverCert,
		Type: &auth.Secret_TlsCertificate{
			TlsCertificate: &auth.TlsCertificate{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_Filename{
						Filename: certFile,
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_Filename{
						Filename: keyFile,
					},
				},
			},
		},
	}
}

func validationContextSecret(caFile string) cache_types.Resource {
	return &auth.Secret{
		Name: validationContext,
		Type: &auth.Secret_ValidationContext{
			ValidationContext: &auth.CertificateValidationContext{
				TrustedCa: &core.DataSource{
					Specifier: &core.DataSource_Filename{
						Filename: caFile,
					},
				},
			},
		},
	}
}
