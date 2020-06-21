package sds_server

import (
	"context"

	"google.golang.org/grpc"
)

type EnvoySdsServer interface {
	UpdateSDSConfig(ctx context.Context, sslKeyFile, sslCertFile, sslCaFile string) error
}

type EnvoySdsServerFactory func(ctx context.Context, srv *grpc.Server) EnvoySdsServer

