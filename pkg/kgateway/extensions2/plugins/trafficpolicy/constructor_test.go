package trafficpolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

func TestFetchGatewayExtensionErrorNamesTheExtension(t *testing.T) {
	discoveryErr := errors.New("error discovering OpenID provider config at https://idp.example.com/.well-known/openid-configuration; unexpected status code 503")
	c := &TrafficPolicyConstructor{
		commoncol: &collections.CommonCollections{},
		gatewayExtensions: krt.NewStaticCollection(nil, []TrafficPolicyGatewayExtensionIR{
			{Name: krt.Named{Name: "failing", Namespace: "idp"}.ResourceName(), Err: discoveryErr},
			{Name: krt.Named{Name: "healthy", Namespace: "default"}.ResourceName()},
		}),
	}
	ref := func(name string, namespace *gwv1.Namespace) shared.NamespacedObjectReference {
		return shared.NamespacedObjectReference{Name: gwv1.ObjectName(name), Namespace: namespace}
	}

	t.Run("failing extension", func(t *testing.T) {
		r := require.New(t)
		gwExt, err := c.FetchGatewayExtension(krt.TestingDummyContext{}, ref("failing", new(gwv1.Namespace("idp"))), "default")

		r.NotNil(gwExt, "the extension is still returned alongside its error")
		r.ErrorIs(err, discoveryErr)
		r.EqualError(err, "idp/failing: "+discoveryErr.Error())
	})

	t.Run("missing extension", func(t *testing.T) {
		r := require.New(t)
		gwExt, err := c.FetchGatewayExtension(krt.TestingDummyContext{}, ref("missing", nil), "default")

		r.Nil(gwExt)
		r.ErrorIs(err, pluginutils.ErrGatewayExtensionNotFound)
		r.ErrorContains(err, "default/missing: ")
	})

	t.Run("healthy extension", func(t *testing.T) {
		r := require.New(t)
		gwExt, err := c.FetchGatewayExtension(krt.TestingDummyContext{}, ref("healthy", nil), "default")

		r.NoError(err)
		r.NotNil(gwExt)
	})
}
