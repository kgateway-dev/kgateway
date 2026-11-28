// A minimal gRPC external authorization server used by e2e tests. Its decisions
// can be based entirely on Envoy dynamic metadata rather than on client-supplied
// headers. An optional flag also lets it assert that a header is still present,
// which proves legacy pre-AuthN placement.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// defaultMetadataNamespace is the namespace kgateway's jwt_authn filter writes
	// verified payloads to.
	defaultMetadataNamespace = "envoy.filters.http.jwt_authn"
	// jwtPayloadKey is the field kgateway configures via payload_in_metadata.
	jwtPayloadKey = "payload"
	// allowedHeaderKey is added to allowed requests so the backend echo can prove
	// the request passed through this server.
	allowedHeaderKey = "x-ext-authz-check-result"
)

var (
	logger = slog.Default().With("component", "e2e/extauth")

	grpcport          = flag.String("grpcport", ":9000", "grpcport")
	metadataNamespace = flag.String("metadata-namespace", defaultMetadataNamespace, "dynamic metadata namespace to inspect")
	claimKey          = flag.String("claim-key", "email", "JWT payload claim to require")
	claimValue        = flag.String("claim-value", "", "required value of the JWT payload claim")
	skipJWTMetadata   = flag.Bool("skip-jwt-metadata", false, "skip the JWT dynamic metadata check")
	requiredHeader    = flag.String("require-header", "", "request header that must be present")
)

type authorizationServer struct{}

func (s *authorizationServer) Check(_ context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error) {
	if !*skipJWTMetadata && *claimValue == "" {
		return denied("server misconfigured: --claim-value is required"), nil
	}

	attributes := req.GetAttributes()
	metadata := attributes.GetMetadataContext().GetFilterMetadata()

	if !*skipJWTMetadata && !hasJWTClaim(metadata, *metadataNamespace, *claimKey, *claimValue) {
		logger.Info("denying request without matching JWT metadata",
			"namespace", *metadataNamespace,
			"claim", *claimKey,
			"namespaces", len(metadata),
		)
		return denied(fmt.Sprintf("%s=%s not found in %q JWT metadata", *claimKey, *claimValue, *metadataNamespace)), nil
	}

	if *requiredHeader != "" && !hasRequestHeader(attributes.GetRequest().GetHttp(), *requiredHeader) {
		logger.Info("denying request without required header", "header", *requiredHeader)
		return denied(fmt.Sprintf("required header %q is missing", *requiredHeader)), nil
	}

	logger.Info("allowing request",
		"jwt_metadata_checked", !*skipJWTMetadata,
		"required_header", *requiredHeader,
	)
	return allowed(), nil
}

// hasJWTClaim reports whether the verified JWT payload in the given metadata
// namespace carries the expected claim. Envoy writes payload_in_metadata as:
//
//	envoy.filters.http.jwt_authn:
//	  payload:
//	    claim: value
func hasJWTClaim(metadata map[string]*structpb.Struct, namespace, claimKey, claimValue string) bool {
	if claimValue == "" {
		return false
	}

	ns, ok := metadata[namespace]
	if !ok || ns == nil {
		return false
	}

	payload := ns.GetFields()[jwtPayloadKey].GetStructValue().GetFields()
	value := payload[claimKey]
	return value != nil && value.GetStringValue() == claimValue
}

// hasRequestHeader reports whether the request contains the named header with a
// non-empty value. Header names are compared case-insensitively because Envoy
// lowercases them on HTTP/2 check requests.
func hasRequestHeader(request *authv3.AttributeContext_HttpRequest, name string) bool {
	if request == nil || name == "" {
		return false
	}
	for headerName, value := range request.GetHeaders() {
		if strings.EqualFold(headerName, name) && value != "" {
			return true
		}
	}
	return false
}

func allowed() *authv3.CheckResponse {
	return &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: int32(codes.OK)},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: []*envoycorev3.HeaderValueOption{
					{
						Header: &envoycorev3.HeaderValue{
							Key:   allowedHeaderKey,
							Value: "allow",
						},
					},
				},
			},
		},
	}
}

func denied(reason string) *authv3.CheckResponse {
	return &authv3.CheckResponse{
		Status: &rpcstatus.Status{
			Code:    int32(codes.PermissionDenied),
			Message: reason,
		},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &envoytypev3.HttpStatus{Code: envoytypev3.StatusCode_Forbidden},
				Body:   "extauth: " + reason + "\n",
			},
		},
	}
}

type healthServer struct{}

func (s *healthServer) Check(_ context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (s *healthServer) Watch(_ *grpc_health_v1.HealthCheckRequest, _ grpc_health_v1.Health_WatchServer) error {
	return errors.New("Watch is not implemented")
}

func (s *healthServer) List(_ context.Context, _ *grpc_health_v1.HealthListRequest) (*grpc_health_v1.HealthListResponse, error) {
	return &grpc_health_v1.HealthListResponse{
		Statuses: map[string]*grpc_health_v1.HealthCheckResponse{
			"": {Status: grpc_health_v1.HealthCheckResponse_SERVING},
		},
	}, nil
}

func main() {
	flag.Parse()

	lis, err := net.Listen("tcp", *grpcport)
	if err != nil {
		logger.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	s := grpc.NewServer(grpc.MaxConcurrentStreams(1000))
	authv3.RegisterAuthorizationServer(s, &authorizationServer{})
	grpc_health_v1.RegisterHealthServer(s, &healthServer{})

	logger.Info("starting gRPC server",
		"port", *grpcport,
		"metadata_namespace", *metadataNamespace,
		"claim", *claimKey,
		"skip_jwt_metadata", *skipJWTMetadata,
		"required_header", *requiredHeader,
	)

	gracefulStop := make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-gracefulStop
		logger.Info("caught termination signal, stopping")
		s.GracefulStop()
	}()

	if err := s.Serve(lis); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
