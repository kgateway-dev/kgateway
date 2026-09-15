package main

import (
	"context"
	"net"
	"testing"
	"time"

	rlqsv3 "github.com/envoyproxy/go-control-plane/envoy/service/rate_limit_quota/v3"
	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestStreamRateLimitQuotas(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	rlqsv3.RegisterRateLimitQuotaServiceServer(grpcServer, &server{})
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		require.NoError(t, <-serveErr)
	})

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stream, err := rlqsv3.NewRateLimitQuotaServiceClient(conn).StreamRateLimitQuotas(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&rlqsv3.RateLimitQuotaUsageReports{
		Domain: "test",
		BucketQuotaUsages: []*rlqsv3.RateLimitQuotaUsageReports_BucketQuotaUsage{
			{
				BucketId:    &rlqsv3.BucketId{Bucket: map[string]string{"decision": "deny"}},
				TimeElapsed: durationpb.New(time.Second),
			},
			{
				BucketId:    &rlqsv3.BucketId{Bucket: map[string]string{"decision": "allow"}},
				TimeElapsed: durationpb.New(time.Second),
			},
		},
	}))
	response, err := stream.Recv()
	require.NoError(t, err)
	require.Len(t, response.GetBucketAction(), 2)
	require.Equal(t, envoytypev3.RateLimitStrategy_DENY_ALL,
		response.GetBucketAction()[0].GetQuotaAssignmentAction().GetRateLimitStrategy().GetBlanketRule())
	require.Equal(t, envoytypev3.RateLimitStrategy_ALLOW_ALL,
		response.GetBucketAction()[1].GetQuotaAssignmentAction().GetRateLimitStrategy().GetBlanketRule())
}
