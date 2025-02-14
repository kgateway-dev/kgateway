//go:build ignore

package route_options

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	setupManifest                        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	basicRtoManifest                     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "basic-rto.yaml")
	basicRtoTargetRefManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "basic-rto-targetref.yaml")
	extraRtoManifest                     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "extra-rto.yaml")
	extraRtoTargetRefManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "extra-rto-targetref.yaml")
	badRtoManifest                       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "bad-rto.yaml")
	badRtoTargetRefManifest              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "bad-rto-targetref.yaml")
	httproute1Manifest                   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute1.yaml")
	httproute1ExtensionManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute1-extension.yaml")
	httproute1BadExtensionManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute1-bad-extension.yaml")
	httproute1MultipleExtensionsManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute1-multiple-extensions.yaml")
	httproute2Manifest                   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute2.yaml")
	mergeManifest                        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "merge.yaml")

	// When we apply the fault injection manifest files, we expect resources to be created with this metadata
	proxyService = &corev1.Service{
		ObjectMeta: objectMetaInDefault("gw"),
	}
	proxyDeployment = &appsv1.Deployment{
		ObjectMeta: objectMetaInDefault("gw"),
	}
	nginxPod = &corev1.Pod{
		ObjectMeta: objectMetaInDefault("nginx"),
	}
	exampleSvc = &corev1.Service{
		ObjectMeta: objectMetaInDefault("example-svc"),
	}

	// RouteOption resources to be created
	basicRtoMeta          = objectMetaInDefault("basic-rto")
	basicRtoTargetRefMeta = objectMetaInDefault("basic-rto-targetref")
	extraRtoMeta          = objectMetaInDefault("extra-rto")
	extraRtoTargetRefMeta = objectMetaInDefault("extra-rto-targetref")
	badRtoMeta            = objectMetaInDefault("bad-rto")
	badRtoTargetRefMeta   = objectMetaInDefault("bad-rto-targetref")
	extref1RtoMeta        = objectMetaInDefault("extref1")
	extref2RtoMeta        = objectMetaInDefault("extref2")
	target1RtoMeta        = objectMetaInDefault("target-1")
	target2RtoMeta        = objectMetaInDefault("target-2")

	// Expected response matchers for various route options applied
	expectedResponseWithBasicHeader          = expectedResponseWithFooHeader("basic-rto")
	expectedResponseWithBasicTargetRefHeader = expectedResponseWithFooHeader("basic-rto-targetref")
	expectedResponseWithExtraHeader          = expectedResponseWithFooHeader("extra-rto")
	expectedResponseWithExtraTargetRefHeader = expectedResponseWithFooHeader("extra-rto-targetref")
)

func objectMetaInDefault(name string) metav1.ObjectMeta {
	return objectMeta(name, "default")
}

func objectMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
}

func expectedResponseWithFooHeader(value string) *matchers.HttpResponse {
	return &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]interface{}{
			"foo": gomega.Equal(value),
		},
		Body: gstruct.Ignore(),
	}
}
