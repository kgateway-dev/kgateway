package local_rate_limit

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	simpleServiceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	commonManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	rateLimitManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "rate-limit.yaml")

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
)
