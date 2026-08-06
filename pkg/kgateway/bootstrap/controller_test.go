package bootstrap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

const (
	syncTimeout = 10 * time.Second
	syncPoll    = 10 * time.Millisecond
)

var watchLabel = map[string]string{wellknown.WatchLabel: wellknown.WatchLabelValue}

func hmacSecret(labels map[string]string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wellknown.OAuth2HMACSecret.Name,
			Namespace: wellknown.OAuth2HMACSecret.Namespace,
			Labels:    labels,
		},
		Data: map[string][]byte{wellknown.OAuth2HMACSecretKey: []byte("existing-key")},
	}
}

// newSyncedController starts a controller against a fake client seeded with objs.
func newSyncedController(t *testing.T, objs ...client.Object) (*controller, apiclient.Client) {
	t.Helper()

	client := apifake.NewClient(t, objs...)
	c := NewController(client)
	client.RunAndWait(t.Context().Done())
	require.Eventually(t, c.secretClient.HasSynced, syncTimeout, syncPoll,
		"bootstrap Secret client should sync")
	return c, client
}

func TestReconcileCreatesHMACSecretWithWatchLabel(t *testing.T) {
	c, client := newSyncedController(t)

	require.NoError(t, c.reconcile(wellknown.OAuth2HMACSecret))

	created, err := client.Kube().CoreV1().
		Secrets(wellknown.OAuth2HMACSecret.Namespace).
		Get(t.Context(), wellknown.OAuth2HMACSecret.Name, metav1.GetOptions{})
	require.NoError(t, err)
	// Without this label the Secrets collection could not read back the key that kgateway
	// just created for itself when secretDiscoveryMode is LABELED. It is applied in every
	// mode so that switching modes needs no migration.
	require.Equal(t, watchLabel, created.Labels)
	require.NotEmpty(t, created.Data[wellknown.OAuth2HMACSecretKey])
}

// TestReconcileLabelsPreexistingHMACSecret covers upgrading an install whose HMAC Secret was
// created before kgateway labeled it: the Secret already exists, so it is never recreated,
// and it would be invisible in LABELED mode unless reconcile adds the label.
func TestReconcileLabelsPreexistingHMACSecret(t *testing.T) {
	existing := hmacSecret(map[string]string{"operator-owned": "yes"})
	c, client := newSyncedController(t, existing)

	require.NoError(t, c.reconcile(wellknown.OAuth2HMACSecret))

	patched, err := client.Kube().CoreV1().
		Secrets(wellknown.OAuth2HMACSecret.Namespace).
		Get(t.Context(), wellknown.OAuth2HMACSecret.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, map[string]string{wellknown.WatchLabel: wellknown.WatchLabelValue, "operator-owned": "yes"}, patched.Labels,
		"the watch label should be added without dropping labels set by the operator")
	require.Equal(t, []byte("existing-key"), patched.Data[wellknown.OAuth2HMACSecretKey],
		"the existing key must be preserved")
}

func TestReconcileLeavesAlreadyLabeledHMACSecretAlone(t *testing.T) {
	c, client := newSyncedController(t, hmacSecret(watchLabel))

	require.NoError(t, c.reconcile(wellknown.OAuth2HMACSecret))

	secret, err := client.Kube().CoreV1().
		Secrets(wellknown.OAuth2HMACSecret.Namespace).
		Get(t.Context(), wellknown.OAuth2HMACSecret.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, watchLabel, secret.Labels)
	require.Equal(t, []byte("existing-key"), secret.Data[wellknown.OAuth2HMACSecretKey],
		"the key must never be regenerated for a Secret that already matches")
}
