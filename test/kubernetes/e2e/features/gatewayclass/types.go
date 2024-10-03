package gatewayclass

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
)

var (
	gwParametersManifestFile  = filepath.Join(util.MustGetThisDir(), "testdata", "gatewayparameters.yaml")
	gcManifestFile            = filepath.Join(util.MustGetThisDir(), "testdata", "gatewayclass.yaml")
	supportedCrdsManifestFile = filepath.Join(util.MustGetThisDir(), "../../../../../projects/gateway2/crds", "gateway-crds.yaml")

	gwParams = &v1alpha1.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway-class-feature",
			Namespace: "default",
		},
	}

	gc = &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway-class-feature",
			Namespace: "default",
		},
	}
)
