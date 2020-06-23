package main

import (
	"context"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/sds/pkg/run"
	"github.com/solo-io/go-utils/contextutils"
	v1 "k8s.io/api/core/v1"
)

var (
	secretDir   = "/etc/envoy/ssl/"
	sslKeyFile  = secretDir + v1.TLSPrivateKeyKey        //tls.key
	sslCertFile = secretDir + v1.TLSCertKey              //tls.crt
	sslCaFile   = secretDir + v1.ServiceAccountRootCAKey //ca.crt

	// This must match the value of the sds_config target_uri in the envoy instance that it is providing
	// secrets to.
	sdsServerAddress = "127.0.0.1:8234"
)

func main() {

	ctx := contextutils.WithLogger(context.Background(), "sds_server")
	ctx = contextutils.WithLoggerValues(ctx, "version", version.Version)

	if err := run.Run(ctx, sslKeyFile, sslCertFile, sslCaFile, sdsServerAddress); err != nil {
		contextutils.LoggerFrom(ctx).Fatal(err)
	}
}
