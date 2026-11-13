package backendtlspolicy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
)

// TestBuildTranslateFunc_MissingCAConfigMapNamesSelector pins that a CA ConfigMap the
// ConfigMap label selector excludes is reported with the selector, since an excluded
// ConfigMap is indistinguishable from a missing one.
func TestBuildTranslateFunc_MissingCAConfigMapNamesSelector(t *testing.T) {
	for _, tt := range []struct {
		name         string
		selector     string
		wantSelector bool
	}{
		{name: "unfiltered"},
		{name: "narrowed", selector: "team=a", wantSelector: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			refgrants := krtcollections.NewRefGrantIndex(
				krt.NewStaticCollection[*gwv1b1.ReferenceGrant](nil, nil), apisettings.ReferenceGrantPermissive)
			cfgmaps := krtcollections.NewConfigMapIndex(
				krt.NewStaticCollection[*corev1.ConfigMap](nil, nil), refgrants,
				krtcollections.WithConfigMapLabelSelector(tt.selector))
			translate := buildTranslateFunc(cfgmaps, nil)

			_, err := translate(krt.TestingDummyContext{}, &gwv1.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "policy", Namespace: "default"},
				Spec: gwv1.BackendTLSPolicySpec{
					Validation: gwv1.BackendTLSPolicyValidation{
						CACertificateRefs: []gwv1.LocalObjectReference{{Kind: "ConfigMap", Name: "ca"}},
						Hostname:          "example.com",
					},
				},
			})
			require.ErrorContains(t, err, ErrConfigMapNotFound.Error())
			require.ErrorContains(t, err, "default/ca")
			require.Equal(t, tt.wantSelector, strings.Contains(err.Error(), `label selector "team=a" (Helm value configMapLabelSelector`), err.Error())
		})
	}
}
