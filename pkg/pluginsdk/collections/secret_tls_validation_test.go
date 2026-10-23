package collections_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/cert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// TestSecretTLSValidationFollowsSecretUpdates checks that the TLS validation cached on a Secret
// is reused while the Secret is unchanged, and that every update to the Secret is validated again.
func TestSecretTLSValidationFollowsSecretUpdates(t *testing.T) {
	ctx := t.Context()

	certPEM, keyPEM, err := cert.GenerateSelfSignedCertKey("example.com", nil, nil)
	require.NoError(t, err)
	tlsSecret := func(resourceVersion string, certChain []byte) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "tls", Namespace: "default", ResourceVersion: resourceVersion},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{corev1.TLSCertKey: certChain, corev1.TLSPrivateKeyKey: keyPEM},
		}
	}

	fakeClient := apifake.NewClient(t, tlsSecret("1", certPEM))
	commoncol, err := collections.NewCommonCollections(
		ctx, krtutil.NewKrtOptions(ctx.Done(), nil), fakeClient, wellknown.DefaultGatewayControllerName, apisettings.Settings{},
	)
	require.NoError(t, err)
	fakeClient.RunAndWait(ctx.Done())
	require.Eventually(t, commoncol.Secrets.HasSynced, 10*time.Second, 10*time.Millisecond, "secrets should sync")

	getSecret := func() *ir.Secret {
		secret, err := commoncol.Secrets.GetSecret(
			krt.TestingDummyContext{},
			krtcollections.From{GroupKind: schema.GroupKind{Group: wellknown.GatewayGroup, Kind: wellknown.GatewayKind}, Namespace: "default"},
			gwv1.SecretObjectReference{Name: "tls"},
		)
		if err != nil {
			return nil
		}
		return secret
	}
	updateSecret := func(secret *corev1.Secret) {
		_, err := fakeClient.Kube().CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		require.NoError(t, err)
	}

	var first *ir.Secret
	require.Eventually(t, func() bool {
		first = getSecret()
		return first != nil
	}, 10*time.Second, 10*time.Millisecond, "the Secret should be in the collection")
	require.NoError(t, first.ValidateTLS())
	assert.Same(t, first.TLSValidation, getSecret().TLSValidation, "an unchanged Secret should keep its cached validation")

	updateSecret(tlsSecret("2", []byte("not a cert")))
	require.Eventually(t, func() bool {
		secret := getSecret()
		return secret != nil && secret.Obj.GetResourceVersion() == "2"
	}, 10*time.Second, 10*time.Millisecond, "the update should reach the collection")
	assert.ErrorIs(t, getSecret().ValidateTLS(), ir.ErrInvalidTlsSecret, "the invalid update should be validated again")

	updateSecret(tlsSecret("3", certPEM))
	require.Eventually(t, func() bool {
		secret := getSecret()
		return secret != nil && secret.Obj.GetResourceVersion() == "3"
	}, 10*time.Second, 10*time.Millisecond, "the update should reach the collection")
	assert.NoError(t, getSecret().ValidateTLS(), "the fixed Secret should be validated again")
}
