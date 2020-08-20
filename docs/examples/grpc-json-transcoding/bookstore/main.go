package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// requires https://github.com/googleapis/googleapis to be in ${GOOGLE_PROTOS_HOME}

//go:generate mkdir -p descriptors
//go:generate protoc -I${GOOGLE_PROTOS_HOME} -I${PROTOBUF_HOME} -I. --include_source_info --go_out=plugins=grpc:. --include_imports --descriptor_set_out=descriptors/proto.pb bookstore.proto

func main() {
	port := flag.Int("p", 8080, "port")
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	RegisterBookstoreServer(grpcServer, NewServer())
	log.Printf("listening on %v", *port)
	reflection.Register(grpcServer)
	grpcServer.Serve(lis)
}
