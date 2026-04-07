package collections

import (
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetServedVersions(t *testing.T) {
	t.Run("returns requested served versions", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "tlsroutes.gateway.networking.k8s.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: "v1alpha3", Served: true},
					{Name: "v1", Served: true},
					{Name: "v1beta1", Served: false},
				},
			},
		})

		got := getServedVersions(client, "tlsroutes.gateway.networking.k8s.io", "v1", "v1alpha3", "v1beta1")
		require.True(t, got.Authoritative)
		require.True(t, got.Exists)
		require.True(t, got.Served["v1"])
		require.True(t, got.Served["v1alpha3"])
		require.False(t, got.Served["v1beta1"])
	})

	t.Run("tracks absence authoritatively", func(t *testing.T) {
		got := getServedVersions(apiextensionsfake.NewClientset(), "tcproutes.gateway.networking.k8s.io", "v1alpha2")
		require.True(t, got.Authoritative)
		require.False(t, got.Exists)
		require.False(t, got.Served["v1alpha2"])
	})

	t.Run("returns non-authoritative when discovery is unavailable", func(t *testing.T) {
		got := getServedVersions(nil, "tcproutes.gateway.networking.k8s.io", "v1alpha2")
		require.False(t, got.Authoritative)
		require.False(t, got.Exists)
		require.False(t, got.Served["v1alpha2"])
	})
}
