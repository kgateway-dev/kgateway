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
	// just created for itself when discovery.secrets.mode is LABELED. It is applied in every
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

func TestNeedsWatchLabel(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{name: "no labels", labels: nil, want: true},
		{name: "label absent", labels: map[string]string{"other": "x"}, want: true},
		{name: "label set to false", labels: map[string]string{wellknown.WatchLabel: "false"}, want: true},
		{name: "label intact", labels: watchLabel, want: false},
		{name: "label alongside others", labels: map[string]string{wellknown.WatchLabel: wellknown.WatchLabelValue, "other": "x"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, needsWatchLabel(hmacSecret(tt.labels)))
		})
	}
}

// TestWatchLabelRemovalIsHealed covers someone editing the watch label off the HMAC Secret
// while the controller runs. This client is scoped by name so it still sees the Secret, but
// the label-filtered Secrets collection has dropped it, so the label has to be restored
// rather than waiting for a controller restart.
func TestWatchLabelRemovalIsHealed(t *testing.T) {
	c, client := newSyncedController(t, hmacSecret(watchLabel))

	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go c.queue.Run(stop)

	_, err := client.Kube().CoreV1().
		Secrets(wellknown.OAuth2HMACSecret.Namespace).
		Update(t.Context(), hmacSecret(map[string]string{"other": "x"}), metav1.UpdateOptions{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		secret, err := client.Kube().CoreV1().
			Secrets(wellknown.OAuth2HMACSecret.Namespace).
			Get(t.Context(), wellknown.OAuth2HMACSecret.Name, metav1.GetOptions{})
		return err == nil && secret.Labels[wellknown.WatchLabel] == wellknown.WatchLabelValue
	}, syncTimeout, syncPoll, "removing the watch label should be healed without a restart")

	secret, err := client.Kube().CoreV1().
		Secrets(wellknown.OAuth2HMACSecret.Namespace).
		Get(t.Context(), wellknown.OAuth2HMACSecret.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "x", secret.Labels["other"], "unrelated labels should be preserved")
	require.Equal(t, []byte("existing-key"), secret.Data[wellknown.OAuth2HMACSecretKey],
		"healing the label must not rotate the key")
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
