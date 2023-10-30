package scheme

import (
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// TranslatorScheme is the scheme of resources used by the translator
var TranslatorScheme = runtime.NewScheme()

func init() {

	// TODO GENERATE??? @ilackarms
	_ = gloov1.AddToScheme(TranslatorScheme)
	_ = v1beta1.AddToScheme(TranslatorScheme)
	_ = gwv1.AddToScheme(TranslatorScheme)
}
