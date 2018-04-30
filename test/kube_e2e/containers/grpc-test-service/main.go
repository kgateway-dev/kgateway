package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/solo-io/gloo/test/kube_e2e/containers/grpc-test-service/bookstore/protos"
	"github.com/solo-io/gloo/test/kube_e2e/containers/grpc-test-service/server"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// requires https://github.com/googleapis/googleapis to be in ${HOME}/workspace/googleapis

//go:generate mkdir -p bookstore
//go:generate mkdir -p descriptors
//go:generate protoc -I${HOME}/workspace/googleapis -I. --include_source_info --gogo_out=plugins=grpc:bookstore  --include_imports --descriptor_set_out=descriptors/proto.pb protos/bookstore.proto

func main() {
	port := flag.Int("p", 8080, "port")
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer(grpc.StreamInterceptor(
		grpc_middleware.ChainStreamServer(
			grpc_ctxtags.StreamServerInterceptor(),
			grpc_zap.StreamServerInterceptor(zap.NewNop()),
			func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				log.Printf("%v", info.FullMethod)
				return handler(srv, ss)
			},
		)))
	bookstore.RegisterBookstoreServer(grpcServer, server.NewServer())
	log.Printf("listening on %v", *port)
	reflection.Register(grpcServer)
	grpcServer.Serve(lis)
}
