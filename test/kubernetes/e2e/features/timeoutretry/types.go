package timeoutretry

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

const (
	gatewayName = "test"
)

var (
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")

	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      gatewayName,
		Namespace: "default",
	}
)
