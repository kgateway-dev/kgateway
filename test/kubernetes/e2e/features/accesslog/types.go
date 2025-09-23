package accesslog

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// manifests
	setupManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	fileSinkManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "filesink.yaml")
	grpcServiceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "grpc.yaml")
	oTelManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "otel.yaml")

	// Core infrastructure objects that we need to track
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	// TestAccessLogWithGrpcSink
	accessLoggerObjectMeta = metav1.ObjectMeta{
		Name:      "gateway-proxy-access-logger",
		Namespace: "default",
	}

	setup = base.TestCase{
		Manifests: []string{e2edefaults.CurlPodManifest, setupManifest},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestAccessLogWithFileSink": {
			Manifests: []string{fileSinkManifest},
		},
		"TestAccessLogWithGrpcSink": {
			Manifests: []string{grpcServiceManifest},
		},
		"TestAccessLogWithOTelSink": {
			Manifests: []string{oTelManifest},
		},
	}
)
