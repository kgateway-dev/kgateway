//go:build e2e

package common

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"istio.io/istio/pkg/test/util/retry"
)

// GrpcReflectionAssertResponseMetadata uses a native gRPC client (grpc-go) from the test runner
// to call the Server Reflection service. This avoids needing generated protos (like yages.Echo)
// while still exercising the gRPC dataplane and allowing assertions on response metadata.
func (g *Gateway) GrpcReflectionAssertResponseMetadata(
	t *testing.T,
	port int,
	authority string,
	expectedKey string,
	expectedValue string,
	timeout ...time.Duration,
) {
	t.Helper()

	addr := fmt.Sprintf("%s:%d", g.Address, port)
	expectedKey = strings.ToLower(expectedKey)

	tmo := 10 * time.Second
	if len(timeout) > 0 && timeout[0] > 0 {
		tmo = timeout[0]
	}

	retry.UntilSuccessOrFail(t, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), tmo)
		defer cancel()

		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithAuthority(authority),
		)
		if err != nil {
			return fmt.Errorf("grpc client create %q failed: %w", addr, err)
		}
		defer conn.Close()

		client := grpc_reflection_v1.NewServerReflectionClient(conn)
		stream, err := client.ServerReflectionInfo(ctx)
		if err != nil {
			return fmt.Errorf("open ServerReflectionInfo stream failed: %w", err)
		}

		// Ask for a minimal response to force headers/metadata to be returned.
		// This is equivalent to the "grpcurl list" / reflection path most tests use.
		err = stream.Send(&grpc_reflection_v1.ServerReflectionRequest{
			MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{
				ListServices: "",
			},
		})
		if err != nil {
			return fmt.Errorf("send reflection request failed: %w", err)
		}

		_, err = stream.Recv()
		if err != nil {
			return fmt.Errorf("recv reflection response failed: %w", err)
		}

		// Close the client send-side. Server reflection should respond per-request and then
		// terminate once the client is done. We drain until EOF to ensure trailers are available.
		_ = stream.CloseSend()
		for {
			_, recvErr := stream.Recv()
			if recvErr == nil {
				continue
			}
			if recvErr == io.EOF {
				break
			}
			return fmt.Errorf("recv reflection response (drain) failed: %w", recvErr)
		}

		headerMD, hdrErr := stream.Header()
		if hdrErr != nil {
			return fmt.Errorf("get reflection response headers failed: %w", hdrErr)
		}
		trailerMD := stream.Trailer()

		// Keys are normalized to lowercase in grpc metadata.
		gotHeaders := headerMD.Get(expectedKey)
		for _, v := range gotHeaders {
			if v == expectedValue {
				return nil
			}
		}

		gotTrailers := trailerMD.Get(expectedKey)
		for _, v := range gotTrailers {
			if v == expectedValue {
				return nil
			}
		}

		return fmt.Errorf(
			"expected gRPC metadata %q to contain %q; got headers=%v trailers=%v",
			expectedKey, expectedValue, gotHeaders, gotTrailers,
		)
	})
}
