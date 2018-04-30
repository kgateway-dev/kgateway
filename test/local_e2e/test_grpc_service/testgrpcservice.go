package testgrpcservice

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/solo-io/gloo/test/local_e2e/test_grpc_service/glootest/protos"
	"google.golang.org/grpc"
)

//go:generate mkdir -p glootest
//go:generate mkdir -p descriptors
//go:generate protoc -I. --gogo_out=plugins=grpc:glootest  --descriptor_set_out=descriptors/proto.pb protos/glootest.proto

func RunServer() *TestGRPCServer {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	grpcServer := grpc.NewServer()
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
