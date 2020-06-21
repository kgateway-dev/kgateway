package sds_server

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/ioutil"

	"github.com/solo-io/go-utils/hashutils"
	"google.golang.org/grpc"
)

type EnvoySdsServer interface {
	UpdateSDSConfig(ctx context.Context, sslKeyFile, sslCertFile, sslCaFile string) error
}

type EnvoySdsServerFactory func(ctx context.Context, srv *grpc.Server) EnvoySdsServer

func GetSnapshotVersion(sslKeyFile, sslCertFile, sslCaFile string) (string, error) {
	var err error
	key, err := ioutil.ReadFile(sslKeyFile)
	if err != nil {
		return "", err
	}
	cert, err := ioutil.ReadFile(sslCertFile)
	if err != nil {
		return "", err
	}
	ca, err := ioutil.ReadFile(sslCaFile)
	if err != nil {
		return "", err
	}
	hash, err := hashutils.HashAllSafe(fnv.New64(), key, cert, ca)
	return fmt.Sprintf("%d", hash), err
}
