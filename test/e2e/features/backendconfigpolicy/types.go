//go:build e2e

package backendconfigpolicy

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	setupManifest                 = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	nginxManifest                 = filepath.Join(fsutils.MustGetThisDir(), "testdata", "nginx.yaml")
	dnsManifest                   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "dns.yaml")
	tlsInsecureManifest           = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tls-insecure.yaml")
	simpleTLSManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "simple-tls.yaml")
	systemCAManifest              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "system-ca.yaml")
	outlierDetectionManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "outlierdetection.yaml")
	missingTargetManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "missing-target.yaml")
	upstreamProxyProtocolManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "upstream-proxy-protocol.yaml")

	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: "kgateway-base",
	}
)
