package grpc_test

import (
	"testing"

	"google.golang.org/grpc"

	"fmt"
	"net"

	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/tags"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/test/e2e/containers/grpc-test-service/bookstore/protos"
	"github.com/solo-io/gloo/test/e2e/containers/grpc-test-service/server"
	"github.com/solo-io/gloo/test/helpers"
	"go.uber.org/zap"
	"google.golang.org/grpc/reflection"
)

func TestGrpc(t *testing.T) {
	RegisterFailHandler(Fail)
	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "GRPC Discovery Suite")
}

var (
	port       = 8080
	grpcServer *grpc.Server
)

var _ = BeforeSuite(func() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	helpers.Must(err)
	grpcServer = grpc.NewServer(grpc.StreamInterceptor(
		grpc_middleware.ChainStreamServer(
			grpc_ctxtags.StreamServerInterceptor(),
			grpc_zap.StreamServerInterceptor(zap.NewNop()),
			func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				log.Printf("%v", info.FullMethod)
				return handler(srv, ss)
			},
		)))
	bookstore.RegisterBookstoreServer(grpcServer, server.NewServer())
	reflection.Register(grpcServer)
	log.Printf("grpc listening on %v", port)
	go grpcServer.Serve(lis)
})

var _ = AfterSuite(func() {
	grpcServer.Stop()
})
