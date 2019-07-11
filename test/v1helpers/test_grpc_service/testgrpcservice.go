package testgrpcservice

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	glootest "github.com/solo-io/gloo/test/v1helpers/test_grpc_service/glootest/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// For reflection to work, this uses the golang/proto plugin. To install it, run this command:
//    go get -u github.com/golang/protobuf/protoc-gen-go

// In the unlikely event that you need to re-generate this proto, open this directory in a terminal
// and run the following commands:
//    mkdir -p glootest
//    mkdir -p descriptors
//    protoc -I. --go_out=plugins=grpc:glootest --descriptor_set_out=descriptors/proto.pb protos/glootest.proto

func RunServer(ctx context.Context) *TestGRPCServer {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	grpcServer := grpc.NewServer()
	reflection.Register(grpcServer)
	srv := newServer()
	glootest.RegisterTestServiceServer(grpcServer, srv)
	go grpcServer.Serve(lis)
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

	return srv
}

func newServer() *TestGRPCServer {
	return &TestGRPCServer{
		C: make(chan *glootest.TestRequest),
	}
}

type TestGRPCServer struct {
	C    chan *glootest.TestRequest
	Port uint32
}

// Returns a list of all shelves in the bookstore.
func (s *TestGRPCServer) TestMethod(_ context.Context, req *glootest.TestRequest) (*glootest.TestResponse, error) {
	if req == nil {
		return nil, errors.New("cannot be nil")
	}
	go func() {
		s.C <- req
	}()
	return &glootest.TestResponse{Str: req.Str}, nil
}
