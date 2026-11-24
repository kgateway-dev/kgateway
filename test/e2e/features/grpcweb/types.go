//go:build e2e

package grpcweb

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	setupManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayGRPCWebManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-grpc-web.yaml")
	routeGRPCWebManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-grpc-web.yaml")

	setup = base.TestCase{
		Manifests: []string{setupManifest},
	}

	testCases = map[string]*base.TestCase{
		"TestGRPCWebOnRoute": {
			Manifests: []string{routeGRPCWebManifest},
		},
		"TestGRPCWebOnGatewayWithRouteDisable": {
			Manifests: []string{gatewayGRPCWebManifest},
		},
	}
)
