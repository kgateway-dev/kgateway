package testgrpcservice

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/onsi/ginkgo/v2"

	glootest "github.com/solo-io/gloo/test/v1helpers/test_grpc_service/glootest/protos"
	"github.com/solo-io/go-utils/healthchecker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// For reflection to work, this uses the golang/proto plugin. To install it, run this command:
//    go get -u github.com/golang/protobuf/protoc-gen-go

// In the unlikely event that you need to re-generate this proto, open this directory in a terminal
// and run the following commands:
//    mkdir -p glootest
//    mkdir -p descriptors
//    cd /tmp/
//    git clone https://github.com/protocolbuffers/protobuf
//    git clone http://github.com/googleapis/googleapis
//    export PROTOBUF_HOME=$PWD/protobuf/src
//    export GOOGLE_PROTOS_HOME=$PWD/googleapis
//    cd -
//    protoc -I. -I${GOOGLE_PROTOS_HOME} -I${PROTOBUF_HOME} --include_source_info --include_imports --go_out=plugins=grpc:glootest --descriptor_set_out=descriptors/proto.pb protos/glootest.proto

func RunServer(ctx context.Context) *TestGRPCServer {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	grpcServer := grpc.NewServer()
	reflection.Register(grpcServer)
	srv := newServer()
	hc := healthchecker.NewGrpc("TestService", health.NewServer(), false, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, hc.GetServer())
	glootest.RegisterTestServiceServer(grpcServer, srv)

	go func() {
		defer ginkgo.GinkgoRecover()

		_ = grpcServer.Serve(lis)
	}()
	go func() {
		defer ginkgo.GinkgoRecover()

		<-ctx.Done()
		grpcServer.Stop()
	}()

	time.Sleep(time.Millisecond)

	addr := lis.Addr().String()
	_, portstr, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}

	port, err := strconv.Atoi(portstr)
	if err != nil {
		panic(err)
	}

	srv.Port = uint32(port)
	srv.HealthChecker = hc

	return srv
}

func newServer() *TestGRPCServer {
	return &TestGRPCServer{
		C: make(chan *glootest.TestRequest),
	}
}

type TestGRPCServer struct {
	C             chan *glootest.TestRequest
	Port          uint32
	HealthChecker healthchecker.HealthChecker
	glootest.UnimplementedTestServiceServer
}

// Returns a list of all shelves in the bookstore.
func (s *TestGRPCServer) TestMethod(_ context.Context, req *glootest.TestRequest) (*glootest.TestResponse, error) {
	if req == nil {
		return &glootest.TestResponse{Str: "cannot be nil"}, nil
		//return nil, errors.New("cannot be nil")
	}
	go func() {
		s.C <- req
	}()
	return &glootest.TestResponse{Str: req.GetStr()}, nil
}

// Returns a list of all shelves in the bookstore.
func (s *TestGRPCServer) TestParameterMethod(_ context.Context, req *glootest.TestRequest) (*glootest.TestResponse, error) {
	if req == nil {
		return nil, errors.New("cannot be nil")
	}
	go func() {
		s.C <- &glootest.TestRequest{Str: req.GetStr()}
	}()
	return &glootest.TestResponse{Str: req.GetStr()}, nil
}
