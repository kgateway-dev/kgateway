package sds_server_v3

import (
	"context"

	sds_server "github.com/solo-io/gloo/projects/sds/pkg/server"
	"github.com/solo-io/go-utils/contextutils"

	"google.golang.org/grpc"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	secret "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	cache_types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v3"
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

func NewEnvoySdsServerV3(ctx context.Context, grpcServer *grpc.Server) sds_server.EnvoySdsServer {
	hasher := &EnvoyKey{}
	snapshotCache := cache.NewSnapshotCache(false, hasher, nil)
	srv := server.NewServer(ctx, snapshotCache, nil)

	// register services
	secret.RegisterSecretDiscoveryServiceServer(grpcServer, srv)
	return &envoySdsServerV3{
		srv:           srv,
		snapshotCache: snapshotCache,
	}
}

type envoySdsServerV3 struct {
	srv           server.Server
	snapshotCache cache.SnapshotCache
}

func (e envoySdsServerV3) UpdateSDSConfig(
	ctx context.Context,
	snapshotVersion, sslKeyFile, sslCertFile, sslCaFile string) error {
	contextutils.LoggerFrom(ctx).Infof("Updating SDS config. Snapshot version is %s", snapshotVersion)

	items := []cache_types.Resource{
		serverCertSecret(sslCertFile, sslKeyFile),
		validationContextSecret(sslCaFile),
	}
	secretSnapshot := cache.Snapshot{}
	secretSnapshot.Resources[cache_types.Secret] = cache.NewResources(snapshotVersion, items)
	return e.snapshotCache.SetSnapshot(sdsClient, secretSnapshot)
}

func serverCertSecret(certFile, keyFile string) cache_types.Resource {
	return &tls.Secret{
		Name: serverCert,
		Type: &tls.Secret_TlsCertificate{
			TlsCertificate: &tls.TlsCertificate{
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
	return &tls.Secret{
		Name: validationContext,
		Type: &tls.Secret_ValidationContext{
			ValidationContext: &tls.CertificateValidationContext{
				TrustedCa: &core.DataSource{
					Specifier: &core.DataSource_Filename{
						Filename: caFile,
					},
				},
			},
		},
	}
}
