package scheme

import (
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	for _, f := range []func(*runtime.Scheme) error{
		apiv1.AddToScheme, apiv1beta1.AddToScheme, corev1.AddToScheme, appsv1.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			os.Exit(1)
		}
	}
	return scheme

}
