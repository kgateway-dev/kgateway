// rlqs is a deterministic Rate Limit Quota Service used by kgateway's E2E
// tests. A bucket whose "decision" value is "deny" receives DENY_ALL; every
// other bucket receives ALLOW_ALL.
package main

import (
	"flag"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	rlqsv3 "github.com/envoyproxy/go-control-plane/envoy/service/rate_limit_quota/v3"
	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

var grpcAddress = flag.String("grpc-address", ":18081", "address for the gRPC server")

type server struct {
	rlqsv3.UnimplementedRateLimitQuotaServiceServer
}

func (s *server) StreamRateLimitQuotas(stream rlqsv3.RateLimitQuotaService_StreamRateLimitQuotasServer) error {
	for {
		report, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		response := &rlqsv3.RateLimitQuotaResponse{}
		for _, usage := range report.GetBucketQuotaUsages() {
			rule := envoytypev3.RateLimitStrategy_ALLOW_ALL
			if usage.GetBucketId().GetBucket()["decision"] == "deny" {
				rule = envoytypev3.RateLimitStrategy_DENY_ALL
			}
			response.BucketAction = append(response.BucketAction, &rlqsv3.RateLimitQuotaResponse_BucketAction{
				BucketId: usage.GetBucketId(),
				BucketAction: &rlqsv3.RateLimitQuotaResponse_BucketAction_QuotaAssignmentAction_{
					QuotaAssignmentAction: &rlqsv3.RateLimitQuotaResponse_BucketAction_QuotaAssignmentAction{
						AssignmentTimeToLive: durationpb.New(10 * time.Minute),
						RateLimitStrategy: &envoytypev3.RateLimitStrategy{
							Strategy: &envoytypev3.RateLimitStrategy_BlanketRule_{BlanketRule: rule},
						},
					},
				},
			})
		}
		if len(response.GetBucketAction()) > 0 {
			if err := stream.Send(response); err != nil {
				return err
			}
		}
	}
}

func main() {
	flag.Parse()
	listener, err := net.Listen("tcp", *grpcAddress)
	if err != nil {
		slog.Error("failed to listen", "address", *grpcAddress, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	rlqsv3.RegisterRateLimitQuotaServiceServer(grpcServer, &server{})
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	slog.Info("starting RLQS test server", "address", *grpcAddress)
	if err := grpcServer.Serve(listener); err != nil {
		slog.Error("RLQS test server stopped", "error", err)
		os.Exit(1)
	}
}
