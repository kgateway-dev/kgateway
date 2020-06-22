package sds_server

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/ioutil"

	"github.com/hashicorp/go-multierror"
	"github.com/solo-io/go-utils/hashutils"
	"google.golang.org/grpc"
)

// Interface to allow running multiple EnvoySDSServers simultaneously.
// This is especially useful to support piecemeal upgrades from V2 -> V3.
type EnvoySdsServer interface {
	UpdateSDSConfig(ctx context.Context, snapshotVersion, sslKeyFile, sslCertFile, sslCaFile string) error
}

type EnvoySdsServerFactory func(ctx context.Context, srv *grpc.Server) EnvoySdsServer

type EnvoySdsServerList []EnvoySdsServer

func (e EnvoySdsServerList) UpdateSDSConfig(
	ctx context.Context,
	snapshotVersion, sslKeyFile, sslCertFile, sslCaFile string,
) error {
	multiErr := multierror.Error{}
	for _, v := range e {
		if err := v.UpdateSDSConfig(ctx, snapshotVersion, sslKeyFile, sslCertFile, sslCaFile); err != nil {
			multiErr.Errors = append(multiErr.Errors, err)
		}
	}
	return multiErr.ErrorOrNil()
}

// Visible for testing
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
